package main

import (
    "fmt"
	"log"
	"runtime"
	"time"

	"github.com/gofiber/fiber/v2"
    "github.com/google/uuid"
)

// SetupRoutes configures all API routes
func SetupRoutes(app *fiber.App, entityManager *EntityManager, wsClient *WebSocketClient) {
	// Health check
	app.Get("/health", healthHandler)

	// Entity routes
	app.Get("/api/entities", getAllEntitiesHandler(entityManager))
	app.Get("/api/entities/:id", getEntityByIDHandler(entityManager))

	// Device routes
	app.Post("/api/register-device", registerDeviceHandler)
	app.Get("/api/devices", getAllDevicesHandler)
	app.Get("/api/devices/:token/exists", checkDeviceExistsHandler)
	app.Delete("/api/devices/:token", deleteDeviceHandler)

	// Subscription routes
	app.Post("/api/update-ride-subscriptions", updateRideSubscriptionsHandler)

	// APNS Message tracking
	app.Get("/api/apns-messages", getAPNSMessagesHandler)
	app.Post("/api/apns-receipt", apnsReceiptHandler(entityManager))
	app.Get("/api/apns-receipts", getAPNSReceiptsHandler)

	// Metrics
	app.Get("/api/metrics", metricsHandler(entityManager, wsClient))

    // APNS send route
    app.Post("/api/notifications/send", sendAPNSNotificationHandler)
}

// healthHandler handles health check requests
func healthHandler(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "healthy",
	})
}

// getAllEntitiesHandler returns all entities
func getAllEntitiesHandler(entityManager *EntityManager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		entities := entityManager.GetAllEntities()
		return c.JSON(entities)
	}
}

// getEntityByIDHandler returns a specific entity
func getEntityByIDHandler(entityManager *EntityManager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		entityID := c.Params("id")
		entity, exists := entityManager.GetEntity(entityID)
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Entity not found",
			})
		}
		return c.JSON(entity)
	}
}

// registerDeviceHandler handles device registration
func registerDeviceHandler(c *fiber.Ctx) error {
	var registration DeviceRegistration
	if err := c.BodyParser(&registration); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Set default environment if not provided
	if registration.Environment == "" {
		registration.Environment = "development"
	}

	log.Printf("Received device registration: DeviceToken=%s, AppVersion=%s, Environment=%s, LastUpdated=%v",
		registration.DeviceToken, registration.AppVersion, registration.Environment, registration.LastUpdated)

	if registration.DeviceToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Device token is required",
		})
	}

	// Validate environment
	if registration.Environment != "development" && registration.Environment != "production" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Environment must be 'development' or 'production'",
		})
	}

	if err := db.StoreDeviceToken(registration); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"status": "Device registered successfully",
	})
}

// getAllDevicesHandler returns all registered devices
func getAllDevicesHandler(c *fiber.Ctx) error {
	devices, err := db.GetAllDevices()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(devices)
}

// checkDeviceExistsHandler checks if a device exists
func checkDeviceExistsHandler(c *fiber.Ctx) error {
	token := c.Params("token")
	device, err := db.GetDeviceToken(token)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	if device == nil {
		return c.JSON(fiber.Map{
			"exists":  false,
			"message": "Device not found",
		})
	}

	return c.JSON(fiber.Map{
		"exists": true,
		"device": device,
	})
}

// deleteDeviceHandler deletes a device
func deleteDeviceHandler(c *fiber.Ctx) error {
	token := c.Params("token")
	
	// Check if device exists before attempting deletion
	device, err := db.GetDeviceToken(token)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Error checking device existence",
		})
	}
	
	if device == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "No device found with that token",
		})
	}
	
	// Attempt deletion
	if err := db.DeleteDeviceToken(token); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	
	return c.JSON(fiber.Map{
		"status": "Device deleted successfully",
	})
}

// getAPNSMessagesHandler returns recent APNS messages for debugging
func getAPNSMessagesHandler(c *fiber.Ctx) error {
	limit := 100 // Default limit
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit := c.QueryInt("limit", 100); parsedLimit > 0 && parsedLimit <= 1000 {
			limit = parsedLimit
		}
	}

	messages, err := db.GetAPNSMessages(limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"messages": messages,
		"count":    len(messages),
		"limit":    limit,
	})
}

