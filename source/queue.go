package main

import (
	"log"
	"sync/atomic"
	"time"
)

type PushRequest struct {
	DeviceToken    string
	Message        string
	EntityID       string
	EntityName     string
	ParkID         string
	OldStatus      string
	NewStatus      string
	OldWaitTime    int
	NewWaitTime    int
	Environment    string // "development" or "production"
	NotificationID string
	Timestamp      time.Time // API-reported event time (entity lastUpdated)
	TimeOfLastStatus time.Time
}

// EntityQueue is a buffered channel for entity updates. Sized to absorb a full
// reconnect snapshot burst (~2-3k attraction entities across all resorts)
// plus live-update headroom.
var EntityQueue = make(chan Entity, 5000)

// entityQueueDrops counts updates dropped because EntityQueue was full,
// exposed via /api/metrics.
var entityQueueDrops atomic.Uint64

// PushQueue is for push notifications
var PushQueue = make(chan PushRequest, 100)

func Push(req PushRequest) {
	select {
	case PushQueue <- req:
	case <-shutdownDone:
		log.Printf("[SHUTDOWN] Dropping push: %s", req.EntityName)
	default:
		log.Printf("Push queue full, dropping notification for %s", req.EntityName)
	}
}

// QueueEntity adds an entity to the processing queue
func QueueEntity(entity Entity) {
	select {
	case EntityQueue <- entity:
		// Entity queued successfully
	default:
		// Queue is full, log and drop
		entityQueueDrops.Add(1)
		log.Printf("Entity queue full, dropping update for %s", entity.Name)
	}
}
