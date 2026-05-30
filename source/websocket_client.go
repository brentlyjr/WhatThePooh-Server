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
	ID   string
	Name string
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

// Keepalive timing: pingPeriod < pongWait; pongOverdueWarn < pongWait.
const (
	wsPingPeriod      = 30 * time.Second
	wsPongWait        = 60 * time.Second
	wsPongOverdueWarn = 45 * time.Second
	wsWatchdogEvery   = 5 * time.Second
)

type WebSocketClient struct {
	url             string
	apiKey          string
	conn            *websocket.Conn
	done            chan struct{}
	closeOnce       sync.Once
	lastMessageTime time.Time
	entityManager   *EntityManager
	writeMu         sync.Mutex

	// Connection diagnostics
	connectedAt       time.Time
	lastPingRecv      time.Time
	lastPongRecv      time.Time
	lastPingSent      time.Time
	messageCount      uint64
	pongOverdueWarned bool
	lastRTT           time.Duration
	diagMu            sync.RWMutex

	// Message counters
	messageCounts struct {
		sync.RWMutex
		eventCounts  map[string]uint64
		statusCounts map[EntityStatus]uint64
	}
}

// SubscriptionMessage represents the message sent to subscribe to an entity
type SubscriptionMessage struct {
	Event            string `json:"event"`
	EntityID         string `json:"entityId"`
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
		url:             url,
		apiKey:          apiKey,
		done:            make(chan struct{}),
		lastMessageTime: time.Now(),
		entityManager:   entityManager,
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

func formatSince(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return time.Since(t).Round(time.Millisecond).String()
}

func (c *WebSocketClient) logDisconnectSnapshot(reason string, err error) {
	c.diagMu.RLock()
	uptime := time.Since(c.connectedAt).Round(time.Second)
	sinceLastMsg := formatSince(c.lastMessageTime)
	sinceLastPingSent := formatSince(c.lastPingSent)
	sinceLastPongRecv := formatSince(c.lastPongRecv)
	msgCount := c.messageCount

	pingSent := c.lastPingSent
	pongRecv := c.lastPongRecv
	lastRTT := c.lastRTT
	c.diagMu.RUnlock()

	pingOutstanding := !pingSent.IsZero() && pingSent.After(pongRecv)
	outstandingFor := "0s"
	if pingOutstanding {
		outstandingFor = time.Since(pingSent).Round(time.Millisecond).String()
	}

	hint := "none"
	if reason == "read-deadline-timeout" {
		hint = "no_inbound_within_" + wsPongWait.String()
	}

	errStr := "none"
	if err != nil {
		errStr = err.Error()
	}

	log.Printf("[WS] Connection lost: reason=%s error=%s uptime=%v messages=%d sinceLastMsg=%s sinceLastPingSent=%s sinceLastPongRecv=%s pingOutstanding=%v outstandingFor=%s lastRTT=%v hint=%s",
		reason, errStr, uptime, msgCount, sinceLastMsg, sinceLastPingSent, sinceLastPongRecv, pingOutstanding, outstandingFor, lastRTT.Round(time.Millisecond), hint)
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

			conn, resp, err := dialer.Dial(c.url, headers)
			if err != nil {
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

			c.diagMu.Lock()
			c.messageCount = 0
			c.lastPingSent = time.Time{}
			c.lastPongRecv = time.Time{}
			c.lastPingRecv = time.Time{}
			c.pongOverdueWarned = false
			c.lastRTT = 0
			c.diagMu.Unlock()

			AddReconnectionTimestamp()
			log.Printf("[WS] Connected to WebSocket at %s", c.connectedAt.Format("2006-01-02 15:04:05 MST"))

			c.conn.SetReadDeadline(time.Now().Add(wsPongWait))

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

				c.diagMu.Lock()
				lastPing := c.lastPingSent
				rtt := now.Sub(lastPing)
				c.lastPongRecv = now
				c.lastRTT = rtt

				if c.pongOverdueWarned {
					log.Printf("[WS] Pong received (RTT=%v); keepalive OK", rtt.Round(time.Millisecond))
					c.pongOverdueWarned = false
				}
				c.diagMu.Unlock()

				c.conn.SetReadDeadline(now.Add(wsPongWait))
				return nil
			})

			c.conn.SetCloseHandler(func(code int, text string) error {
				log.Printf("[WS] Server close frame: code=%d text=%q", code, text)
				return nil
			})

			for _, resort := range resorts {
				if err := c.subscribe(resort.ID); err != nil {
					log.Printf("Failed to subscribe to %s (%s): %v", resort.Name, resort.ID, err)
				} else {
					log.Printf("Subscribed to %s (%s)", resort.Name, resort.ID)
				}
			}

			ticker := time.NewTicker(wsWatchdogEvery)
			pingTicker := time.NewTicker(wsPingPeriod)
			pingStop := make(chan struct{})

			go func() {
				defer ticker.Stop()
				defer pingTicker.Stop()
				for {
					select {
					case <-ticker.C:
						now := time.Now()

						c.diagMu.RLock()
						sent := c.lastPingSent
						recv := c.lastPongRecv
						warned := c.pongOverdueWarned
						c.diagMu.RUnlock()

						if !sent.IsZero() && sent.After(recv) {
							overdue := now.Sub(sent)
							if overdue >= wsPongOverdueWarn && !warned {
								log.Printf("[WS] Pong overdue: ping_sent=%v ago, last_pong=%v ago, outstanding=true",
									overdue.Round(time.Millisecond), formatSince(recv))

								c.diagMu.Lock()
								c.pongOverdueWarned = true
								c.diagMu.Unlock()
							}
						}
					case <-pingTicker.C:
						c.writeMu.Lock()
						err := c.conn.WriteMessage(websocket.PingMessage, []byte{})
						c.writeMu.Unlock()

						if err != nil {
							log.Printf("[WS] Error sending ping: %v", err)
							return
						}

						// Extend read window on ping send so quiet feeds don't die if pongs are slow.
						c.conn.SetReadDeadline(time.Now().Add(wsPongWait))

						c.diagMu.Lock()
						c.lastPingSent = time.Now()
						c.diagMu.Unlock()

					case <-pingStop:
						return
					case <-c.done:
						return
					}
				}
			}()

			for {
				_, message, err := c.conn.ReadMessage()
				if err != nil {
					reason := classifyDisconnect(err)
					c.logDisconnectSnapshot(reason, err)
					break
				}
				now := time.Now()
				c.lastMessageTime = now
				// Inbound frames refresh the read window; pong path tracked separately for diagnostics.
				c.conn.SetReadDeadline(now.Add(wsPongWait))
				c.diagMu.Lock()
				c.messageCount++
				c.diagMu.Unlock()
				c.handleMessage(message)
			}

			close(pingStop)
			c.conn.Close()
			log.Printf("[WS] Reconnecting in 5 seconds...")
			time.Sleep(5 * time.Second)
		}
	}
}

