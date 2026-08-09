package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Resort struct {
	ID   string
	Name string
}

var resorts = []Resort{
	// Disney Resorts
	{ID: "bfc89fd6-314d-44b4-b89e-df1a89cf991e", Name: "Disneyland Resort"},
	{ID: "e957da41-3552-4cf6-b636-5babc5cbc4e5", Name: "Walt Disney World® Resort"},
	{ID: "abcfffe7-01f2-4f92-ae61-5093346f5a68", Name: "Hong Kong Disneyland Parks"},
	{ID: "faff60df-c766-4470-8adb-dee78e813f42", Name: "Tokyo Disney Resort"},
	{ID: "6e1464ca-1e9b-49c3-8937-c5c6f6675057", Name: "Shanghai Disney Resort"},
	{ID: "e8d0207f-da8a-4048-bec8-117aa946b2c2", Name: "Disneyland Paris"},

	// Universal Resorts
	{ID: "9fc68f1c-3f5e-4f09-89f2-aab2cf1a0741", Name: "Universal Studios"},
	{ID: "89db5d43-c434-4097-b71f-f6869f495a22", Name: "Universal Orlando Resort"},
}

const (
	// defaultReadWait is the floor for the inbound-frame deadline. The server
	// announces its heartbeat interval in the welcome frame (30s); the actual
	// deadline is max(defaultReadWait, 3x heartbeat interval).
	defaultReadWait = 90 * time.Second

	// welcomeTimeout closes a connection that never delivers a welcome frame.
	welcomeTimeout = 15 * time.Second

	// bootstrapSnapshotDebounce marks a channel's snapshot complete once this
	// long has passed since its last snapshot frame (chunked snapshots stream
	// back-to-back, so an intra-channel gap this long means the snapshot ended).
	bootstrapSnapshotDebounce = 2 * time.Second

	// resyncCooldown rate-limits per-channel unsubscribe/resubscribe resyncs.
	resyncCooldown = 60 * time.Second

	// resyncAckTimeout escalates a stalled channel resync to a full reconnect.
	resyncAckTimeout = 15 * time.Second

	maxSubscribeRetries = 3

	minBackoff = 1 * time.Second
	maxBackoff = 30 * time.Second
)

// previewFrame is the envelope for every server -> client frame.
type previewFrame struct {
	Type    string          `json:"type"`
	Channel string          `json:"channel"`
	Seq     *int64          `json:"seq"`
	Ts      int64           `json:"ts"` // epoch ms, server send time
	Data    json.RawMessage `json:"data"`
}

type welcomeData struct {
	User                string `json:"user"`
	Protocol            string `json:"protocol"`
	HeartbeatIntervalMs int    `json:"heartbeatIntervalMs"`
	Limits              struct {
		MaxConnections   int  `json:"maxConnections"`
		MaxSubscriptions int  `json:"maxSubscriptions"`
		Firehose         bool `json:"firehose"`
	} `json:"limits"`
}

type subscribedData struct {
	SubscriptionID string `json:"subscriptionId"`
	EntityID       string `json:"entityId"`
	EntityType     string `json:"entityType"`
	Filter         string `json:"filter"`
	Name           string `json:"name"`
	ReqID          string `json:"reqId"`
}

