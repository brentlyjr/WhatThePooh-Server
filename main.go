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

// getEnvOrExit returns the value of the environment variable or exits if it's not set
func getEnvOrExit(key string) string {
	value := os.Getenv(key)
	if value == "" {
		log.Fatalf("Required environment variable %s is not set", key)
	}
	return value
}

// getEnvWithDefault returns the value of the environment variable or the default value if not set
func getEnvWithDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

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
		AuthKeyPath: getEnvOrExit("APNS_KEY_PATH"),
		KeyID:       getEnvOrExit("APNS_KEY_ID"),
		TeamID:      getEnvOrExit("APNS_TEAM_ID"),
		BundleID:    getEnvOrExit("APNS_BUNDLE_ID"),
		IsDev:       os.Getenv("APNS_ENV") == "development",
	}

	// If APNS_KEY_PATH is not set, try to find the key file
	if apnsConfig.AuthKeyPath == "" {
		// First try the container path
		containerPath := "/app/keys/AuthKey_MU2W4LLRSY.p8"
		if _, err := os.Stat(containerPath); err == nil {
			log.Printf("Found APNS key at container path: %s", containerPath)
			apnsConfig.AuthKeyPath = containerPath
		} else {
			log.Printf("APNS key not found at container path: %s (error: %v)", containerPath, err)
			// Fall back to local path
			localPath := "keys/AuthKey_MU2W4LLRSY.p8"
			if _, err := os.Stat(localPath); err == nil {
				log.Printf("Found APNS key at local path: %s", localPath)
				apnsConfig.AuthKeyPath = localPath
			} else {
				log.Printf("APNS key not found at local path: %s (error: %v)", localPath, err)
				// List current directory contents for debugging
				if files, err := os.ReadDir("."); err == nil {
					log.Printf("Current directory contents:")
					for _, file := range files {
						log.Printf("- %s", file.Name())
					}
				}
				// List /app/keys directory contents for debugging (if in container)
				if _, err := os.Stat("/app/keys"); err == nil {
					if files, err := os.ReadDir("/app/keys"); err == nil {
						log.Printf("/app/keys directory contents:")
						for _, file := range files {
							log.Printf("- %s", file.Name())
						}
					}
				} else {
					log.Printf("Directory /app/keys not found, likely not in container.")
				}
			}
		}
	}

	log.Printf("Using APNS key path: %s", apnsConfig.AuthKeyPath)

	if err := InitializeAPNS(apnsConfig); err != nil {
		log.Fatal("Failed to initialize APNS:", err)
	}

	// Get WebSocket URL and API key from environment variables
	websocketURL := getEnvWithDefault("WEBSOCKET_URL", "wss://api.themeparks.wiki/v1/entity/live")
	apiKey := getEnvOrExit("THEMEPARK_API_KEY")

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
