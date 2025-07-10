package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// Entity represents a theme park attraction or other entity
type Entity struct {
	EntityID           string       `json:"entityId"`
	Name              string       `json:"name"`
	EntityType        string       `json:"entityType"`
	ParkID            string       `json:"parkId"`
	WaitTime          int          `json:"waitTime"`
	Status            EntityStatus `json:"status"`
	LastStatusChange  time.Time    `json:"lastStatusChange"`
	LastWaitTimeChange time.Time    `json:"lastWaitTimeChange"`
}

// EntityStatus represents the status of an entity
type EntityStatus string

const (
	StatusOperating    EntityStatus = "OPERATING"
	StatusClosed       EntityStatus = "CLOSED"
	StatusRefurbishment EntityStatus = "REFURBISHMENT"
	StatusDown         EntityStatus = "DOWN"
	StatusUnknown      EntityStatus = "UNKNOWN"
)

// REST API response structures
type ParkLiveDataResponse struct {
	ID       string       `json:"id"`
	Name     string       `json:"name"`
	EntityType string     `json:"entityType"`
	Timezone string       `json:"timezone"`
	LiveData []LiveDataEntity `json:"liveData"`
}

type LiveDataEntity struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	EntityType   string                 `json:"entityType"`
	ParkID       string                 `json:"parkId"`
	ExternalID   string                 `json:"externalId"`
	Status       string                 `json:"status"`
	LastUpdated  string                 `json:"lastUpdated"`
	Queue        map[string]QueueData   `json:"queue,omitempty"`
	OperatingHours []OperatingHour     `json:"operatingHours,omitempty"`
}

type QueueData struct {
	WaitTime *int `json:"waitTime"`
}

