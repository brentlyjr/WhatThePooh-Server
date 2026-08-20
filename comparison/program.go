package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Program struct {
	resortID     string
	websocketURL string
	restURL      string
	apiKey       string
	httpClient   *http.Client

	mu               sync.Mutex
	wsState          map[string]Attraction
	restState        map[string]Attraction
	pending          map[string]*pendingFields
	wsBaselineDone   bool
	restBaselineDone bool
	matchedTotal     int
	unmatchedTotal   int
	divergedTotal    int
	supersededTotal  int
	snapshotDebounce *time.Timer

	writeMu   sync.Mutex
	conn      *websocket.Conn
	heartbeat time.Duration
}

func NewProgram(apiKey, websocketURL, restURL string) *Program {
	if websocketURL == "" {
		websocketURL = defaultWebSocketURL
	}
	if restURL == "" {
		restURL = defaultRESTURL
	}
	return &Program{
		resortID:     disneylandResortID,
		websocketURL: websocketURL,
		restURL:      restURL,
		apiKey:       apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		wsState:   make(map[string]Attraction),
		restState: make(map[string]Attraction),
		pending:   make(map[string]*pendingFields),
		heartbeat: 30 * time.Second,
	}
}

func (p *Program) Run(ctx context.Context) {
	log.Printf("starting comparison for Disneyland Resort (%s)", p.resortID)
	log.Printf("websocket %s (subprotocol=preview)", p.websocketURL)
	log.Printf("REST poll every %s: %s/%s/live?entityType=ATTRACTION", pollInterval, p.restURL, p.resortID)

	go p.runWebSocket(ctx)
	go p.runPolling(ctx)

	<-ctx.Done()
	p.dumpShutdown()
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
