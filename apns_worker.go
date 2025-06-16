package main

import (
	"fmt"
	"log"
	"os"
	"regexp"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/payload"
	"github.com/sideshow/apns2/token"
)

type APNSConfig struct {
	AuthKeyBytes []byte
	KeyID        string
	TeamID       string
	BundleID     string
	IsDev        bool
}

type NotificationRequest struct {
	DeviceToken string `json:"deviceToken"`
	Message     string `json:"message"`
	Title       string `json:"title"`
	Badge       int    `json:"badge"`
}

var apnsClient *apns2.Client

func InitializeAPNS(config APNSConfig) error {
	authKey, err := token.AuthKeyFromBytes(config.AuthKeyBytes)
	if err != nil {
		return err
	}

	tkn := &token.Token{
		AuthKey: authKey,
		KeyID:   config.KeyID,
		TeamID:  config.TeamID,
	}

	if config.IsDev {
		apnsClient = apns2.NewTokenClient(tkn).Development()
	} else {
		apnsClient = apns2.NewTokenClient(tkn).Production()
	}

	return nil
}

// ValidateDeviceToken checks if a token matches the expected format
func ValidateDeviceToken(token string) bool {
	// APNS device tokens are 64 characters long and contain only hexadecimal characters
	matched, err := regexp.MatchString(`^[0-9a-fA-F]{64}$`, token)
	if err != nil {
		return false
	}
	return matched
}

// TestDeviceToken sends a silent notification to verify the token is valid
func TestDeviceToken(deviceToken string) error {
	notification := &apns2.Notification{
		DeviceToken: deviceToken,
		Topic:       os.Getenv("APNS_BUNDLE_ID"),
		Payload:     payload.NewPayload().ContentAvailable(),
	}

	res, err := apnsClient.Push(notification)
	if err != nil {
		return fmt.Errorf("failed to send test notification: %v", err)
	}

	if !res.Sent() {
		return fmt.Errorf("invalid token: %s", res.Reason)
	}

	return nil
}

// RegisterDevice validates and stores a device token
func RegisterDevice(registration DeviceRegistration) error {
	// Validate token format
	if !ValidateDeviceToken(registration.DeviceToken) {
		return fmt.Errorf("invalid device token format")
	}

	// Test the token with a silent notification
	if err := TestDeviceToken(registration.DeviceToken); err != nil {
		return fmt.Errorf("token validation failed: %v", err)
	}

	// Store the token in the database
	return db.StoreDeviceToken(registration)
}

func SendPushNotification(req NotificationRequest) error {
	notification := &apns2.Notification{
		DeviceToken: req.DeviceToken,
		Topic:       os.Getenv("APNS_BUNDLE_ID"),
		Payload: payload.NewPayload().
			AlertTitle(req.Title).
			AlertBody(req.Message).
			Badge(req.Badge),
	}

	res, err := apnsClient.Push(notification)
	if err != nil {
		return err
	}

	if !res.Sent() {
		// If the token is invalid, remove it from the database
		if res.Reason == apns2.ReasonBadDeviceToken || res.Reason == apns2.ReasonUnregistered {
			log.Printf("Removing invalid device token: %s", req.DeviceToken)
			// It's good practice to handle the error from deletion
			if delErr := db.DeleteDeviceToken(req.DeviceToken); delErr != nil {
				log.Printf("Error removing device token %s: %v", req.DeviceToken, delErr)
			}
		}
		return fmt.Errorf("push failed: %s", res.Reason)
	}

	return nil
}

// StartAPNSWorkers starts a pool of workers to send push notifications.
func StartAPNSWorkers(numWorkers int) {
	log.Printf("Starting %d APNS worker(s)...", numWorkers)
	for i := 0; i < numWorkers; i++ {
		go apnsSender(i + 1)
	}
}

// apnsSender is a single worker that consumes from the PushQueue.
func apnsSender(id int) {
	log.Printf("APNS Sender Worker %d started", id)
	bundleID := os.Getenv("APNS_BUNDLE_ID")

	for req := range PushQueue {
		log.Printf("[Worker %d] Sending push to %s", id, req.DeviceToken)

		notification := &apns2.Notification{
			DeviceToken: req.DeviceToken,
			Topic:       bundleID,
			Payload:     payload.NewPayload().Alert(req.Message).Badge(1).MutableContent(),
		}

		res, err := apnsClient.Push(notification)
		if err != nil {
			log.Printf("[Worker %d] Push error for token %s: %v", id, req.DeviceToken, err)
			continue
		}

		if res.Sent() {
			log.Printf("[Worker %d] Push sent successfully to %s", id, req.DeviceToken)
		} else {
			log.Printf("[Worker %d] Push failed for token %s: %s", id, req.DeviceToken, res.Reason)
			// If the token is invalid or unregistered, remove it from our database
			if res.Reason == apns2.ReasonBadDeviceToken || res.Reason == apns2.ReasonUnregistered {
				log.Printf("[Worker %d] Removing invalid device token: %s", id, req.DeviceToken)
				if delErr := db.DeleteDeviceToken(req.DeviceToken); delErr != nil {
					log.Printf("[Worker %d] Error removing device token %s: %v", id, req.DeviceToken, delErr)
				}
			}
		}
	}
}

// GetRegisteredDevices returns all registered device tokens
func GetRegisteredDevices() ([]DeviceRegistration, error) {
	return db.GetAllDevices()
}
