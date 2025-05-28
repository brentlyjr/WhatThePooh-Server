package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"time"

	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/payload"
	"github.com/sideshow/apns2/token"
)

type APNSConfig struct {
	AuthKeyPath string
	KeyID       string
	TeamID      string
	BundleID    string
	IsDev       bool
}

type NotificationRequest struct {
	DeviceToken string `json:"deviceToken"`
	Message     string `json:"message"`
	Title       string `json:"title"`
	Badge       int    `json:"badge"`
}

var apnsClient *apns2.Client

func InitializeAPNS(config APNSConfig) error {
	authKey, err := token.AuthKeyFromFile(config.AuthKeyPath)
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
		return fmt.Errorf("push failed: %s", res.Reason)
	}

	return nil
}

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

// GetRegisteredDevices returns all registered device tokens
func GetRegisteredDevices() ([]DeviceRegistration, error) {
	return db.GetAllDevices()
}
