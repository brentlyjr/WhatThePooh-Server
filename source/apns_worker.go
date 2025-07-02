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
	EntityID    string `json:"entityId"`
	ParkID      string `json:"parkId"`
	OldStatus   string `json:"oldStatus"`
	NewStatus   string `json:"newStatus"`
	OldWaitTime int    `json:"oldWaitTime"`
	NewWaitTime int    `json:"newWaitTime"`
	Environment string `json:"environment"` // "development" or "production"
}

var apnsClient *apns2.Client
var apnsDevClient *apns2.Client
var apnsProdClient *apns2.Client

// ValidateAPNSConfiguration logs detailed information about the APNS configuration
func ValidateAPNSConfiguration() {
	log.Printf("=== APNS Configuration Validation ===")
	log.Printf("Bundle ID: %s", os.Getenv("APNS_BUNDLE_ID"))
	log.Printf("APNS Environment: %s", os.Getenv("APNS_ENV"))
	log.Printf("APNS Key ID: %s", os.Getenv("APNS_KEY_ID"))
	log.Printf("APNS Team ID: %s", os.Getenv("APNS_TEAM_ID"))
	
	// Check if we're in development or production mode
	if apnsDevClient != nil {
		log.Printf("APNS Development Client: Initialized")
	} else {
		log.Printf("APNS Development Client: NOT INITIALIZED")
	}
	
	if apnsProdClient != nil {
		log.Printf("APNS Production Client: Initialized")
	} else {
		log.Printf("APNS Production Client: NOT INITIALIZED")
	}
	log.Printf("=====================================")
}

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

	// Initialize both development and production clients
	apnsDevClient = apns2.NewTokenClient(tkn).Development()
	apnsProdClient = apns2.NewTokenClient(tkn).Production()
	
	// Set the default client based on the environment variable for backward compatibility
	if config.IsDev {
		apnsClient = apnsDevClient
		log.Printf("APNS initialized with DEVELOPMENT as default")
	} else {
		apnsClient = apnsProdClient
		log.Printf("APNS initialized with PRODUCTION as default")
	}

	// Validate configuration after initialization
	ValidateAPNSConfiguration()

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

// getAPNSClient returns the appropriate APNS client based on the device environment
func getAPNSClient(environment string) *apns2.Client {
	switch environment {
	case "development":
		return apnsDevClient
	case "production":
		return apnsProdClient
	default:
		// Default to development for backward compatibility
		return apnsDevClient
	}
}

// TestDeviceTokenWithDetails sends a silent notification to verify the token is valid and logs detailed information
func TestDeviceTokenWithDetails(deviceToken string, environment string) error {
	log.Printf("=== Testing Device Token: %s (Environment: %s) ===", deviceToken, environment)
	
	// Validate token format first
	if !ValidateDeviceToken(deviceToken) {
		log.Printf("Token format validation failed")
		return fmt.Errorf("invalid device token format")
	}
	log.Printf("Token format validation passed")
	
	client := getAPNSClient(environment)
	
	notification := &apns2.Notification{
		DeviceToken: deviceToken,
		Topic:       os.Getenv("APNS_BUNDLE_ID"),
		Payload:     payload.NewPayload().ContentAvailable(),
	}

	// Log notification details
	log.Printf("Test Notification Details:")
	log.Printf("  - Device Token: %s", notification.DeviceToken)
	log.Printf("  - Topic: %s", notification.Topic)
	log.Printf("  - Payload: %s", notification.Payload)
	log.Printf("  - Priority: %d", notification.Priority)
	log.Printf("  - Environment: %s", environment)

	res, err := client.Push(notification)
	if err != nil {
		log.Printf("Push error: %v", err)
		return fmt.Errorf("failed to send test notification: %v", err)
	}

	log.Printf("APNS Response:")
	log.Printf("  - Status Code: %d", res.StatusCode)
	log.Printf("  - Reason: %s", res.Reason)
	log.Printf("  - ApnsID: %s", res.ApnsID)
	log.Printf("  - Sent: %t", res.Sent())

	if !res.Sent() {
		log.Printf("Test failed - Token is invalid")
		log.Printf("Error Details:")
		switch res.Reason {
		case apns2.ReasonBadDeviceToken:
			log.Printf("  - Bad Device Token: Token format is invalid or device is not registered")
		case apns2.ReasonUnregistered:
			log.Printf("  - Unregistered: Device token is no longer valid for the topic")
		case apns2.ReasonBadTopic:
			log.Printf("  - Bad Topic: Topic is invalid or not authorized")
		case apns2.ReasonTopicDisallowed:
			log.Printf("  - Topic Disallowed: Topic is not allowed for this app")
		default:
			log.Printf("  - Unknown Error: %s", res.Reason)
		}
		return fmt.Errorf("invalid token: %s (Status: %d)", res.Reason, res.StatusCode)
	}

	log.Printf("Test passed - Token is valid")
	log.Printf("================================")
	return nil
}