type errorData struct {
	Code      int    `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
	ReqID     string `json:"reqId"`
}

// Client -> server frames.
type subscribeMsg struct {
	Type     string `json:"type"`
	Channel  string `json:"channel"`
	Filter   string `json:"filter,omitempty"`
	Snapshot bool   `json:"snapshot,omitempty"`
	ReqID    string `json:"reqId,omitempty"` // resort GUID, echoed in error frames
}

type unsubscribeMsg struct {
	Type string `json:"type"`
	Data struct {
		SubscriptionID string `json:"subscriptionId"`
	} `json:"data"`
}

type pongMsg struct {
	Type string `json:"type"`
}

// channelState tracks one resort subscription on the current connection.
type channelState struct {
	name           string
	subscribed     bool
	failed         bool // non-retryable subscribe error; running degraded
	subscriptionID string
	lastSeq        int64 // -1 = no data frame seen since (re)subscribe
	snapshotFrames int
	gapCount       int
	subRetries     int
	lastResync     time.Time
	resyncPending  bool
}

// ChannelStats is the exported per-channel view for /api/metrics.
type ChannelStats struct {
	Name           string `json:"name"`
	Subscribed     bool   `json:"subscribed"`
	Failed         bool   `json:"failed"`
	LastSeq        int64  `json:"last_seq"`
	SnapshotFrames int    `json:"snapshot_frames"`
}

// bootstrapState accumulates snapshot entities until every resort channel has
// delivered its full snapshot, then releases them to main.go exactly once.
type bootstrapState struct {
	mu        sync.Mutex
	active    bool
	entities  map[string]Entity    // accumulator, keyed by EntityID
	lastSnap  map[string]time.Time // per channel: last snapshot frame time
	complete  map[string]bool      // per channel
	startedAt time.Time
	duration  time.Duration
	done      chan []Entity
	once      sync.Once
}

type WebSocketClient struct {
	url             string
	apiKey          string
	conn            *websocket.Conn
	done            chan struct{}
	closeOnce       sync.Once
	lastMessageTime time.Time
	writeMu         sync.Mutex

	connectedAt  time.Time
	messageCount uint64

	// backoff is only touched from the Connect goroutine (handleFrame runs on
	// the same goroutine, inside the read loop).
	backoff      time.Duration
	welcomeTimer *time.Timer

	heartbeatMu       sync.RWMutex
	heartbeatInterval time.Duration

	channelsMu sync.Mutex
	channels   map[string]*channelState
	seqGaps    uint64
	resyncs    uint64

	bootstrap *bootstrapState

	lastCloseMu   sync.RWMutex
	lastCloseTime time.Time
	lastCloseCode int
	lastCloseText string

	// Message counters
	messageCounts struct {
		sync.RWMutex
		eventCounts   map[string]uint64
		statusCounts  map[EntityStatus]uint64
		loggedUnknown map[string]bool
	}
}

func NewWebSocketClient(url, apiKey string) *WebSocketClient {
	client := &WebSocketClient{
		url:             url,
		apiKey:          apiKey,
		done:            make(chan struct{}),
		lastMessageTime: time.Now(),
		backoff:         minBackoff,
		channels:        make(map[string]*channelState),
		bootstrap: &bootstrapState{
			active:   true,
			entities: make(map[string]Entity),
			lastSnap: make(map[string]time.Time),
			complete: make(map[string]bool),
			done:     make(chan []Entity, 1),
		},
	}
	for _, r := range resorts {
		client.channels[r.ID] = &channelState{name: r.Name, lastSeq: -1}
	}
	client.messageCounts.eventCounts = make(map[string]uint64)
	client.messageCounts.statusCounts = make(map[EntityStatus]uint64)
	client.messageCounts.loggedUnknown = make(map[string]bool)
	return client
}

func (c *WebSocketClient) incrementCounter(eventType string) {
	c.messageCounts.Lock()
	defer c.messageCounts.Unlock()
	c.messageCounts.eventCounts[eventType]++
}

func (c *WebSocketClient) incrementStatusCounter(status EntityStatus) {
	c.messageCounts.Lock()
	defer c.messageCounts.Unlock()
	c.messageCounts.statusCounts[status]++
}

func formatSince(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return time.Since(t).Round(time.Millisecond).String()
}

// readWait is the inbound-frame deadline: the server heartbeats every
// heartbeatInterval (announced in welcome), so 3 missed heartbeats — never
// less than defaultReadWait — means the connection is dead.
func (c *WebSocketClient) readWait() time.Duration {
	c.heartbeatMu.RLock()
	hb := c.heartbeatInterval
	c.heartbeatMu.RUnlock()
	if hb > 0 && 3*hb > defaultReadWait {
		return 3 * hb
	}
	return defaultReadWait
}

func (c *WebSocketClient) logDisconnectSnapshot(reason string, err error) {
	uptime := time.Since(c.connectedAt).Round(time.Second)
	sinceLastMsg := formatSince(c.lastMessageTime)

	hint := "none"
	if reason == "read-deadline-timeout" {
		hint = "no_inbound_within_" + c.readWait().String()
	}

	errStr := "none"
	if err != nil {
		errStr = err.Error()
	}

	c.lastCloseMu.RLock()
	closeTime := c.lastCloseTime
	closeCode := c.lastCloseCode
	closeText := c.lastCloseText
	c.lastCloseMu.RUnlock()

	closeFrame := "none"
	if !closeTime.IsZero() {
		closeFrame = fmt.Sprintf("code=%d text=%q at=%s", closeCode, closeText, closeTime.Format(time.RFC3339))
	}

	log.Printf("[WS] Connection lost: reason=%s error=%s uptime=%v messages=%d sinceLastMsg=%s serverCloseFrame=%s hint=%s",
		reason, errStr, uptime, c.messageCount, sinceLastMsg, closeFrame, hint)
}

func (c *WebSocketClient) isShuttingDown() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *WebSocketClient) Connect() {
	c.bootstrap.mu.Lock()
	if c.bootstrap.startedAt.IsZero() {
		c.bootstrap.startedAt = time.Now()
	}
	c.bootstrap.mu.Unlock()
	go c.bootstrapTicker()

	for {
		select {
		case <-c.done:
			return
		default:
		}

		headers := http.Header{
			"X-API-Key": {c.apiKey},
			"Origin":    {"https://themeparks.wiki"},
		}

		log.Printf("[WS] Connecting to %s (subprotocol=preview, key=%s…)", c.url, keyPrefix(c.apiKey))

		dialer := websocket.Dialer{
			HandshakeTimeout: 45 * time.Second,
			Subprotocols:     []string{"preview"},
		}

		conn, resp, err := dialer.Dial(c.url, headers)
		if err != nil {
			log.Printf("[WS] Failed to connect: %v", err)
			if resp != nil {
				log.Printf("[WS] Response status: %s", resp.Status)
			}
			if !c.sleepBackoff() {
				return
			}
			continue
		}

		if conn.Subprotocol() != "preview" {
			// The server serves the legacy shape when the preview subprotocol
			// isn't negotiated — parsing it as preview frames would silently
			// drop everything, so treat this as a failed connection.
			log.Printf("[WS] Server negotiated subprotocol %q instead of \"preview\" — closing", conn.Subprotocol())
			conn.Close()
			if !c.sleepBackoff() {
				return
			}
			continue
		}

		c.conn = conn
		c.connectedAt = time.Now()
		c.messageCount = 0
		c.lastCloseMu.Lock()
		c.lastCloseTime = time.Time{}
		c.lastCloseCode = 0
		c.lastCloseText = ""
		c.lastCloseMu.Unlock()

		c.resetChannelsForConnect()

		AddReconnectionTimestamp()
		log.Printf("[WS] Connected at %s (subprotocol=%s)", c.connectedAt.Format("2006-01-02 15:04:05 MST"), conn.Subprotocol())

		// Subscriptions are sent from the welcome handler; a connection that
		// never delivers a welcome frame is dead.
		c.welcomeTimer = time.AfterFunc(welcomeTimeout, func() {
			log.Printf("[WS] No welcome frame within %s — closing connection", welcomeTimeout)
			conn.Close()
		})

		c.conn.SetReadDeadline(time.Now().Add(c.readWait()))

		// WriteControl is internally synchronized and documented safe to call
		// concurrently with WriteMessage, so it deliberately skips writeMu
		// (gorilla's default ping handler does the same).
		c.conn.SetPingHandler(func(appData string) error {
			deadline := time.Now().Add(c.readWait())
			c.conn.SetReadDeadline(deadline)
			return c.conn.WriteControl(websocket.PongMessage, []byte(appData), deadline)
		})

		c.conn.SetCloseHandler(func(code int, text string) error {
			now := time.Now()
			c.lastCloseMu.Lock()
			c.lastCloseTime = now
			c.lastCloseCode = code
			c.lastCloseText = text
			c.lastCloseMu.Unlock()

			log.Printf("[WS] Server close frame: code=%d text=%q", code, text)
			return nil
		})

		for {
			_, message, err := c.conn.ReadMessage()
			if err != nil {
				if c.isShuttingDown() {
					c.logDisconnectSnapshot("shutdown", nil)
					return
				}
				reason := classifyDisconnect(err)
				c.logDisconnectSnapshot(reason, err)
				break
			}
			now := time.Now()
			c.lastMessageTime = now
			c.conn.SetReadDeadline(now.Add(c.readWait()))
			c.messageCount++
			c.handleFrame(message)
		}

		c.welcomeTimer.Stop()
		if c.isShuttingDown() {
			return
		}
		c.conn.Close()

		// Auth-flavored closes won't heal on a fast retry (the key may be
		// rotated live, so keep trying — just slowly).
		c.lastCloseMu.RLock()
		closeCode := c.lastCloseCode
		c.lastCloseMu.RUnlock()
		switch closeCode {
		case 3000, 3001, 3003:
			log.Printf("[WS] Authentication/permission close (code=%d) — backing off to %s", closeCode, maxBackoff)
			c.backoff = maxBackoff
		}

		if !c.sleepBackoff() {
			return
		}
	}
}

func keyPrefix(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return key[:4]
}

// sleepBackoff waits out the current backoff (with jitter), doubles it up to
// maxBackoff, and reports whether the client should keep running.
func (c *WebSocketClient) sleepBackoff() bool {
	wait := c.backoff + time.Duration(rand.Int63n(int64(c.backoff/2)+1))
	log.Printf("[WS] Reconnecting in %s...", wait.Round(time.Millisecond))
	select {
	case <-time.After(wait):
	case <-c.done:
		return false
	}
	c.backoff *= 2
	if c.backoff > maxBackoff {
		c.backoff = maxBackoff
	}
	return true
}

// resetChannelsForConnect clears per-connection channel state. During bootstrap
// it also re-opens every channel's completion gate — fresh full snapshots are
// coming — while keeping the accumulator (map overwrite is idempotent).
func (c *WebSocketClient) resetChannelsForConnect() {
	c.channelsMu.Lock()
	for _, cs := range c.channels {
		cs.subscribed = false
		cs.failed = false
		cs.subscriptionID = ""
		cs.lastSeq = -1
		cs.snapshotFrames = 0
		cs.gapCount = 0
		cs.subRetries = 0
		cs.resyncPending = false
	}
	c.channelsMu.Unlock()

	b := c.bootstrap
	b.mu.Lock()
	if b.active {
		b.complete = make(map[string]bool)
		b.lastSnap = make(map[string]time.Time)
	}
	b.mu.Unlock()
}

func (c *WebSocketClient) sendJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	conn := c.conn
	if conn == nil {
		return fmt.Errorf("not connected")
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

func (c *WebSocketClient) sendSubscribe(resortID string) {
	msg := subscribeMsg{
		Type:     "subscribe",
		Channel:  resortID,
		Filter:   "ATTRACTION",
		Snapshot: true,
		ReqID:    resortID,
	}
	if err := c.sendJSON(msg); err != nil {
		log.Printf("[WS] Failed to send subscribe for %s: %v", resortID, err)
	}
}

func (c *WebSocketClient) handleFrame(message []byte) {
	var f previewFrame
	if err := json.Unmarshal(message, &f); err != nil {
		log.Printf("[WS] Error parsing frame: %v", err)
		return
	}

	c.incrementCounter(f.Type)

	switch f.Type {
	case "welcome":
		c.handleWelcome(f)
	case "subscribed":
		c.handleSubscribed(f)
	case "unsubscribed":
		c.handleUnsubscribed(f)
	case "snapshot":
		c.handleSnapshot(f)
	case "update":
		c.handleUpdate(f)
	case "ping":
		if err := c.sendJSON(pongMsg{Type: "pong"}); err != nil {
			log.Printf("[WS] Failed to send pong: %v", err)
		}
	case "error":
		c.handleError(f)
	default:
		// Tolerant reader: unknown frame types are counted, logged once, ignored.
		c.messageCounts.Lock()
		seen := c.messageCounts.loggedUnknown[f.Type]
		c.messageCounts.loggedUnknown[f.Type] = true
		c.messageCounts.Unlock()
		if !seen {
			log.Printf("[WS] Ignoring unknown frame type %q (logged once)", f.Type)
		}
	}
}

func (c *WebSocketClient) handleWelcome(f previewFrame) {
	if c.welcomeTimer != nil {
		c.welcomeTimer.Stop()
	}
	c.backoff = minBackoff // healthy session

	var wd welcomeData
	if err := json.Unmarshal(f.Data, &wd); err != nil {
		log.Printf("[WS] Error parsing welcome data: %v", err)
	}
	if wd.HeartbeatIntervalMs > 0 {
		c.heartbeatMu.Lock()
		c.heartbeatInterval = time.Duration(wd.HeartbeatIntervalMs) * time.Millisecond
		c.heartbeatMu.Unlock()
	}
	log.Printf("[WS] Welcome: user=%s protocol=%s heartbeat=%dms limits={connections:%d subscriptions:%d firehose:%t}",
		wd.User, wd.Protocol, wd.HeartbeatIntervalMs,
		wd.Limits.MaxConnections, wd.Limits.MaxSubscriptions, wd.Limits.Firehose)

	maxSubs := wd.Limits.MaxSubscriptions
	if maxSubs > 0 && maxSubs < len(resorts) {
		log.Printf("[WS] WARNING: plan allows %d subscriptions but %d resorts are configured — subscribing to the first %d only",
			maxSubs, len(resorts), maxSubs)
	}

	for i, resort := range resorts {
		if maxSubs > 0 && i >= maxSubs {
			c.markChannelFailed(resort.ID, "over subscription limit")
			continue
		}
		c.sendSubscribe(resort.ID)
	}
}

func (c *WebSocketClient) handleSubscribed(f previewFrame) {
	var sd subscribedData
	if err := json.Unmarshal(f.Data, &sd); err != nil {
		log.Printf("[WS] Error parsing subscribed data: %v", err)
		return
	}

	c.channelsMu.Lock()
	cs := c.channelFor(sd.EntityID)
	cs.subscribed = true
	cs.failed = false
	cs.subscriptionID = sd.SubscriptionID
	cs.lastSeq = -1
	cs.snapshotFrames = 0
	cs.gapCount = 0
	cs.resyncPending = false
	c.channelsMu.Unlock()

	// A (re)subscribe means a fresh snapshot is coming: re-open this channel's
	// bootstrap gate so a half-received earlier snapshot can't complete it.
	b := c.bootstrap
	b.mu.Lock()
	if b.active {
		b.complete[sd.EntityID] = false
		delete(b.lastSnap, sd.EntityID)
	}
	b.mu.Unlock()

	log.Printf("[WS] Subscribed channel=%s name=%q subscriptionId=%s filter=%s", sd.EntityID, sd.Name, sd.SubscriptionID, sd.Filter)
}

func (c *WebSocketClient) handleUnsubscribed(f previewFrame) {
	c.channelsMu.Lock()
	cs, ok := c.channels[f.Channel]
	pending := ok && cs.resyncPending
	c.channelsMu.Unlock()

	log.Printf("[WS] Unsubscribed channel=%s (resyncPending=%t)", f.Channel, pending)
	if pending {
		// Resync flow: fresh snapshot self-heals whatever the gap lost.
		c.sendSubscribe(f.Channel)
	}
}

func (c *WebSocketClient) handleSnapshot(f previewFrame) {
	var items []LiveDataEntity
	if err := json.Unmarshal(f.Data, &items); err != nil {
		log.Printf("[WS] Error parsing snapshot data on channel %s: %v", f.Channel, err)
		return
	}

	c.trackSeq(f)
	c.channelsMu.Lock()
	c.channelFor(f.Channel).snapshotFrames++
	c.channelsMu.Unlock()

	entities := make([]Entity, 0, len(items))
	for _, item := range items {
		entity, ok := convertLiveDataEntity(item, f.Ts)
		if !ok {
			continue
		}
		c.incrementStatusCounter(entity.Status)
		entities = append(entities, entity)
	}

	b := c.bootstrap
	b.mu.Lock()
	if b.active {
		b.lastSnap[f.Channel] = time.Now()
		for _, e := range entities {
			b.entities[e.EntityID] = e
		}
		b.mu.Unlock()
		return
	}
	b.mu.Unlock()

	// Post-bootstrap (reconnect/resync) snapshots flow through the same path
	// as live updates: ProcessEntity no-ops on unchanged status, so only real
	// diffs notify.
	for _, e := range entities {
		QueueEntity(e)
	}
}

func (c *WebSocketClient) handleUpdate(f previewFrame) {
	var item LiveDataEntity
	if err := json.Unmarshal(f.Data, &item); err != nil {
		log.Printf("[WS] Error parsing update data on channel %s: %v", f.Channel, err)
		return
	}

	c.trackSeq(f)

	if entity, ok := convertLiveDataEntity(item, f.Ts); ok {
		c.incrementStatusCounter(entity.Status)
		QueueEntity(entity)
	}

	// An update after at least one snapshot frame on the same channel is an
	// authoritative end-of-snapshot signal (frames are in-order per channel).
	// A bare update with no snapshot yet must NOT complete the channel.
	b := c.bootstrap
	completed := false
	b.mu.Lock()
	if b.active {
		if _, sawSnapshot := b.lastSnap[f.Channel]; sawSnapshot && !b.complete[f.Channel] {
			b.complete[f.Channel] = true
			completed = true
		}
	}
	b.mu.Unlock()
	if completed {
		c.evaluateBootstrap()
	}
}

func (c *WebSocketClient) handleError(f previewFrame) {
	var ed errorData
	if err := json.Unmarshal(f.Data, &ed); err != nil {
		log.Printf("[WS] Error frame (unparseable data): channel=%s raw=%s", f.Channel, string(f.Data))
		return
	}
	log.Printf("[WS] Error frame: code=%d message=%q retryable=%t reqId=%s channel=%s", ed.Code, ed.Message, ed.Retryable, ed.ReqID, f.Channel)

	// Subscribe errors carry our reqId (the resort GUID).
	if ed.ReqID == "" {
		return
	}
	c.channelsMu.Lock()
	cs, known := c.channels[ed.ReqID]
	if !known {
		c.channelsMu.Unlock()
		return
	}
	if ed.Retryable && cs.subRetries < maxSubscribeRetries {
		cs.subRetries++
		delay := time.Duration(1<<cs.subRetries) * time.Second // 2s, 4s, 8s
		c.channelsMu.Unlock()
		resortID := ed.ReqID
		time.AfterFunc(delay, func() {
			if !c.isShuttingDown() {
				log.Printf("[WS] Retrying subscribe for %s after retryable error", resortID)
				c.sendSubscribe(resortID)
			}
		})
		return
	}
	c.channelsMu.Unlock()
	c.markChannelFailed(ed.ReqID, ed.Message)
}

// markChannelFailed puts a channel into degraded mode: no data will flow for
// it this connection, and the bootstrap gate counts it complete so startup
// can't hang on it.
func (c *WebSocketClient) markChannelFailed(resortID, reason string) {
	c.channelsMu.Lock()
	cs := c.channelFor(resortID)
	cs.failed = true
	name := cs.name
	c.channelsMu.Unlock()

	log.Printf("[WS] Channel %s (%q) marked failed: %s — continuing degraded", resortID, name, reason)

	b := c.bootstrap
	changed := false
	b.mu.Lock()
	if b.active && !b.complete[resortID] {
		b.complete[resortID] = true
		changed = true
	}
	b.mu.Unlock()
	if changed {
		c.evaluateBootstrap()
	}
}

// channelFor must be called with channelsMu held.
func (c *WebSocketClient) channelFor(id string) *channelState {
	cs, ok := c.channels[id]
	if !ok {
		cs = &channelState{name: id, lastSeq: -1}
		c.channels[id] = cs
	}
	return cs
}

// trackSeq maintains the per-channel cursor. seq is opaque and may skip values
// (docs: "don't assume it steps by one") — and on a filtered subscription,
// events filtered out server-side still consume seq numbers, so gaps carry no
// loss signal at all. Gaps only log and count; the resync escalation is
// reserved for a cursor that went backwards without our resubscribe, which
// means the server restarted the channel.
func (c *WebSocketClient) trackSeq(f previewFrame) {
	if f.Seq == nil || f.Channel == "" {
		return
	}
	seq := *f.Seq

	c.channelsMu.Lock()
	cs := c.channelFor(f.Channel)
	needResync := false
	if cs.lastSeq >= 0 {
		if seq > cs.lastSeq+1 {
			cs.gapCount++
			c.seqGaps++
//			log.Printf("[WS] Seq gap on channel %s (%q): %d -> %d (gap #%d this session)", f.Channel, cs.name, cs.lastSeq, seq, cs.gapCount)
		} else if seq <= cs.lastSeq {
			log.Printf("[WS] Seq went backwards on channel %s (%q): %d -> %d — server channel restart", f.Channel, cs.name, cs.lastSeq, seq)
			needResync = true
		}
	}
	cs.lastSeq = seq
	c.channelsMu.Unlock()

	if needResync {
		c.maybeResync(f.Channel)
	}
}

// maybeResync unsubscribes and resubscribes (snapshot:true) a single channel
// whose stream looks unhealthy. Rate-limited; a stalled resync escalates to a
// full reconnect, whose cold snapshot self-heals everything anyway.
func (c *WebSocketClient) maybeResync(channel string) {
	b := c.bootstrap
	b.mu.Lock()
	bootstrapActive := b.active
	b.mu.Unlock()
	if bootstrapActive {
		return
	}

	c.channelsMu.Lock()
	cs := c.channelFor(channel)
	if cs.resyncPending || time.Since(cs.lastResync) < resyncCooldown || cs.subscriptionID == "" {
		c.channelsMu.Unlock()
		return
	}
	cs.lastResync = time.Now()
	cs.gapCount = 0
	cs.resyncPending = true
	subID := cs.subscriptionID
	name := cs.name
	c.resyncs++
	c.channelsMu.Unlock()

	log.Printf("[WS] Resyncing channel %s (%q): unsubscribing %s, will resubscribe with snapshot", channel, name, subID)

	msg := unsubscribeMsg{Type: "unsubscribe"}
	msg.Data.SubscriptionID = subID
	if err := c.sendJSON(msg); err != nil {
		log.Printf("[WS] Failed to send unsubscribe for %s: %v", channel, err)
	}

	time.AfterFunc(resyncAckTimeout, func() {
		c.channelsMu.Lock()
		stalled := c.channels[channel] != nil && c.channels[channel].resyncPending
		c.channelsMu.Unlock()
		if stalled && !c.isShuttingDown() {
			log.Printf("[WS] Resync of channel %s stalled for %s — forcing full reconnect", channel, resyncAckTimeout)
			if conn := c.conn; conn != nil {
				conn.Close()
			}
		}
	})
}

// --- Bootstrap gate ---

// bootstrapTicker periodically re-evaluates the gate: the snapshot-debounce
// completion condition is time-driven, so frame arrival alone can't fire it.
func (c *WebSocketClient) bootstrapTicker() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			b := c.bootstrap
			b.mu.Lock()
			active := b.active
			b.mu.Unlock()
			if !active {
				return
			}
			c.evaluateBootstrap()
		}
	}
}

// evaluateBootstrap checks every resort channel for completion — an update
// after its snapshot, a debounce-expired snapshot, or a failed subscribe —
// and fires the gate exactly once when all are done.
func (c *WebSocketClient) evaluateBootstrap() {
	b := c.bootstrap
	b.mu.Lock()
	if !b.active {
		b.mu.Unlock()
		return
	}

	now := time.Now()
	allDone := true
	for _, r := range resorts {
		if b.complete[r.ID] {
			continue
		}
		if last, ok := b.lastSnap[r.ID]; ok && now.Sub(last) >= bootstrapSnapshotDebounce {
			b.complete[r.ID] = true
			continue
		}
		allDone = false
	}
	if !allDone {
		b.mu.Unlock()
		return
	}

	b.active = false
	b.duration = time.Since(b.startedAt)
	entities := make([]Entity, 0, len(b.entities))
	for _, e := range b.entities {
		entities = append(entities, e)
	}
	duration := b.duration
	b.mu.Unlock()

	healthy := 0
	c.channelsMu.Lock()
	for _, r := range resorts {
		if cs, ok := c.channels[r.ID]; ok && !cs.failed && cs.snapshotFrames > 0 {
			healthy++
		}
	}
	c.channelsMu.Unlock()

	b.once.Do(func() {
		log.Printf("[STARTUP] WS bootstrap complete: %d entities from %d/%d channels in %s",
			len(entities), healthy, len(resorts), duration.Round(time.Millisecond))
		b.done <- entities
	})
}

// BootstrapDone delivers the accumulated snapshot entities exactly once, when
// every resort channel has completed its initial snapshot.
func (c *WebSocketClient) BootstrapDone() <-chan []Entity {
	return c.bootstrap.done
}

// ForceBootstrapComplete ends the bootstrap phase immediately (startup timeout
// path) and returns whatever accumulated. Idempotent with the normal gate.
func (c *WebSocketClient) ForceBootstrapComplete() []Entity {
	b := c.bootstrap
	b.mu.Lock()
	b.active = false
	if b.duration == 0 {
		b.duration = time.Since(b.startedAt)
	}
	entities := make([]Entity, 0, len(b.entities))
	for _, e := range b.entities {
		entities = append(entities, e)
	}
	b.mu.Unlock()

	// Consume the once so a late normal firing can't send a duplicate.
	b.once.Do(func() {})
	return entities
}

// --- Disconnect classification / shutdown / metrics ---

func classifyDisconnect(err error) string {
	if closeErr, ok := err.(*websocket.CloseError); ok {
		switch closeErr.Code {
		case 3000:
			return "server-close(code=3000 auth-failed)"
		case 3001:
			return "server-close(code=3001 query-string-credentials)"
		case 3003:
			return "server-close(code=3003 insufficient-permissions)"
		case 4029:
			return "server-close(code=4029 connection-cap)"
		default:
			return fmt.Sprintf("server-close(code=%d)", closeErr.Code)
		}
	}
	errStr := err.Error()
	if strings.Contains(errStr, "i/o timeout") {
		return "read-deadline-timeout"
	}
	if strings.Contains(errStr, "operation timed out") {
		return "read-deadline-timeout"
	}
	if strings.Contains(errStr, "use of closed network connection") {
		return "connection-already-closed"
	}
	if strings.Contains(errStr, "connection reset by peer") {
		return "connection-reset"
	}
	if strings.Contains(errStr, "EOF") {
		return "eof"
	}
	return "unknown"
}

func (c *WebSocketClient) Close() {
	c.closeOnce.Do(func() {
		close(c.done)
		if c.conn != nil {
			c.conn.Close()
		}
	})
}

func (c *WebSocketClient) GetEventStats() map[string]uint64 {
	c.messageCounts.RLock()
	defer c.messageCounts.RUnlock()

	stats := make(map[string]uint64)
	for eventType, count := range c.messageCounts.eventCounts {
		stats[eventType] = count
	}
	return stats
}

func (c *WebSocketClient) GetStatusStats() map[EntityStatus]uint64 {
	c.messageCounts.RLock()
	defer c.messageCounts.RUnlock()

	stats := make(map[EntityStatus]uint64)
	for status, count := range c.messageCounts.statusCounts {
		stats[status] = count
	}
	return stats
}

func (c *WebSocketClient) GetChannelStats() map[string]ChannelStats {
	c.channelsMu.Lock()
	defer c.channelsMu.Unlock()

	stats := make(map[string]ChannelStats, len(c.channels))
	for id, cs := range c.channels {
		stats[id] = ChannelStats{
			Name:           cs.name,
			Subscribed:     cs.subscribed,
			Failed:         cs.failed,
			LastSeq:        cs.lastSeq,
			SnapshotFrames: cs.snapshotFrames,
		}
	}
	return stats
}

func (c *WebSocketClient) GetSeqGaps() uint64 {
	c.channelsMu.Lock()
	defer c.channelsMu.Unlock()
	return c.seqGaps
}

func (c *WebSocketClient) GetResyncs() uint64 {
	c.channelsMu.Lock()
	defer c.channelsMu.Unlock()
	return c.resyncs
}

// GetBootstrapStats reports whether the startup bootstrap has completed and
// how long it took.
func (c *WebSocketClient) GetBootstrapStats() (complete bool, duration time.Duration) {
	b := c.bootstrap
	b.mu.Lock()
	defer b.mu.Unlock()
	return !b.active, b.duration
}

func (c *WebSocketClient) GetHeartbeatInterval() time.Duration {
	c.heartbeatMu.RLock()
	defer c.heartbeatMu.RUnlock()
	return c.heartbeatInterval
}
