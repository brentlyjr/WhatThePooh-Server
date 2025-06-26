package main

import (
	"log"
	"runtime"
	"time"

	"github.com/gofiber/fiber/v2"
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

	// Metrics
	app.Get("/api/metrics", metricsHandler(entityManager, wsClient))

	// Test routes
	app.Post("/api/test/status-change", testStatusChangeHandler)
	app.Post("/api/test/status-change-custom", testStatusChangeCustomHandler)
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

	log.Printf("Received device registration: DeviceToken=%s, AppVersion=%s, DeviceType=%s, LastUpdated=%v",
		registration.DeviceToken, registration.AppVersion, registration.DeviceType, registration.LastUpdated)

	if registration.DeviceToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Device token is required",
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
	if err := db.DeleteDeviceToken(token); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"status": "Device deleted successfully",
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

		return c.JSON(fiber.Map{
			"queue_length":   len(EntityQueue),
			"entity_count":   len(entityManager.GetAllEntities()),
			"device_count":   deviceCount,
			"goroutines":     runtime.NumGoroutine(),
			"restarts":       GetReconnectionTimestamps(),
			"events":         wsClient.GetEventStats(),
			"statuses":       wsClient.GetStatusStats(),
			"server_start":   serverStartTime,
		})
	}
}

// testStatusChangeHandler simulates a status change
func testStatusChangeHandler(c *fiber.Ctx) error {
	msg := StatusChangeMessage{
		EntityID:  "f0d4b531-e291-471b-9527-00410c2bbd65",
		ParkID:    "ca888437-ebb4-4d50-aed2-d227f7096968",
		OldStatus: "DOWN",
		NewStatus: "OPERATING",
		Timestamp: time.Now(),
	}

	messageBus.PublishStatus(msg)

	return c.JSON(fiber.Map{
		"status":    "Test status change published",
		"message":   msg,
		"timestamp": time.Now(),
	})
}

// testStatusChangeCustomHandler simulates a custom status change
func testStatusChangeCustomHandler(c *fiber.Ctx) error {
	var testData struct {
		EntityID  string `json:"entityId"`
		ParkID    string `json:"parkId"`
		OldStatus string `json:"oldStatus"`
		NewStatus string `json:"newStatus"`
	}

	if err := c.BodyParser(&testData); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	msg := StatusChangeMessage{
		EntityID:  testData.EntityID,
		ParkID:    testData.ParkID,
		OldStatus: EntityStatus(testData.OldStatus),
		NewStatus: EntityStatus(testData.NewStatus),
		Timestamp: time.Now(),
	}

	messageBus.PublishStatus(msg)

	return c.JSON(fiber.Map{
		"status":    "Custom test status change published",
		"message":   msg,
		"timestamp": time.Now(),
	})
} 