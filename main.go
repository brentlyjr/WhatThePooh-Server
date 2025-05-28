package main

import (
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
)

var db Database

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found")
	}

	// Initialize SQLite database
	sqliteDB, err := NewSQLiteDB()
	if err != nil {
		log.Fatal("Failed to initialize SQLite database:", err)
	}

	// Initialize cached database
	db = NewCachedDB(sqliteDB)

	// Initialize APNS
	apnsConfig := APNSConfig{
		AuthKeyPath: os.Getenv("APNS_KEY_PATH"),
		KeyID:       os.Getenv("APNS_KEY_ID"),
		TeamID:      os.Getenv("APNS_TEAM_ID"),
		BundleID:    os.Getenv("APNS_BUNDLE_ID"),
		IsDev:       os.Getenv("APNS_ENV") == "development",
	}

	// If APNS_KEY_PATH is not set, use the default path
	if apnsConfig.AuthKeyPath == "" {
		apnsConfig.AuthKeyPath = "./AuthKey_MU2W4LLRSY.p8"
		log.Printf("Using default APNS key path: %s", apnsConfig.AuthKeyPath)
	}

	if err := InitializeAPNS(apnsConfig); err != nil {
		log.Fatal("Failed to initialize APNS:", err)
	}

	// Get WebSocket URL and API key from environment variables
	websocketURL := os.Getenv("WEBSOCKET_URL")
	if websocketURL == "" {
		websocketURL = "wss://api.themeparks.wiki/v1/entity/live"
	}
	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		log.Fatal("API_KEY environment variable is required")
	}

	// Initialize entity manager
	entityManager := NewEntityManager()

	// Start entity processing worker
	go func() {
		for entity := range EntityQueue {
			entityManager.ProcessEntity(entity)
		}
	}()

	// Initialize WebSocket client
	wsClient := NewWebSocketClient(websocketURL, apiKey)

	// Start WebSocket client
	go wsClient.Connect()

	// Create Fiber app
	app := fiber.New()

	// Health check endpoint
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "healthy",
		})
	})

	// Get all entities endpoint
	app.Get("/api/entities", func(c *fiber.Ctx) error {
		entities := entityManager.GetAllEntities()
		return c.JSON(entities)
	})

	// Get entity by ID endpoint
	app.Get("/api/entities/:id", func(c *fiber.Ctx) error {
		entityID := c.Params("id")
		entity, exists := entityManager.GetEntity(entityID)
		if !exists {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Entity not found",
			})
		}
		return c.JSON(entity)
	})

	// Metrics endpoint
	app.Get("/api/metrics", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"queue_length": len(EntityQueue),
			"entity_count": len(entityManager.GetAllEntities()),
			"goroutines":   runtime.NumGoroutine(),
		})
	})

	// Send push notification endpoint
	app.Post("/api/push", func(c *fiber.Ctx) error {
		var req NotificationRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		if req.DeviceToken == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Device token is required",
			})
		}

		if err := SendPushNotification(req); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}

		return c.JSON(fiber.Map{
			"status": "Notification sent successfully",
		})
	})

	// Register device token endpoint
	app.Post("/api/register-device", func(c *fiber.Ctx) error {
		var registration DeviceRegistration
		if err := c.BodyParser(&registration); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

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
	})

	// Get registered devices endpoint
	app.Get("/api/devices", func(c *fiber.Ctx) error {
		devices, err := db.GetAllDevices()
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(devices)
	})

	// Delete device endpoint
	app.Delete("/api/devices/:token", func(c *fiber.Ctx) error {
		token := c.Params("token")
		if err := db.DeleteDeviceToken(token); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		return c.JSON(fiber.Map{
			"status": "Device deleted successfully",
		})
	})

	// Start server in a goroutine
	go func() {
		log.Println("Server started on :8080")
		if err := app.Listen(":8080"); err != nil {
			log.Fatal(err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	// Cleanup
	wsClient.Close()
	log.Println("Shutting down...")
}
