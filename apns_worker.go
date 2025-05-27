package main

import (
	"log"
	"time"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/payload"
	"github.com/sideshow/apns2/token"
)

func StartWorker() {
	authKey, err := token.AuthKeyFromFile("AuthKey_YOURKEYID.p8")
	if err != nil {
		log.Fatal("Failed to load APNs auth key:", err)
	}

	tkn := &token.Token{
		AuthKey: authKey,
		KeyID:   "YOUR_KEY_ID",
		TeamID:  "YOUR_TEAM_ID",
	}

	client := apns2.NewTokenClient(tkn).Development()
	topic := "com.yourcompany.yourapp"

	for req := range PushQueue {
		notification := &apns2.Notification{
			DeviceToken: req.DeviceToken,
			Topic:       topic,
			Payload:     payload.NewPayload().Alert(req.Message).Badge(1),
		}

		res, err := client.Push(notification)
		if err != nil {
			log.Println("Push error:", err)
		} else if res.Sent() {
			log.Println("Push sent to:", req.DeviceToken)
		} else {
			log.Println("Push failed:", res.Reason)
		}

		time.Sleep(500 * time.Millisecond) // optional throttle
	}
}