func (c *WebSocketClient) subscribe(entityID string) error {
	msg := SubscriptionMessage{
		Event:            "subscribe",
		EntityID:         entityID,
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

	var msg LiveDataMessage
	if err := json.Unmarshal(message, &msg); err != nil {
		log.Printf("[%s] Error parsing message: %v", timestamp, err)
		return
	}

	if msg.Event == "error" {
		log.Printf("[%s] WebSocket Error Event: %s", timestamp, string(message))
	}

	c.incrementCounter(msg.Event)

	if msg.Event == "heartbeat" {
		return
	}

	if msg.Event == "livedata" {
		c.incrementStatusCounter(EntityStatus(msg.Data.Status))

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
	} else {
		log.Printf("[%s] Received message: %s", timestamp, string(message))
	}
}

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
	if strings.Contains(errStr, "operation timed out") {
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
	c.closeOnce.Do(func() {
		close(c.done)
		if c.conn != nil {
			c.conn.Close()
		}
	})
}

func (c *WebSocketClient) GetEventStats() map[string]uint64 {
	c.messageCounts.RLock()
	defer c.messageCounts.RUnlock()

	stats := make(map[string]uint64)
	for eventType, count := range c.messageCounts.eventCounts {
		stats[eventType] = count
	}
	return stats
}

func (c *WebSocketClient) GetStatusStats() map[EntityStatus]uint64 {
	c.messageCounts.RLock()
	defer c.messageCounts.RUnlock()

	stats := make(map[EntityStatus]uint64)
	for status, count := range c.messageCounts.statusCounts {
		stats[status] = count
	}
	return stats
}
