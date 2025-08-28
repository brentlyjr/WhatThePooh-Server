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
	app.Post("/api/devices/:token/enable-notifications", enableNotificationsHandler)

	// Subscription routes
	app.Post("/api/update-ride-subscriptions", updateRideSubscriptionsHandler)
	app.Post("/api/disable-subscriptions", disableSubscriptionsHandler)



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

// registerDeviceHandler handles device registration with optional subscriptions
func registerDeviceHandler(c *fiber.Ctx) error {
	var req struct {
		DeviceRegistration
		Subscriptions []NotificationSubscription `json:"subscriptions,omitempty"`
	}
	
	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Set default environment if not provided
	if req.Environment == "" {
		req.Environment = "development"
	}

	log.Printf("Received device registration: DeviceToken=%s, AppVersion=%s, Environment=%s, NotificationsOn=%t, Subscriptions=%d, LastUpdated=%v",
		req.DeviceToken, req.AppVersion, req.Environment, req.NotificationsOn, len(req.Subscriptions), req.LastUpdated)

	if req.DeviceToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Device token is required",
		})
	}

	// Validate environment
	if req.Environment != "development" && req.Environment != "production" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Environment must be 'development' or 'production'",
		})
	}

	// Store the device (this will automatically re-enable notifications if the device was previously disabled)
	if err := db.StoreDeviceToken(req.DeviceRegistration); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// If subscriptions provided, set them
	if len(req.Subscriptions) > 0 {
		now := time.Now().UTC()
		for i := range req.Subscriptions {
			req.Subscriptions[i].DeviceToken = req.DeviceToken
			req.Subscriptions[i].Timestamp = now
		}
		
		if err := db.UpdateSubscriptions(req.DeviceToken, req.Subscriptions); err != nil {
			log.Printf("Failed to set initial subscriptions for device %s: %v", req.DeviceToken, err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Device registered but failed to set subscriptions",
			})
		}
	}

	return c.JSON(fiber.Map{
		"status": "Device registered successfully",
		"subscriptionsCount": len(req.Subscriptions),
	})
}

// disableSubscriptionsHandler disables notifications for a device
func disableSubscriptionsHandler(c *fiber.Ctx) error {
	var req struct {
		DeviceToken string `json:"deviceToken"`
	}
	
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

	// Check if device exists
	device, err := db.GetDeviceToken(req.DeviceToken)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to verify device",
		})
	}

	if device == nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": "Device not found",
		})
	}

	// Disable notifications (keep subscriptions intact)
	if err := db.SetDeviceNotificationState(req.DeviceToken, false); err != nil {
		log.Printf("Failed to disable notifications for device %s: %v", req.DeviceToken, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to disable notifications",
		})
	}

	return c.JSON(fiber.Map{
		"status": "Notifications disabled successfully",
		"deviceToken": req.DeviceToken,
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

// enableNotificationsHandler enables notifications for a device
func enableNotificationsHandler(c *fiber.Ctx) error {
	token := c.Params("token")
	
	// Check if device exists
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
	
	// Enable notifications
	if err := db.SetDeviceNotificationState(token, true); err != nil {
		log.Printf("Failed to enable notifications for device %s: %v", token, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to enable notifications",
		})
	}
	
	return c.JSON(fiber.Map{
		"status": "Notifications enabled successfully",
		"deviceToken": token,
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

	// Set timestamps for all subscriptions
	now := time.Now().UTC()
	for i := range updateData.Subscriptions {
		updateData.Subscriptions[i].DeviceToken = updateData.DeviceToken
		updateData.Subscriptions[i].Timestamp = now
	}

	// Update subscriptions using smart diffing
	if err := db.UpdateSubscriptions(updateData.DeviceToken, updateData.Subscriptions); err != nil {
		log.Printf("Failed to update subscriptions for device %s: %v", updateData.DeviceToken, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "Failed to update subscriptions",
		})
	}

	// Log successful update
	totalSubscriptions := len(updateData.Subscriptions)
	log.Printf("Successfully updated subscriptions for device %s: %d total subscriptions", 
		updateData.DeviceToken, totalSubscriptions)

	return c.JSON(fiber.Map{
		"status": "Subscriptions updated successfully",
		"totalSubscriptions": totalSubscriptions,
		"timestamp": now,
	})
}