// apnsReceiptHandler handles APNS receipt acknowledgments from clients
func apnsReceiptHandler(entityManager *EntityManager) fiber.Handler {
	return func(c *fiber.Ctx) error {
	var receiptData struct {
		DeviceToken    string    `json:"deviceToken"`
		ClientTime     time.Time `json:"clientTime"`
		EntityID       string    `json:"entityId"`
		ParkID         string    `json:"parkId"`
		OldStatus      string    `json:"oldStatus"`
		NewStatus      string    `json:"newStatus"`
		OldWaitTime    int       `json:"oldWaitTime"`
		NewWaitTime    int       `json:"newWaitTime"`
		NotificationID string    `json:"notificationId"`
	}

	if err := c.BodyParser(&receiptData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if receiptData.DeviceToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Device token is required",
		})
	}

	if receiptData.EntityID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Entity ID is required",
		})
	}

	// Create receipt record
	receipt := APNSReceipt{
		DeviceToken:    receiptData.DeviceToken,
		ClientTime:     receiptData.ClientTime,
		ServerTime:     time.Now().UTC(),
		EntityID:       receiptData.EntityID,
		ParkID:         receiptData.ParkID,
		OldStatus:      receiptData.OldStatus,
		NewStatus:      receiptData.NewStatus,
		OldWaitTime:    receiptData.OldWaitTime,
		NewWaitTime:    receiptData.NewWaitTime,
		NotificationID: receiptData.NotificationID,
	}

	// Store receipt in database
	if err := db.StoreAPNSReceipt(receipt); err != nil {
		log.Printf("Failed to store APNS receipt: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to store receipt",
		})
	}

	// Look up entity name for logging
	entityName := receiptData.EntityID
	if entity, exists := entityManager.GetEntity(receiptData.EntityID); exists {
		entityName = entity.Name
	}

	log.Printf("APNS receipt stored for device %s, entity %s", receiptData.DeviceToken, entityName)

	return c.JSON(fiber.Map{
		"status":  "Receipt acknowledged successfully",
		"receipt": receipt,
	})
	}
}

// getAPNSReceiptsHandler returns recent APNS receipts for debugging and monitoring
func getAPNSReceiptsHandler(c *fiber.Ctx) error {
	limit := 100 // Default limit
	if limitParam := c.Query("limit"); limitParam != "" {
		if parsedLimit := c.QueryInt("limit", 100); parsedLimit > 0 && parsedLimit <= 1000 {
			limit = parsedLimit
		}
	}

	receipts, err := db.GetAPNSReceipts(limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"receipts": receipts,
		"count":    len(receipts),
		"limit":    limit,
	})
}

// metricsHandler returns server metrics
func metricsHandler(entityManager *EntityManager, wsClient *WebSocketClient) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get device count
		devices, err := db.GetAllDevices()
		deviceCount := 0
		if err != nil {
			log.Printf("Error getting device count for metrics: %v", err)
		} else {
			deviceCount = len(devices)
		}

		// Get entity statistics
		entityStats := map[string]interface{}{
			"total_entities": len(entityManager.GetAllEntities()),
			"statuses":      make(map[string]int),
		}
		
		// Calculate entity statistics
		entities := entityManager.GetAllEntities()
		for _, entity := range entities {
			// Count by status
			status := string(entity.Status)
			entityStats["statuses"].(map[string]int)[status]++
		}

		return c.JSON(fiber.Map{
			"queue_length":   len(EntityQueue),
			"entity_count":   len(entityManager.GetAllEntities()),
			"entity_stats":   entityStats,
			"device_count":   deviceCount,
			"goroutines":     runtime.NumGoroutine(),
			"restarts":       GetReconnectionTimestamps(),
			"events":         wsClient.GetEventStats(),
			"statuses":       wsClient.GetStatusStats(),
			"apns_messages":  GetAPNSMessageStats(),
			"server_start":   serverStartTime,
		})
	}
}

