package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Resort struct {
	ID         string
	Name       string
}

var resorts = []Resort{
	// Disney Resorts
	{ID: "bfc89fd6-314d-44b4-b89e-df1a89cf991e", Name: "Disneyland Resort"},
	{ID: "e957da41-3552-4cf6-b636-5babc5cbc4e5", Name: "Walt Disney World® Resort"},
	{ID: "abcfffe7-01f2-4f92-ae61-5093346f5a68", Name: "Hong Kong Disneyland Parks"},
	{ID: "faff60df-c766-4470-8adb-dee78e813f42", Name: "Tokyo Disney Resort"},
	{ID: "6e1464ca-1e9b-49c3-8937-c5c6f6675057", Name: "Shanghai Disney Resort"},
	{ID: "e8d0207f-da8a-4048-bec8-117aa946b2c2", Name: "Disneyland Paris"},

	// Universal Resorts
	{ID: "9fc68f1c-3f5e-4f09-89f2-aab2cf1a0741", Name: "Universal Studios"},
	{ID: "89db5d43-c434-4097-b71f-f6869f495a22", Name: "Universal Orlando Resort"},
}

type WebSocketClient struct {
	url     string
	apiKey  string
	conn    *websocket.Conn
	done    chan struct{}
	lastMessageTime time.Time
	entityManager *EntityManager
	writeMu sync.Mutex

	// Connection diagnostics
	connectedAt   time.Time
	lastPingRecv  time.Time
	lastPongRecv  time.Time
	lastPingSent  time.Time
	messageCount  uint64
	diagMu        sync.RWMutex

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

func NewWebSocketClient(url, apiKey string, entityManager *EntityManager) *WebSocketClient {
	client := &WebSocketClient{
		url:     url,
		apiKey:  apiKey,
		done:    make(chan struct{}),
		lastMessageTime: time.Now(),
		entityManager: entityManager,
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
			c.connectedAt = time.Now()
			// Record the reconnection timestamp
			AddReconnectionTimestamp()
			log.Printf("[WS] Connected to WebSocket at %s", c.connectedAt.Format("2006-01-02 15:04:05 MST"))

			// Set a read deadline to detect unresponsive connections
			pongWait := 60 * time.Second
			c.conn.SetReadDeadline(time.Now().Add(pongWait))

			c.conn.SetPingHandler(func(appData string) error {
				now := time.Now()
				c.diagMu.Lock()
				c.lastPingRecv = now
				c.diagMu.Unlock()
				log.Printf("[WS] Received Ping from server (appData=%q), sending Pong", appData)
				c.writeMu.Lock()
				defer c.writeMu.Unlock()
				c.conn.SetWriteDeadline(now.Add(10 * time.Second))
				err := c.conn.WriteControl(websocket.PongMessage, []byte(appData), now.Add(10*time.Second))
				if err != nil {
					log.Printf("[WS] Error sending Pong: %v", err)
				}
				return err
			})

			c.conn.SetPongHandler(func(appData string) error {
				now := time.Now()
				c.diagMu.RLock()
				lastPing := c.lastPingSent
				c.diagMu.RUnlock()
				rtt := now.Sub(lastPing)
				c.diagMu.Lock()
				c.lastPongRecv = now
				c.diagMu.Unlock()
				log.Printf("[WS] Received Pong from server (RTT=%v)", rtt)
				// Extend the read deadline since we received a pong
				c.conn.SetReadDeadline(now.Add(pongWait))
				return nil
			})

			c.conn.SetCloseHandler(func(code int, text string) error {
				c.diagMu.RLock()
				uptime := time.Since(c.connectedAt)
				sinceLastMsg := time.Since(c.lastMessageTime)
				sinceLastPing := time.Since(c.lastPingRecv)
				sinceLastPong := time.Since(c.lastPongRecv)
				msgCount := c.messageCount
				c.diagMu.RUnlock()
				log.Printf("[WS] Server closed connection: code=%d text=%q uptime=%v messages=%d sinceLastMsg=%v sinceLastPing=%v sinceLastPong=%v",
					code, text, uptime, msgCount, sinceLastMsg, sinceLastPing, sinceLastPong)
				return nil
			})

			// Subscribe to all resorts before starting the ping goroutine
			// to avoid concurrent writes on the WebSocket connection
			for _, resort := range resorts {
				if err := c.subscribe(resort.ID); err != nil {
					log.Printf("Failed to subscribe to %s (%s): %v", resort.Name, resort.ID, err)
				} else {
					log.Printf("Subscribed to %s (%s)", resort.Name, resort.ID)
				}
			}

			// Start a ticker to send pings
			pingPeriod := 30 * time.Second // Must be less than pongWait
			ticker := time.NewTicker(pingPeriod)
			pingStop := make(chan struct{})

			go func() {
				defer ticker.Stop()
				for {
					select {
					case <-ticker.C:
						c.writeMu.Lock()
						err := c.conn.WriteMessage(websocket.PingMessage, []byte{})
						c.writeMu.Unlock()
						now := time.Now()
						if err != nil {
							log.Printf("[WS] Error sending ping: %v", err)
							return
						}
						c.diagMu.Lock()
						c.lastPingSent = now
						c.diagMu.Unlock()
					case <-pingStop:
						return
					case <-c.done:
						return
					}
				}
			}()

			// Start reading messages
			c.diagMu.Lock()
			c.messageCount = 0
			c.diagMu.Unlock()
			for {
				_, message, err := c.conn.ReadMessage()
				if err != nil {
					c.diagMu.RLock()
					uptime := time.Since(c.connectedAt)
					sinceLastMsg := time.Since(c.lastMessageTime)
					sinceLastPingSent := time.Since(c.lastPingSent)
					sinceLastPingRecv := time.Since(c.lastPingRecv)
					sinceLastPongRecv := time.Since(c.lastPongRecv)
					msgCount := c.messageCount
					c.diagMu.RUnlock()

					reason := classifyDisconnect(err)
					log.Printf("[WS] Connection lost: reason=%s error=%v", reason, err)
					log.Printf("[WS] Diagnostics: uptime=%v messages=%d sinceLastMsg=%v sinceLastPingSent=%v sinceLastPingRecv=%v sinceLastPongRecv=%v",
						uptime, msgCount, sinceLastMsg, sinceLastPingSent, sinceLastPingRecv, sinceLastPongRecv)
					break
				}
				c.lastMessageTime = time.Now()
				c.diagMu.Lock()
				c.messageCount++
				c.diagMu.Unlock()
				c.handleMessage(message)
			}

			close(pingStop) // Stop the pinger when the read loop exits
			c.conn.Close()
			log.Printf("[WS] Reconnecting in 5 seconds...")
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
		QueueEntity(entity)

		// Increment message counter
		// c.incrementMsgCounter() // This was removed as it's not defined

		// log.Printf("[%s] Queued update for %s (Wait Time: %d, Status: %s)", 
		// 	timestamp, msg.Name, waitTime, msg.Data.Status)
	} else {
		log.Printf("[%s] Received message: %s", timestamp, string(message))
	}
}

// classifyDisconnect returns a human-readable reason for the disconnect
func classifyDisconnect(err error) string {
	if websocket.IsCloseError(err,
		websocket.CloseNormalClosure,
		websocket.CloseGoingAway,
		websocket.CloseNoStatusReceived) {
		closeErr, ok := err.(*websocket.CloseError)
		if ok {
			return fmt.Sprintf("server-close(code=%d)", closeErr.Code)
		}
		return "server-close"
	}
	if websocket.IsUnexpectedCloseError(err) {
		return "unexpected-close"
	}
	errStr := err.Error()
	if strings.Contains(errStr, "i/o timeout") {
		return "read-deadline-timeout"
	}
	if strings.Contains(errStr, "use of closed network connection") {
		return "connection-already-closed"
	}
	if strings.Contains(errStr, "connection reset by peer") {
		return "connection-reset"
	}
	if strings.Contains(errStr, "EOF") {
		return "eof"
	}
	return "unknown"
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