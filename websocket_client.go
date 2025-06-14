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
	{ID: "7340550b-c14d-4def-80bb-acdb51d49a66", Name: "Disneyland Park", Type: Disney, IsSelected: true, IsVisible: true},
	{ID: "832fcd51-ea19-4e77-85c7-75d5843b127c", Name: "Disney California Adventure Park", Type: Disney, IsSelected: false, IsVisible: true},
	{ID: "75ea578a-adc8-4116-a54d-dccb60765ef9", Name: "Magic Kingdom Park", Type: Disney, IsSelected: false, IsVisible: true},
	{ID: "47f90d2c-e191-4239-a466-5892ef59a88b", Name: "EPCOT", Type: Disney, IsSelected: false, IsVisible: true},
	{ID: "288747d1-8b4f-4a64-867e-ea7c9b27bad8", Name: "Disney's Hollywood Studios", Type: Disney, IsSelected: false, IsVisible: true},
	{ID: "1c84a229-8862-4648-9c71-378ddd2c7693", Name: "Disney's Animal Kingdom Theme Park", Type: Disney, IsSelected: false, IsVisible: true},
	{ID: "bd0eb47b-2f02-4d4d-90fa-cb3a68988e3b", Name: "Hong Kong Disneyland", Type: Disney, IsSelected: false, IsVisible: false},
	{ID: "3cc919f1-d16d-43e0-8c3f-1dd269bd1a42", Name: "Tokyo Disneyland", Type: Disney, IsSelected: false, IsVisible: false},
	{ID: "67b290d5-3478-4f23-b601-2f8fb71ba803", Name: "Tokyo DisneySea", Type: Disney, IsSelected: false, IsVisible: false},
	{ID: "ddc4357c-c148-4b36-9888-07894fe75e83", Name: "Shanghai Disneyland", Type: Disney, IsSelected: false, IsVisible: false},
	{ID: "dae968d5-630d-4719-8b06-3d107e944401", Name: "Disneyland Park (Paris)", Type: Disney, IsSelected: false, IsVisible: false},
	{ID: "ca888437-ebb4-4d50-aed2-d227f7096968", Name: "Walt Disney Studios Park", Type: Disney, IsSelected: false, IsVisible: false},
	// Universal Parks
	{ID: "bc4005c5-8c7e-41d7-b349-cdddf1796427", Name: "Universal Studios Hollywood", Type: Universal, IsSelected: false, IsVisible: true},
	{ID: "eb3f4560-2383-4a36-9152-6b3e5ed6bc57", Name: "Universal Studios Florida", Type: Universal, IsSelected: false, IsVisible: true},
	{ID: "267615cc-8943-4c2a-ae2c-5da728ca591f", Name: "Universal Islands of Adventure", Type: Universal, IsSelected: false, IsVisible: true},
	{ID: "12dbb85b-265f-44e6-bccf-f1faa17211fc", Name: "Universal's Epic Universe", Type: Universal, IsSelected: false, IsVisible: true},
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
	log.Printf("[%s] Raw message: %s", timestamp, string(message))

	var msg LiveDataMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Printf("Failed to parse message: %v", err)
		return
	}

	// Increment counter for this event type
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

		log.Printf("[%s] Queued update for %s (Wait Time: %d, Status: %s)", 
			timestamp, msg.Name, waitTime, msg.Data.Status)
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