type OperatingHour struct {
	Type      string `json:"type"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

// WebSocket message structures
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



type ComparisonProgram struct {
	websocketEntities map[string]Entity
	pollingEntities   map[string]Entity
	websocketMutex    sync.RWMutex
	pollingMutex      sync.RWMutex
	
	// Configuration
	disneylandParkID string
	websocketURL     string
	restAPIURL       string
	apiKey           string
	
	// HTTP client for REST API
	httpClient *http.Client
}

func NewComparisonProgram() *ComparisonProgram {
	return &ComparisonProgram{
		websocketEntities: make(map[string]Entity),
		pollingEntities:   make(map[string]Entity),
		disneylandParkID:  "bfc89fd6-314d-44b4-b89e-df1a89cf991e", // Disneyland Resort
		websocketURL:      "wss://themeparkswiki.herokuapp.com/v1/live",
		restAPIURL:        "https://api.themeparks.wiki/v1/entity",
		apiKey:            "519dd9c1-cc1e-4d4a-906d-d628cf0250bc", // From your existing code
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (cp *ComparisonProgram) Start() {
	log.Printf("Starting comparison program for Disneyland Resort...")
	
	// First, do initial polling to build baseline data
	log.Printf("Building initial baseline data via polling...")
	cp.fetchAndUpdatePolling()
	
	// Copy polling data to websocket array
	cp.pollingMutex.RLock()
	cp.websocketMutex.Lock()
	for id, entity := range cp.pollingEntities {
		cp.websocketEntities[id] = entity
	}
	cp.websocketMutex.Unlock()
	cp.pollingMutex.RUnlock()
	
	log.Printf("Initial baseline established. %d entities loaded into both arrays.", len(cp.pollingEntities))
	
	// Start websocket connection
	go cp.startWebSocket()
	
	// Start polling (will skip initial fetch since we already did it)
	go cp.startPolling()
	
	// Keep the main thread alive
	select {}
}

func (cp *ComparisonProgram) startWebSocket() {
	for {
		log.Printf("Connecting to websocket: %s", cp.websocketURL)
		
		headers := http.Header{
			"X-API-Key": {cp.apiKey},
			"Origin":    {"https://themeparks.wiki"},
		}

		dialer := websocket.Dialer{
			HandshakeTimeout: 45 * time.Second,
			Subprotocols:     []string{"v1"},
		}

		conn, resp, err := dialer.Dial(cp.websocketURL, headers)
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
				log.Printf("Failed to connect to websocket: %v", err)
				if resp != nil {
					log.Printf("Response Status: %s", resp.Status)
					log.Printf("Response Headers: %v", resp.Header)
				}
				time.Sleep(5 * time.Second)
				continue
			}
		}
		
		log.Printf("Websocket connected successfully")
		
		// Subscribe to Disneyland Resort
		subscribeMsg := map[string]interface{}{
			"event": "subscribe",
			"entityId": cp.disneylandParkID,
			"entityTypeFilter": "ATTRACTION",
		}
		
		if err := conn.WriteJSON(subscribeMsg); err != nil {
			log.Printf("Failed to send subscription: %v", err)
			conn.Close()
			time.Sleep(5 * time.Second)
			continue
		}
		
		log.Printf("Subscribed to Disneyland Resort (ID: %s)", cp.disneylandParkID)
		
		// Listen for messages
		for {
			// Read the raw message first
			_, rawMessage, err := conn.ReadMessage()
			if err != nil {
				log.Printf("Websocket read error: %v", err)
				break
			}
			
			// Print the raw JSON message for debugging
//			log.Printf("Raw websocket message: %s", string(rawMessage))
			
			// Parse into our struct
			var msg LiveDataMessage
			if err := json.Unmarshal(rawMessage, &msg); err != nil {
				log.Printf("Failed to parse websocket message: %v", err)
				continue
			}
			
			// Only process ATTRACTION entities
			if msg.EntityType != "ATTRACTION" {
				continue
			}
			
			cp.handleWebSocketUpdate(msg)
		}
		
		conn.Close()
		log.Printf("Websocket disconnected, reconnecting in 5 seconds...")
		time.Sleep(5 * time.Second)
	}
}

func (cp *ComparisonProgram) handleWebSocketUpdate(update LiveDataMessage) {
	// Use current time since the websocket doesn't provide lastUpdated
	lastUpdated := time.Now()
	
	// Extract wait time from queue data
	waitTime := 0
	if update.Data.Queue.STANDBY.WaitTime != nil {
		waitTime = *update.Data.Queue.STANDBY.WaitTime
	}
	
	// Convert status string to EntityStatus
	status := EntityStatus(update.Data.Status)
	
	// Create entity
	newEntity := Entity{
		EntityID:           update.EntityID,
		Name:              update.Name,
		EntityType:        update.EntityType,
		ParkID:            update.ParkID,
		WaitTime:          waitTime,
		Status:            status,
		LastStatusChange:  lastUpdated,
		LastWaitTimeChange: lastUpdated,
	}
	
	cp.websocketMutex.Lock()
	defer cp.websocketMutex.Unlock()
	
			// Check if entity exists and if there are changes
		if existingEntity, exists := cp.websocketEntities[update.EntityID]; exists {
			// Check for status change
			if existingEntity.Status != newEntity.Status {
				fmt.Printf("[%s] Websocket connection updated ride %s from %s to %s (lastUpdated: %s)\n", 
					time.Now().Format("15:04:05.000"), update.Name, existingEntity.Status, newEntity.Status, lastUpdated.Format("2006/01/02 15:04:05"))
			}
			
			// Check for wait time change
			if existingEntity.WaitTime != newEntity.WaitTime {
				fmt.Printf("[%s] Websocket connection updated ride %s wait time from %d to %d (lastUpdated: %s)\n", 
					time.Now().Format("15:04:05.000"), update.Name, existingEntity.WaitTime, newEntity.WaitTime, lastUpdated.Format("2006/01/02 15:04:05"))
			}
		} else {
			fmt.Printf("[%s] Websocket connection added new ride %s (Status: %s, Wait Time: %d) (lastUpdated: %s)\n", 
				time.Now().Format("15:04:05.000"), update.Name, newEntity.Status, newEntity.WaitTime, lastUpdated.Format("2006/01/02 15:04:05"))
		}
	
	cp.websocketEntities[update.EntityID] = newEntity
}

func (cp *ComparisonProgram) startPolling() {
	// Skip initial fetch since it's already done in Start()
	
	// Poll every minute
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()
	
	for range ticker.C {
		cp.fetchAndUpdatePolling()
	}
}

func (cp *ComparisonProgram) fetchAndUpdatePolling() {
	url := fmt.Sprintf("%s/%s/live?entityType=ATTRACTION", cp.restAPIURL, cp.disneylandParkID)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Printf("Failed to create REST request: %v", err)
		return
	}
	
	// Add API key header
	req.Header.Set("X-API-Key", cp.apiKey)
	req.Header.Set("User-Agent", "WhatThePooh-Comparison/1.0")
	
	resp, err := cp.httpClient.Do(req)
	if err != nil {
		log.Printf("Failed to make REST request: %v", err)
		return
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		log.Printf("REST API request failed with status %d: %s", resp.StatusCode, string(body))
		return
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("Failed to read REST response body: %v", err)
		return
	}
	
	var response ParkLiveDataResponse
	if err := json.Unmarshal(body, &response); err != nil {
		log.Printf("Failed to parse REST JSON response: %v", err)
		return
	}
	
	cp.updatePollingEntities(response.LiveData)
}

func (cp *ComparisonProgram) updatePollingEntities(restEntities []LiveDataEntity) {
	cp.pollingMutex.Lock()
	defer cp.pollingMutex.Unlock()
	
	for _, restEntity := range restEntities {
		// Only process ATTRACTION entities
		if restEntity.EntityType != "ATTRACTION" {
			continue
		}
		
		// Parse last updated time
		lastUpdated, err := time.Parse(time.RFC3339, restEntity.LastUpdated)
		if err != nil {
			log.Printf("Warning: Could not parse lastUpdated for entity %s: %v", restEntity.ID, err)
			lastUpdated = time.Now()
		}
		
		// Extract wait time from queue data
		waitTime := 0
		if restEntity.Queue != nil {
			if standby, exists := restEntity.Queue["STANDBY"]; exists && standby.WaitTime != nil {
				waitTime = *standby.WaitTime
			}
		}
		
		// Convert status string to EntityStatus
		status := EntityStatus(restEntity.Status)
		
		// Create entity
		newEntity := Entity{
			EntityID:           restEntity.ID,
			Name:              restEntity.Name,
			EntityType:        restEntity.EntityType,
			ParkID:            restEntity.ParkID,
			WaitTime:          waitTime,
			Status:            status,
			LastStatusChange:  lastUpdated,
			LastWaitTimeChange: lastUpdated,
		}
		

		
		// Check if entity exists and if there are changes
		if existingEntity, exists := cp.pollingEntities[restEntity.ID]; exists {
			// Check for status change
			if existingEntity.Status != newEntity.Status {
				fmt.Printf("[%s] Polling updated ride %s from %s to %s (lastUpdated: %s)\n", 
					time.Now().Format("15:04:05.000"), restEntity.Name, existingEntity.Status, newEntity.Status, lastUpdated.Format("2006/01/02 15:04:05"))
			}
			
			// Check for wait time change
			if existingEntity.WaitTime != newEntity.WaitTime {
				fmt.Printf("[%s] Polling updated ride %s wait time from %d to %d (lastUpdated: %s)\n", 
					time.Now().Format("15:04:05.000"), restEntity.Name, existingEntity.WaitTime, newEntity.WaitTime, lastUpdated.Format("2006/01/02 15:04:05"))
			}
		} else {
			fmt.Printf("[%s] Polling added new ride %s (Status: %s, Wait Time: %d) (lastUpdated: %s)\n", 
				time.Now().Format("15:04:05.000"), restEntity.Name, newEntity.Status, newEntity.WaitTime, lastUpdated.Format("2006/01/02 15:04:05"))
		}
		
		cp.pollingEntities[restEntity.ID] = newEntity
	}
	
	log.Printf("Polling update complete. Total entities: %d", len(cp.pollingEntities))
}

func main() {
	// Set up logging
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	
	// Create and start comparison program
	program := NewComparisonProgram()
	
	// Handle graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-c
		log.Printf("Shutting down comparison program...")
		os.Exit(0)
	}()
	
	program.Start()
} 