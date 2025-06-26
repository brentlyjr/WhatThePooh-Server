package main

import (
	"encoding/base64"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/joho/godotenv"
)

var db Database
var (
	reconnectionTimestamps []time.Time
	reconnectionMutex     sync.RWMutex
	serverStartTime       time.Time
)

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

// AddReconnectionTimestamp adds a new reconnection timestamp to the global array
func AddReconnectionTimestamp() {
	reconnectionMutex.Lock()
	defer reconnectionMutex.Unlock()
	
	// Add new timestamp
	reconnectionTimestamps = append(reconnectionTimestamps, time.Now())
	
	// Keep only the last 100 timestamps
	if len(reconnectionTimestamps) > 100 {
		reconnectionTimestamps = reconnectionTimestamps[len(reconnectionTimestamps)-100:]
	}
}

// GetReconnectionTimestamps returns a copy of the reconnection timestamps
func GetReconnectionTimestamps() []time.Time {
	reconnectionMutex.RLock()
	defer reconnectionMutex.RUnlock()
	
	// Return a copy of the timestamps
	timestamps := make([]time.Time, len(reconnectionTimestamps))
	copy(timestamps, reconnectionTimestamps)
	return timestamps
}

func main() {
	// Record server start time
	serverStartTime = time.Now()

	// Load .env file for local development.
	// In GCP, these variables are set in the environment directly.
	// godotenv.Load() will not return an error if the .env file doesn't exist.
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using environment variables from system")
	}

	// Initialize SQLite database
	sqliteDB, err := NewSQLiteDB()
	if err != nil {
		log.Fatal("Failed to initialize SQLite database:", err)
	}

	// Initialize cached database
	db = NewCachedDB(sqliteDB)

	// Decode the base64-encoded APNS key from the environment variable
	apnsKeyBase64 := getEnvOrExit("APNS_KEY_BASE64")
	apnsKeyBytes, err := base64.StdEncoding.DecodeString(apnsKeyBase64)
	if err != nil {
		log.Fatal("Failed to decode APNS_KEY_BASE64:", err)
	}

	// Initialize APNS
	apnsConfig := APNSConfig{
		AuthKeyBytes: apnsKeyBytes,
		KeyID:        getEnvOrExit("APNS_KEY_ID"),
		TeamID:       getEnvOrExit("APNS_TEAM_ID"),
		BundleID:     getEnvOrExit("APNS_BUNDLE_ID"),
		IsDev:        os.Getenv("APNS_ENV") == "development",
	}

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

	// Start message processors
	StartMessageProcessors()

	// Start the APNS worker pool
	StartAPNSWorkers(5) // Start 5 workers

	// Create Fiber app
	app := fiber.New()

	// Setup all routes using the handlers.go file
	SetupRoutes(app, entityManager, wsClient)

	// Start the server in a goroutine so it doesn't block
	go func() {
		log.Println("What the Pooh Server started on :8080")
		if err := app.Listen(":8080"); err != nil {
			log.Fatalf("Failed to start server: %v", err)
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
