package main

import (
	"log"
	"time"
)

func convertLiveDataEntity(e LiveDataEntity, frameTs int64) (Attraction, bool) {
	if e.EntityType != "ATTRACTION" {
		return Attraction{}, false
	}

	lastUpdated, err := time.Parse(time.RFC3339, e.LastUpdated)
	if err != nil {
		if frameTs > 0 {
			lastUpdated = time.UnixMilli(frameTs)
		} else {
			log.Printf("warning: could not parse lastUpdated for %s (%s): %v", e.Name, e.ID, err)
			lastUpdated = time.Now()
		}
	}

	waitTime := 0
	waitTimeReported := false
	if e.Queue != nil {
		if standby, exists := e.Queue["STANDBY"]; exists && standby.WaitTime != nil {
			waitTime = *standby.WaitTime
			waitTimeReported = true
		}
	}

	return Attraction{
		ID:               e.ID,
		Name:             e.Name,
		Status:           e.Status,
		WaitTime:         waitTime,
		WaitTimeReported: waitTimeReported,
		LastUpdated:      lastUpdated,
	}, true
}
