package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type ParkType string

const (
	Disney   ParkType = "disney"
	Universal ParkType = "universal"
)

type Park struct {
	ID         string
	Name       string
	Type       ParkType
	IsSelected bool
	IsVisible  bool
}

var parks = []Park{
	// Disney Parks
	{ID: "bfc89fd6-314d-44b4-b89e-df1a89cf991e", Name: "Disneyland Resort"},
	{ID: "e957da41-3552-4cf6-b636-5babc5cbc4e5", Name: "Walt Disney World® Resort"},
	{ID: "abcfffe7-01f2-4f92-ae61-5093346f5a68", Name: "Hong Kong Disneyland Parks"},
	{ID: "faff60df-c766-4470-8adb-dee78e813f42", Name: "Tokyo Disney Resort"},
	{ID: "6e1464ca-1e9b-49c3-8937-c5c6f6675057", Name: "Shanghai Disney Resort"},
	{ID: "e8d0207f-da8a-4048-bec8-117aa946b2c2", Name: "Disneyland Paris"},

	// Universal Parks
	{ID: "9fc68f1c-3f5e-4f09-89f2-aab2cf1a0741", Name: "Universal Studios"},
	{ID: "89db5d43-c434-4097-b71f-f6869f495a22", Name: "Universal Orlando Resort"},
}

type WebSocketClient struct {
	url     string
	apiKey  string
	conn    *websocket.Conn
	done    chan struct{}
	
	// Message counters
	messageCounts struct {
		sync.RWMutex
		eventCounts  map[string]uint64
		statusCounts map[EntityStatus]uint64
	}
}

// SubscriptionMessage represents the message sent to subscribe to an entity
type SubscriptionMessage struct {
	Event    string `json:"event"`
	EntityID string `json:"entityId"`
	EntityTypeFilter string `json:"entityTypeFilter"`
}

// LiveDataMessage represents the full WebSocket message structure
type LiveDataMessage struct {
	Event      string `json:"event"`
	Name       string `json:"name"`
	EntityType string `json:"entityType"`
	EntityID   string `json:"entityId"`
	ParkID     string `json:"parkId"`
	Data       struct {
		Queue struct {
			STANDBY struct {
				WaitTime *int `json:"waitTime"`
			} `json:"STANDBY"`
		} `json:"queue"`
		Status string `json:"status"`
	} `json:"data"`
}

func NewWebSocketClient(url, apiKey string) *WebSocketClient {
	client := &WebSocketClient{
		url:    url,
		apiKey: apiKey,
		done:   make(chan struct{}),
	}
	client.messageCounts.eventCounts = make(map[string]uint64)
	client.messageCounts.statusCounts = make(map[EntityStatus]uint64)
	return client
}

func (c *WebSocketClient) incrementCounter(eventType string) {
	c.messageCounts.Lock()
	defer c.messageCounts.Unlock()
	c.messageCounts.eventCounts[eventType]++
}

func (c *WebSocketClient) incrementStatusCounter(status EntityStatus) {
	c.messageCounts.Lock()
	defer c.messageCounts.Unlock()
	c.messageCounts.statusCounts[status]++
}

