package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
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

// RestClient handles REST API calls to pre-populate entity data
type RestClient struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NewRestClient creates a new REST client
func NewRestClient(apiKey string) *RestClient {
	return &RestClient{
		baseURL: "https://api.themeparks.wiki/v1/entity",
		apiKey:  apiKey,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// PrePopulateEntities fetches data from all parks and pre-populates the entity manager
func (rc *RestClient) PrePopulateEntities(entityManager *EntityManager) error {
	log.Printf("Starting pre-population of entities from REST API...")
	
	totalEntities := 0
	
	// Fetch data for each park
	for _, park := range parks {
		log.Printf("Fetching entities for park: %s (%s)", park.Name, park.ID)
		
		entities, err := rc.fetchParkEntities(park.ID)
		if err != nil {
			log.Printf("Error fetching entities for park %s: %v", park.Name, err)
			continue // Continue with other parks even if one fails
		}
		
		// Convert and add entities to the manager
		count := rc.addEntitiesToManager(entities, entityManager)
		totalEntities += count
		
		log.Printf("Added %d entities for park %s", count, park.Name)
		
		// Small delay between requests to be respectful to the API
		time.Sleep(100 * time.Millisecond)
	}
	
	log.Printf("Pre-population complete! Added %d total entities", totalEntities)
	return nil
}

// fetchParkEntities fetches live data for a specific park
func (rc *RestClient) fetchParkEntities(parkID string) ([]LiveDataEntity, error) {
	url := fmt.Sprintf("%s/%s/live?entityType=ATTRACTION", rc.baseURL, parkID)
	
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}
	
	// Add API key header
	req.Header.Set("X-API-Key", rc.apiKey)
	req.Header.Set("User-Agent", "WhatThePooh-Server/1.0")
	
	resp, err := rc.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API request failed with status %d: %s", resp.StatusCode, string(body))
	}
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}
	
	var response ParkLiveDataResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %v", err)
	}
	
	return response.LiveData, nil
}

// addEntitiesToManager converts REST API entities to our Entity format and adds them to the manager
func (rc *RestClient) addEntitiesToManager(restEntities []LiveDataEntity, entityManager *EntityManager) int {
	count := 0
	
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
		
		// Create our Entity format
		entity := Entity{
			EntityID:           restEntity.ID,
			Name:              restEntity.Name,
			EntityType:        restEntity.EntityType,
			ParkID:            restEntity.ParkID,
			WaitTime:          waitTime,
			Status:            status,
			LastStatusChange:  lastUpdated,
			LastWaitTimeChange: lastUpdated,
		}
		
		// Add to entity manager (this will not trigger status change notifications since it's initial population)
		entityManager.UpdateEntity(entity)
		count++
	}
	
	return count
}

// GetEntityCount returns the current number of entities in the manager
func (rc *RestClient) GetEntityCount(entityManager *EntityManager) int {
	entities := entityManager.GetAllEntities()
	return len(entities)
}

// GetEntityStats returns statistics about the entities in the manager
func (rc *RestClient) GetEntityStats(entityManager *EntityManager) map[string]interface{} {
	entities := entityManager.GetAllEntities()
	
	stats := map[string]interface{}{
		"total_entities": len(entities),
		"parks":         make(map[string]int),
		"statuses":      make(map[string]int),
	}
	
	// Count entities by park
	for _, entity := range entities {
		// Count by park
		parkName := "Unknown"
		for _, park := range parks {
			if park.ID == entity.ParkID {
				parkName = park.Name
				break
			}
		}
		stats["parks"].(map[string]int)[parkName]++
		
		// Count by status
		status := string(entity.Status)
		stats["statuses"].(map[string]int)[status]++
	}
	
	return stats
} 