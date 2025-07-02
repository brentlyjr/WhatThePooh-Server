package main

import (
	"log"
)

type PushRequest struct {
	DeviceToken string
	Message     string
	EntityID    string
	ParkID      string
	OldStatus   string
	NewStatus   string
	OldWaitTime int
	NewWaitTime int
	Environment string // "development" or "production"
}

// EntityQueue is a buffered channel for entity updates
var EntityQueue = make(chan Entity, 1000)

// PushQueue is for push notifications
var PushQueue = make(chan PushRequest, 100)

func Push(req PushRequest) {
	PushQueue <- req
}

// QueueEntity adds an entity to the processing queue
func QueueEntity(entity Entity) {
	select {
	case EntityQueue <- entity:
		// Entity queued successfully
	default:
		// Queue is full, log and drop
		log.Printf("Entity queue full, dropping update for %s", entity.Name)
	}
}
