package main

import (
	"log"
)

// StartMessageProcessors subscribes to the message bus and processes incoming messages.
func StartMessageProcessors() {
	log.Printf("Starting message processors...")

	// Goroutine for handling status changes
	go func() {
		statusCh := messageBus.SubscribeStatus()
		for msg := range statusCh {
			log.Printf("🔔 STATUS CHANGE: Entity %s changed from %s to %s at %v",
				msg.EntityID, msg.OldStatus, msg.NewStatus, msg.Timestamp)
		}
	}()

	// Goroutine for handling wait time changes
	go func() {
		waitTimeCh := messageBus.SubscribeWaitTime()
		for msg := range waitTimeCh {
			log.Printf("⏰ WAIT TIME CHANGE: Entity %s changed from %d to %d minutes at %v",
				msg.EntityID, msg.OldWaitTime, msg.NewWaitTime, msg.Timestamp)
		}
	}()
} 