// sendAPNSNotificationHandler sends a full APNS notification to a specific device token
func sendAPNSNotificationHandler(c *fiber.Ctx) error {
    var req NotificationRequest
    if err := c.BodyParser(&req); err != nil {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "error": "Invalid request body",
        })
    }

    if req.DeviceToken == "" {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "error": "deviceToken is required",
        })
    }

    // Validate token format
    if !ValidateDeviceToken(req.DeviceToken) {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "error": "invalid device token format",
        })
    }

    // Default environment: from DB if present, else development
    if req.Environment == "" {
        if device, err := db.GetDeviceToken(req.DeviceToken); err == nil && device != nil && device.Environment != "" {
            req.Environment = device.Environment
        } else {
            req.Environment = "development"
        }
    }

    // Validate environment value if provided
    if req.Environment != "development" && req.Environment != "production" {
        return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
            "error": "environment must be 'development' or 'production'",
        })
    }

    // Ensure timestamp
    if req.Timestamp.IsZero() {
        req.Timestamp = time.Now().UTC()
    }

    // Provide sensible defaults for title/message if not supplied
    if req.Title == "" {
        if req.ParkID != "" {
            req.Title = getParkName(req.ParkID)
        } else {
            req.Title = "Notification"
        }
    }
    if req.Message == "" {
        if req.EntityName != "" && req.NewStatus != "" {
            req.Message = fmt.Sprintf("%s is now %s", req.EntityName, req.NewStatus)
        } else {
            req.Message = "Test notification"
        }
    }

    // Ensure a notification ID so we can echo it back
    if req.NotificationID == "" {
        req.NotificationID = uuid.New().String()
    }

    if err := SendPushNotification(req); err != nil {
        return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
            "error":   "Failed to send APNS notification",
            "details": err.Error(),
        })
    }

    return c.JSON(fiber.Map{
        "status":          "sent",
        "deviceToken":     req.DeviceToken,
        "environment":     req.Environment,
        "notificationId":  req.NotificationID,
        "title":           req.Title,
        "message":         req.Message,
        "entityId":        req.EntityID,
        "entityName":      req.EntityName,
        "parkId":          req.ParkID,
        "oldStatus":       req.OldStatus,
        "newStatus":       req.NewStatus,
        "oldWaitTime":     req.OldWaitTime,
        "newWaitTime":     req.NewWaitTime,
        "timestamp":       req.Timestamp,
    })
}

// updateRideSubscriptionsHandler handles updating ride subscriptions for a device
func updateRideSubscriptionsHandler(c *fiber.Ctx) error {
	var updateData RideSubscriptionUpdate
	if err := c.BodyParser(&updateData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if updateData.DeviceToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Device token is required",
		})
	}

	if len(updateData.Subscriptions) == 0 {
		// Empty subscriptions array is valid - means unsubscribe from everything
		log.Printf("Device %s is unsubscribing from all notifications", updateData.DeviceToken)
	}

	// Verify device exists
	device, err := db.GetDeviceToken(updateData.DeviceToken)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to verify device token",
		})
	}
	if device == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Device token not found. Please register device first.",
		})
	}

	// Convert the grouped subscriptions into individual subscription records
	var subscriptions []NotificationSubscription
	now := time.Now().UTC()
	
	for _, parkSub := range updateData.Subscriptions {
		for _, entityID := range parkSub.EntityIDs {
			subscription := NotificationSubscription{
				DeviceToken: updateData.DeviceToken,
				EntityID:    entityID,
				ParkID:      parkSub.ParkID,
				Timestamp:   now,
			}
			subscriptions = append(subscriptions, subscription)
		}
	}

	// Update subscriptions using smart diffing
	if err := db.UpdateSubscriptions(updateData.DeviceToken, subscriptions); err != nil {
		log.Printf("Failed to update subscriptions for device %s: %v", updateData.DeviceToken, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update subscriptions",
		})
	}

	// Log successful update
	totalSubscriptions := len(subscriptions)
	log.Printf("Successfully updated subscriptions for device %s: %d total subscriptions across %d parks", 
		updateData.DeviceToken, totalSubscriptions, len(updateData.Subscriptions))

	return c.JSON(fiber.Map{
		"status": "Subscriptions updated successfully",
		"totalSubscriptions": totalSubscriptions,
		"parksCount": len(updateData.Subscriptions),
		"timestamp": now,
	})
} 