// TestDeviceToken sends a silent notification to verify the token is valid
func TestDeviceToken(deviceToken string, environment string) error {
	client := getAPNSClient(environment)
	
	notification := &apns2.Notification{
		DeviceToken: deviceToken,
		Topic:       os.Getenv("APNS_BUNDLE_ID"),
		Payload:     payload.NewPayload().ContentAvailable(),
	}

	res, err := client.Push(notification)
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

	// Set default environment if not specified
	if registration.Environment == "" {
		registration.Environment = "development"
	}

	// Test the token with a silent notification
	if err := TestDeviceToken(registration.DeviceToken, registration.Environment); err != nil {
		return fmt.Errorf("token validation failed: %v", err)
	}

	// Store the token in the database
	return db.StoreDeviceToken(registration)
}

func SendPushNotification(req NotificationRequest) error {
	// Get the appropriate APNS client based on the environment
	client := getAPNSClient(req.Environment)
	
	notification := &apns2.Notification{
		DeviceToken: req.DeviceToken,
		Topic:       os.Getenv("APNS_BUNDLE_ID"),
		Payload: payload.NewPayload().
			ContentAvailable().
			Badge(req.Badge).
			Custom("entityId", req.EntityID).
			Custom("parkId", req.ParkID).
			Custom("oldStatus", req.OldStatus).
			Custom("newStatus", req.NewStatus).
			Custom("oldWaitTime", req.OldWaitTime).
			Custom("newWaitTime", req.NewWaitTime),
	}

	// Create APNS message tracking record
	apnsMessage := APNSMessage{
		DeviceToken: req.DeviceToken,
		Timestamp:   time.Now().UTC(),
		EntityID:    req.EntityID,
		ParkID:      req.ParkID,
		OldStatus:   req.OldStatus,
		NewStatus:   req.NewStatus,
		OldWaitTime: req.OldWaitTime,
		NewWaitTime: req.NewWaitTime,
	}

	res, err := client.Push(notification)
	if err != nil {
		// Update tracking record for failed message
		apnsMessage.Success = false
		apnsMessage.ErrorReason = err.Error()
		
		// Store failed message in database
		if storeErr := db.StoreAPNSMessage(apnsMessage); storeErr != nil {
			log.Printf("Failed to store APNS message record: %v", storeErr)
		}
		return err
	}

	if !res.Sent() {
		// Enhanced logging with detailed APNS response information
		log.Printf("Push failed for token %s", req.DeviceToken)
		log.Printf("APNS Response Details:")
		log.Printf("  - Status Code: %d", res.StatusCode)
		log.Printf("  - Reason: %s", res.Reason)
		log.Printf("  - ApnsID: %s", res.ApnsID)
		log.Printf("  - Sent: %t", res.Sent())
		
		// Log specific error details based on the reason
		switch res.Reason {
		case apns2.ReasonBadDeviceToken:
			log.Printf("  - Error Type: Bad Device Token (Token format is invalid or device is not registered)")
		case apns2.ReasonUnregistered:
			log.Printf("  - Error Type: Unregistered (Device token is no longer valid for the topic)")
		case apns2.ReasonBadTopic:
			log.Printf("  - Error Type: Bad Topic (Topic is invalid or not authorized)")
		case apns2.ReasonTopicDisallowed:
			log.Printf("  - Error Type: Topic Disallowed (Topic is not allowed for this app)")
		case apns2.ReasonBadExpirationDate:
			log.Printf("  - Error Type: Bad Expiration Date (Expiration date is invalid)")
		case apns2.ReasonBadPriority:
			log.Printf("  - Error Type: Bad Priority (Priority value is invalid)")
		case apns2.ReasonMissingDeviceToken:
			log.Printf("  - Error Type: Missing Device Token (Device token is missing)")
		case apns2.ReasonMissingTopic:
			log.Printf("  - Error Type: Missing Topic (Topic is missing)")
		case apns2.ReasonTooManyRequests:
			log.Printf("  - Error Type: Too Many Requests (Rate limit exceeded)")
		case apns2.ReasonIdleTimeout:
			log.Printf("  - Error Type: Idle Timeout (Connection timed out)")
		case apns2.ReasonShutdown:
			log.Printf("  - Error Type: Shutdown (Server is shutting down)")
		case apns2.ReasonInternalServerError:
			log.Printf("  - Error Type: Internal Server Error (APNS server error)")
		case apns2.ReasonServiceUnavailable:
			log.Printf("  - Error Type: Service Unavailable (APNS service unavailable)")
		default:
			log.Printf("  - Error Type: Unknown (%s)", res.Reason)
		}
		
		// Update tracking record for failed message
		apnsMessage.Success = false
		apnsMessage.ErrorReason = res.Reason
		
		// Store failed message in database
		if storeErr := db.StoreAPNSMessage(apnsMessage); storeErr != nil {
			log.Printf("Failed to store APNS message record: %v", storeErr)
		}
		
		// If the token is invalid, remove it from the database
		if res.Reason == apns2.ReasonBadDeviceToken || res.Reason == apns2.ReasonUnregistered {
			log.Printf("Removing invalid device token: %s (Reason: %s, Status: %d)", req.DeviceToken, res.Reason, res.StatusCode)
			// It's good practice to handle the error from deletion
			if delErr := db.DeleteDeviceToken(req.DeviceToken); delErr != nil {
				log.Printf("Error removing device token %s: %v", req.DeviceToken, delErr)
			}
		}
		return fmt.Errorf("push failed: %s", res.Reason)
	}

	// Update tracking record for successful message
	apnsMessage.Success = true
	
	// Store successful message in database
	if storeErr := db.StoreAPNSMessage(apnsMessage); storeErr != nil {
		log.Printf("Failed to store APNS message record: %v", storeErr)
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

// logNotificationDetails logs detailed information about a notification for debugging
func logNotificationDetails(notification *apns2.Notification, workerID int) {
	log.Printf("[Worker %d] Notification Details:", workerID)
	log.Printf("[Worker %d]   - Device Token: %s", workerID, notification.DeviceToken)
	log.Printf("[Worker %d]   - Topic: %s", workerID, notification.Topic)
	log.Printf("[Worker %d]   - Priority: %d", workerID, notification.Priority)
	log.Printf("[Worker %d]   - Expiration: %v", workerID, notification.Expiration)
}

// apnsSender is a single worker that consumes from the PushQueue.
func apnsSender(id int) {
	log.Printf("APNS Sender Worker %d started", id)
	bundleID := os.Getenv("APNS_BUNDLE_ID")

	for req := range PushQueue {
		log.Printf("[Worker %d] Sending push to %s (Environment: %s)", id, req.DeviceToken, req.Environment)

		// Create the payload
		payload := payload.NewPayload().
			ContentAvailable().
			Badge(1).
			Custom("entityId", req.EntityID).
			Custom("parkId", req.ParkID).
			Custom("oldStatus", req.OldStatus).
			Custom("newStatus", req.NewStatus).
			Custom("oldWaitTime", req.OldWaitTime).
			Custom("newWaitTime", req.NewWaitTime)

		// Log the payload structure for debugging
		log.Printf("[Worker %d] APNS Payload Structure: {\"aps\":{\"content-available\":1,\"badge\":1},\"entityId\":\"%s\",\"parkId\":\"%s\",\"oldStatus\":\"%s\",\"newStatus\":\"%s\",\"oldWaitTime\":%d,\"newWaitTime\":%d}", 
			id, req.EntityID, req.ParkID, req.OldStatus, req.NewStatus, req.OldWaitTime, req.NewWaitTime)

		notification := &apns2.Notification{
			DeviceToken: req.DeviceToken,
			Topic:       bundleID,
			Payload:     payload,
		}

		// Log notification details for debugging
		logNotificationDetails(notification, id)

		// Get the appropriate APNS client based on the environment
		client := getAPNSClient(req.Environment)
		
		res, err := client.Push(notification)
		
		// Create APNS message tracking record
		apnsMessage := APNSMessage{
			DeviceToken: req.DeviceToken,
			Timestamp:   time.Now().UTC(),
			EntityID:    req.EntityID,
			ParkID:      req.ParkID,
			OldStatus:   req.OldStatus,
			NewStatus:   req.NewStatus,
			OldWaitTime: req.OldWaitTime,
			NewWaitTime: req.NewWaitTime,
		}

		if err != nil {
			log.Printf("[Worker %d] Push error for token %s: %v", id, req.DeviceToken, err)
			apnsMessage.Success = false
			apnsMessage.ErrorReason = err.Error()
			
			// Store failed message in database
			if storeErr := db.StoreAPNSMessage(apnsMessage); storeErr != nil {
				log.Printf("[Worker %d] Failed to store APNS message record: %v", id, storeErr)
			}
			continue
		}

		if res.Sent() {
			log.Printf("[Worker %d] Push sent successfully to %s", id, req.DeviceToken)
			apnsMessage.Success = true
			
			// Store successful message in database
			if storeErr := db.StoreAPNSMessage(apnsMessage); storeErr != nil {
				log.Printf("[Worker %d] Failed to store APNS message record: %v", id, storeErr)
			}
		} else {
			// Enhanced logging with detailed APNS response information
			log.Printf("[Worker %d] Push failed for token %s", id, req.DeviceToken)
			log.Printf("[Worker %d] APNS Response Details:", id)
			log.Printf("[Worker %d]   - Status Code: %d", id, res.StatusCode)
			log.Printf("[Worker %d]   - Reason: %s", id, res.Reason)
			log.Printf("[Worker %d]   - ApnsID: %s", id, res.ApnsID)
			log.Printf("[Worker %d]   - Sent: %t", id, res.Sent())
			
			// Log specific error details based on the reason
			switch res.Reason {
			case apns2.ReasonBadDeviceToken:
				log.Printf("[Worker %d]   - Error Type: Bad Device Token (Token format is invalid or device is not registered)", id)
			case apns2.ReasonUnregistered:
				log.Printf("[Worker %d]   - Error Type: Unregistered (Device token is no longer valid for the topic)", id)
			case apns2.ReasonBadTopic:
				log.Printf("[Worker %d]   - Error Type: Bad Topic (Topic is invalid or not authorized)", id)
			case apns2.ReasonTopicDisallowed:
				log.Printf("[Worker %d]   - Error Type: Topic Disallowed (Topic is not allowed for this app)", id)
			case apns2.ReasonBadExpirationDate:
				log.Printf("[Worker %d]   - Error Type: Bad Expiration Date (Expiration date is invalid)", id)
			case apns2.ReasonBadPriority:
				log.Printf("[Worker %d]   - Error Type: Bad Priority (Priority value is invalid)", id)
			case apns2.ReasonMissingDeviceToken:
				log.Printf("[Worker %d]   - Error Type: Missing Device Token (Device token is missing)", id)
			case apns2.ReasonMissingTopic:
				log.Printf("[Worker %d]   - Error Type: Missing Topic (Topic is missing)", id)
			case apns2.ReasonTooManyRequests:
				log.Printf("[Worker %d]   - Error Type: Too Many Requests (Rate limit exceeded)", id)
			case apns2.ReasonIdleTimeout:
				log.Printf("[Worker %d]   - Error Type: Idle Timeout (Connection timed out)", id)
			case apns2.ReasonShutdown:
				log.Printf("[Worker %d]   - Error Type: Shutdown (Server is shutting down)", id)
			case apns2.ReasonInternalServerError:
				log.Printf("[Worker %d]   - Error Type: Internal Server Error (APNS server error)", id)
			case apns2.ReasonServiceUnavailable:
				log.Printf("[Worker %d]   - Error Type: Service Unavailable (APNS service unavailable)", id)
			default:
				log.Printf("[Worker %d]   - Error Type: Unknown (%s)", id, res.Reason)
			}
			
			// Update tracking record for failed message
			apnsMessage.Success = false
			apnsMessage.ErrorReason = res.Reason
			
			// Store failed message in database
			if storeErr := db.StoreAPNSMessage(apnsMessage); storeErr != nil {
				log.Printf("[Worker %d] Failed to store APNS message record: %v", id, storeErr)
			}
			
			// If the token is invalid or unregistered, remove it from our database
			if res.Reason == apns2.ReasonBadDeviceToken || res.Reason == apns2.ReasonUnregistered {
				log.Printf("[Worker %d] Removing invalid device token: %s (Reason: %s, Status: %d)", id, req.DeviceToken, res.Reason, res.StatusCode)
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

// GetRecentAPNSMessages returns recent APNS messages for debugging and monitoring
func GetRecentAPNSMessages(limit int) ([]APNSMessage, error) {
	return db.GetAPNSMessages(limit)
}
