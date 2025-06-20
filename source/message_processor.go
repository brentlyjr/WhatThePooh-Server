package main

import (
	"fmt"
	"log"
)

// StartMessageProcessors subscribes to the message bus and processes incoming messages.
func StartMessageProcessors() {
	log.Printf("Starting message processors...")

	// Goroutine for handling status changes (Fan-Out Processor)
	go func() {
		statusCh := messageBus.SubscribeStatus()
		for msg := range statusCh {
			log.Printf("🔔 STATUS CHANGE: Entity %s changed from %s to %s", msg.EntityID, msg.OldStatus, msg.NewStatus)

			// 1. Get all registered devices.
			// In a future state, this would get devices subscribed to this specific entity.
			devices, err := db.GetAllDevices()
			if err != nil {
				log.Printf("Error getting devices for fan-out: %v", err)
				continue
			}

			if len(devices) == 0 {
				log.Printf("FAN-OUT: No devices found for entity %s", msg.EntityID)
				continue
			}

			log.Printf("FAN-OUT: Found %d devices. Enqueuing APNs jobs...", len(devices))

			// 2. Create and enqueue a push notification for each device.
			notificationMsg := fmt.Sprintf("%s: %s -> %s", msg.EntityID, msg.OldStatus, msg.NewStatus)
			for _, device := range devices {
				pushReq := PushRequest{
					DeviceToken: device.DeviceToken,
					Message:     notificationMsg,
					EntityID:    msg.EntityID,
					ParkID:      msg.ParkID,
					OldStatus:   string(msg.OldStatus),
					NewStatus:   string(msg.NewStatus),
					OldWaitTime: msg.OldWaitTime,
					NewWaitTime: msg.NewWaitTime,
				}
				// Use the non-blocking Push function
				Push(pushReq)
			}
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