func (c *WebSocketClient) Connect() {
	for {
		select {
		case <-c.done:
			return
		default:
			headers := http.Header{
				"X-API-Key": {c.apiKey},
				"Origin":    {"https://themeparks.wiki"},
			}

			log.Printf("Attempting to connect to %s with API key: %s", c.url, c.apiKey)
			log.Printf("Headers: %v", headers)

			dialer := websocket.Dialer{
				HandshakeTimeout: 45 * time.Second,
				Subprotocols:     []string{"v1"},
			}

			// First try the original URL
			conn, resp, err := dialer.Dial(c.url, headers)
			if err != nil {
				// If we get a redirect, try the new URL
				if resp != nil && resp.StatusCode == 301 {
					redirectURL := resp.Header.Get("Location")
					if redirectURL != "" {
						log.Printf("Following redirect to: %s", redirectURL)
						conn, _, err = dialer.Dial(redirectURL, headers)
					}
				}
				
				if err != nil {
					log.Printf("Failed to connect: %v", err)
					if resp != nil {
						log.Printf("Response Status: %s", resp.Status)
						log.Printf("Response Headers: %v", resp.Header)
					}
					time.Sleep(5 * time.Second)
					continue
				}
			}

			c.conn = conn
			// Record the reconnection timestamp
			AddReconnectionTimestamp()
			log.Printf("[%s] Connected to WebSocket", time.Now().Format("2006-01-02 15:04:05 MST"))

			// Subscribe to all parks
			for _, park := range parks {
				if err := c.subscribe(park.ID); err != nil {
					log.Printf("Failed to subscribe to %s (%s): %v", park.Name, park.ID, err)
				} else {
					log.Printf("Subscribed to %s (%s)", park.Name, park.ID)
				}
			}

			// Start reading messages
			for {
				_, message, err := c.conn.ReadMessage()
				if err != nil {
					log.Printf("Read error: %v", err)
					break
				}
				c.handleMessage(message)
			}

			c.conn.Close()
			time.Sleep(5 * time.Second)
		}
	}
}

func (c *WebSocketClient) subscribe(entityID string) error {
	msg := SubscriptionMessage{
		Event:    "subscribe",
		EntityID: entityID,
		EntityTypeFilter: "ATTRACTION",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	log.Printf("Sending subscription message: %s", string(data))
	return c.conn.WriteMessage(websocket.TextMessage, data)
}

func (c *WebSocketClient) handleMessage(message []byte) {
	timestamp := time.Now().Format("2006-01-02 15:04:05 MST")
	// log.Printf("[%s] Raw message: %s", timestamp, string(message))

	var msg LiveDataMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Printf("[%s] Error parsing message: %v", timestamp, err)
		return
	}

	// Log error events
	if msg.Event == "error" {
		log.Printf("[%s] WebSocket Error Event: %s", timestamp, string(message))
	}

	c.incrementCounter(msg.Event)

	if msg.Event == "heartbeat" {
		return
	}

	if msg.Event == "livedata" {
		// Increment status counter
		c.incrementStatusCounter(EntityStatus(msg.Data.Status))
		
		// Create entity from message
		waitTime := 0
		if msg.Data.Queue.STANDBY.WaitTime != nil {
			waitTime = *msg.Data.Queue.STANDBY.WaitTime
		}

		entity := Entity{
			EntityID:   msg.EntityID,
			Name:       msg.Name,
			EntityType: msg.EntityType,
			ParkID:     msg.ParkID,
			WaitTime:   waitTime,
			Status:     EntityStatus(msg.Data.Status),
		}

		// Queue the entity for processing
		QueueEntity(entity)

		// log.Printf("[%s] Queued update for %s (Wait Time: %d, Status: %s)", 
		// 	timestamp, msg.Name, waitTime, msg.Data.Status)
	} else {
		log.Printf("[%s] Received message: %s", timestamp, string(message))
	}
}

func (c *WebSocketClient) Close() {
	close(c.done)
	if c.conn != nil {
		c.conn.Close()
	}
}

func (c *WebSocketClient) GetEventStats() map[string]uint64 {
	c.messageCounts.RLock()
	defer c.messageCounts.RUnlock()
	
	// Create a copy of the event counts
	stats := make(map[string]uint64)
	for eventType, count := range c.messageCounts.eventCounts {
		stats[eventType] = count
	}
	return stats
}

func (c *WebSocketClient) GetStatusStats() map[EntityStatus]uint64 {
	c.messageCounts.RLock()
	defer c.messageCounts.RUnlock()
	
	// Create a copy of the status counts
	stats := make(map[EntityStatus]uint64)
	for status, count := range c.messageCounts.statusCounts {
		stats[status] = count
	}
	return stats
} 