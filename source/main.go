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

	if err := loadRideEmojis(); err != nil {
		log.Fatalf("Failed to load ride emojis: %v", err)
	}
	log.Printf("Loaded %d ride emojis", len(rideEmojis))

	// Initialize Supabase database
	supabaseDB, err := NewSupabaseDB()
	if err != nil {
		log.Fatal("Failed to initialize Supabase database:", err)
	}

	// Initialize cached database
	db = NewCachedDB(supabaseDB)

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

	// Phase 1 — Load DB state.
	// EntityManager constructor hydrates em.entities from the entity_status table.
	entityManager := NewEntityManager(db)
	dbSnapshot := entityManager.GetAllEntities()
	log.Printf("[STARTUP] Loaded %d entity statuses from DB", len(dbSnapshot))

	// Phase 2 — Fetch the current REST snapshot and load it silently.
	// No messages are published yet; subscribers haven't attached to the bus.
	restClient := NewRestClient(apiKey)
	restEntities, err := restClient.FetchAllEntities()
	if err != nil {
		log.Printf("Warning: Failed to fetch entities from REST: %v", err)
	}
	entityManager.BulkLoad(restEntities)
	log.Printf("[STARTUP] Fetched %d entities from REST and loaded into memory", len(restEntities))

	// Phase 3 — Wire up the message bus subscribers and APNS workers BEFORE
	// reconciliation. This is the critical ordering: ReconcileAgainst publishes
	// StatusChangeMessages, and MessageBus drops messages when no subscriber is
	// attached.
	StartMessageProcessors()
	StartAPNSWorkers(5)

	// Phase 4 — Start the EntityQueue consumer for live WS updates.
	go func() {
		for entity := range EntityQueue {
			entityManager.ProcessEntity(entity)
		}
	}()

	// Phase 5 — Reconcile DB snapshot vs REST snapshot. Any entity whose status
	// changed while the server was offline gets a push fan-out here.
	entityManager.ReconcileAgainst(dbSnapshot)

	// Phase 6 — Connect the WebSocket for live updates.
	wsClient := NewWebSocketClient(websocketURL, apiKey, entityManager)
	go wsClient.Connect()

	// Phase 7 — Optional read-only reconciliation loop. When RECONCILE_INTERVAL
	// is set (e.g. "10m"), periodically diff the REST snapshot against the DB
	// and log any status discrepancies. Disabled when the env var is empty.
	if intervalStr := os.Getenv("RECONCILE_INTERVAL"); intervalStr != "" {
		interval, err := time.ParseDuration(intervalStr)
		if err != nil {
			log.Printf("Invalid RECONCILE_INTERVAL %q: %v — reconciler disabled", intervalStr, err)
		} else if interval <= 0 {
			log.Printf("RECONCILE_INTERVAL must be positive, got %s — reconciler disabled", interval)
		} else {
			go NewReconciler(restClient, db, interval).Start()
		}
	}

	// Create Fiber app with increased body size limit for feedback logs
	app := fiber.New(fiber.Config{
		BodyLimit: 10 * 1024 * 1024, // 10MB limit for large feedback logs
	})

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
