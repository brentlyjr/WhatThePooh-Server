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

// FetchAllEntities fetches live data for every configured resort and returns a
// flat slice of Entity records. It does not touch the EntityManager or the
// database — callers (typically startup in main.go) feed the result into
// EntityManager.BulkLoad, then run EntityManager.ReconcileAgainst once
// subscribers are attached to the message bus.
func (rc *RestClient) FetchAllEntities() ([]Entity, error) {
	log.Printf("Starting REST fetch of entities from %d resorts...", len(resorts))

	var allEntities []Entity

	for _, resort := range resorts {
		log.Printf("Fetching entities for resort: %s (%s)", resort.Name, resort.ID)

		restEntities, err := rc.fetchResortEntities(resort.ID)
		if err != nil {
			log.Printf("Error fetching entities for resort %s: %v", resort.Name, err)
			continue // Continue with other resorts even if one fails
		}

		count := 0
		for _, re := range restEntities {
			entity, ok := convertRestEntity(re)
			if !ok {
				continue
			}
			allEntities = append(allEntities, entity)
			count++
		}

		log.Printf("Fetched %d entities for resort %s", count, resort.Name)

		// Small delay between requests to be respectful to the API
		time.Sleep(100 * time.Millisecond)
	}

	log.Printf("REST fetch complete! Total entities: %d", len(allEntities))
	return allEntities, nil
}

// fetchResortEntities fetches live data for a specific resort
func (rc *RestClient) fetchResortEntities(resortID string) ([]LiveDataEntity, error) {
	url := fmt.Sprintf("%s/%s/live?entityType=ATTRACTION", rc.baseURL, resortID)
	
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

// convertRestEntity converts a single LiveDataEntity from the REST response
// into our Entity format. Returns (entity, false) for entities that should be
// skipped (e.g., non-ATTRACTION types).
func convertRestEntity(restEntity LiveDataEntity) (Entity, bool) {
	if restEntity.EntityType != "ATTRACTION" {
		return Entity{}, false
	}

	lastUpdated, err := time.Parse(time.RFC3339, restEntity.LastUpdated)
	if err != nil {
		log.Printf("Warning: Could not parse lastUpdated for entity %s: %v", restEntity.ID, err)
		lastUpdated = time.Now()
	}

	waitTime := 0
	if restEntity.Queue != nil {
		if standby, exists := restEntity.Queue["STANDBY"]; exists && standby.WaitTime != nil {
			waitTime = *standby.WaitTime
		}
	}

	return Entity{
		EntityID:           restEntity.ID,
		Name:               restEntity.Name,
		EntityType:         restEntity.EntityType,
		ParkID:             restEntity.ParkID,
		WaitTime:           waitTime,
		Status:             EntityStatus(restEntity.Status),
		LastStatusChange:   lastUpdated,
		LastWaitTimeChange: lastUpdated,
	}, true
}