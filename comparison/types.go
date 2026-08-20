package main

import (
	"encoding/json"
	"strconv"
	"time"
)

const (
	disneylandResortID  = "bfc89fd6-314d-44b4-b89e-df1a89cf991e"
	defaultWebSocketURL = "wss://api.themeparks.wiki/v1/live"
	defaultRESTURL      = "https://api.themeparks.wiki/v1/entity"

	pollInterval     = time.Minute
	unmatchedPolls   = 5
	snapshotDebounce = 2 * time.Second

	defaultReadWait = 90 * time.Second
	welcomeTimeout  = 15 * time.Second
	minBackoff      = 1 * time.Second
	maxBackoff      = 30 * time.Second
)

type Attraction struct {
	ID               string
	Name             string
	Status           string
	WaitTime         int
	WaitTimeReported bool
	LastUpdated      time.Time
}

type pendingFields struct {
	status *pendingStatus
	wait   *pendingWait
}

type pendingStatus struct {
	value       string
	receivedAt  time.Time
	name        string
	pollsWaited int
}

type pendingWait struct {
	waitTime    int
	reported    bool
	receivedAt  time.Time
	name        string
	pollsWaited int
}

type ParkLiveDataResponse struct {
	ID         string           `json:"id"`
	Name       string           `json:"name"`
	EntityType string           `json:"entityType"`
	Timezone   string           `json:"timezone"`
	LiveData   []LiveDataEntity `json:"liveData"`
}

// LiveDataEntity is shared by REST /live and preview WebSocket snapshot/update frames.
type LiveDataEntity struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	EntityType     string               `json:"entityType"`
	ParkID         string               `json:"parkId"`
	DestinationID  string               `json:"destinationId"`
	ExternalID     string               `json:"externalId"`
	Status         string               `json:"status"`
	LastUpdated    string               `json:"lastUpdated"`
	Queue          map[string]QueueData `json:"queue,omitempty"`
	OperatingHours []OperatingHour      `json:"operatingHours,omitempty"`
}

type QueueData struct {
	WaitTime *int `json:"waitTime"`
}

type OperatingHour struct {
	Type      string `json:"type"`
	StartTime string `json:"startTime"`
	EndTime   string `json:"endTime"`
}

type previewFrame struct {
	Type    string          `json:"type"`
	Channel string          `json:"channel"`
	Seq     *int64          `json:"seq"`
	Ts      int64           `json:"ts"`
	Data    json.RawMessage `json:"data"`
}

type welcomeData struct {
	HeartbeatIntervalMs int `json:"heartbeatIntervalMs"`
}

type subscribeMsg struct {
	Type     string `json:"type"`
	Channel  string `json:"channel"`
	Filter   string `json:"filter,omitempty"`
	Snapshot bool   `json:"snapshot,omitempty"`
	ReqID    string `json:"reqId,omitempty"`
}

type pongMsg struct {
	Type string `json:"type"`
}

type errorData struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	ReqID     string `json:"reqId"`
}

func formatWait(waitTime int, reported bool) string {
	if !reported {
		return "none"
	}
	return strconv.Itoa(waitTime)
}

func waitEqual(aWait int, aReported bool, bWait int, bReported bool) bool {
	if aReported != bReported {
		return false
	}
	if !aReported {
		return true
	}
	return aWait == bWait
}

func formatLag(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Second {
		return d.Round(time.Millisecond).String() + " after"
	}
	return d.Round(time.Second).String() + " after"
}

func clock(t time.Time) string {
	return t.Format("15:04:05")
}

func lastUpdatedStamp(t time.Time) string {
	return t.Format("15:04:05")
}
