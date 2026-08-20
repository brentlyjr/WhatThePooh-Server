package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

func (p *Program) runWebSocket(ctx context.Context) {
	backoff := minBackoff
	for {
		if ctx.Err() != nil {
			return
		}

		connected, err := p.connectAndRead(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			log.Printf("websocket disconnected: %v", err)
		}
		if connected {
			backoff = minBackoff
		}

		wait := backoff + time.Duration(rand.Int63n(int64(backoff/2)+1))
		log.Printf("websocket reconnecting in %s...", wait.Round(time.Millisecond))
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (p *Program) connectAndRead(ctx context.Context) (bool, error) {
	headers := http.Header{
		"X-API-Key": {p.apiKey},
		"Origin":    {"https://themeparks.wiki"},
	}
	dialer := websocket.Dialer{
		HandshakeTimeout: 45 * time.Second,
		Subprotocols:     []string{"preview"},
	}

	log.Printf("connecting to websocket...")
	conn, resp, err := dialer.DialContext(ctx, p.websocketURL, headers)
	if err != nil {
		if resp != nil {
			return false, fmt.Errorf("dial failed: %w (status %s)", err, resp.Status)
		}
		return false, fmt.Errorf("dial failed: %w", err)
	}
	defer conn.Close()
	stopClose := make(chan struct{})
	defer close(stopClose)
	go func() {
		select {
		case <-ctx.Done():
			conn.Close()
		case <-stopClose:
		}
	}()

	if conn.Subprotocol() != "preview" {
		return true, fmt.Errorf("server negotiated subprotocol %q instead of preview", conn.Subprotocol())
	}

	p.writeMu.Lock()
	p.conn = conn
	p.writeMu.Unlock()
	defer func() {
		p.writeMu.Lock()
		p.conn = nil
		p.writeMu.Unlock()
	}()

	log.Printf("websocket connected (subprotocol=preview)")

	welcomeTimer := time.AfterFunc(welcomeTimeout, func() {
		log.Printf("no welcome frame within %s — closing", welcomeTimeout)
		conn.Close()
	})
	defer welcomeTimer.Stop()

	conn.SetReadDeadline(time.Now().Add(p.readWait()))
	conn.SetPingHandler(func(appData string) error {
		deadline := time.Now().Add(p.readWait())
		conn.SetReadDeadline(deadline)
		return conn.WriteControl(websocket.PongMessage, []byte(appData), deadline)
	})

	for {
		if ctx.Err() != nil {
			return true, ctx.Err()
		}
		_, message, err := conn.ReadMessage()
		if err != nil {
			return true, err
		}
		now := time.Now()
		conn.SetReadDeadline(now.Add(p.readWait()))
		p.handleFrame(conn, welcomeTimer, message)
	}
}

func (p *Program) readWait() time.Duration {
	p.mu.Lock()
	hb := p.heartbeat
	p.mu.Unlock()
	wait := 3 * hb
	if wait < defaultReadWait {
		return defaultReadWait
	}
	return wait
}

func (p *Program) sendJSON(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	if p.conn == nil {
		return fmt.Errorf("not connected")
	}
	return p.conn.WriteMessage(websocket.TextMessage, data)
}

func (p *Program) handleFrame(conn *websocket.Conn, welcomeTimer *time.Timer, message []byte) {
	var f previewFrame
	if err := json.Unmarshal(message, &f); err != nil {
		log.Printf("websocket: failed to parse frame: %v", err)
		return
	}

	switch f.Type {
	case "welcome":
		welcomeTimer.Stop()
		var wd welcomeData
		if err := json.Unmarshal(f.Data, &wd); err != nil {
			log.Printf("websocket: failed to parse welcome: %v", err)
		} else if wd.HeartbeatIntervalMs > 0 {
			p.mu.Lock()
			p.heartbeat = time.Duration(wd.HeartbeatIntervalMs) * time.Millisecond
			p.mu.Unlock()
			conn.SetReadDeadline(time.Now().Add(p.readWait()))
		}
		log.Printf("websocket welcome received; subscribing to Disneyland Resort")
		msg := subscribeMsg{
			Type:     "subscribe",
			Channel:  p.resortID,
			Filter:   "ATTRACTION",
			Snapshot: true,
			ReqID:    p.resortID,
		}
		if err := p.sendJSON(msg); err != nil {
			log.Printf("websocket: subscribe failed: %v", err)
		}

	case "subscribed":
		log.Printf("subscribed to Disneyland Resort (filter=ATTRACTION, snapshot=true)")

	case "snapshot":
		p.handleSnapshot(f)

	case "update":
		p.handleUpdate(f)

	case "ping":
		if err := p.sendJSON(pongMsg{Type: "pong"}); err != nil {
			log.Printf("websocket: pong failed: %v", err)
		}

	case "error":
		var ed errorData
		if err := json.Unmarshal(f.Data, &ed); err != nil {
			log.Printf("websocket error frame: %s", string(f.Data))
			return
		}
		log.Printf("websocket error: code=%d message=%q retryable=%v", ed.Code, ed.Message, ed.Retryable)

	case "unsubscribed":
		log.Printf("websocket unsubscribed from channel %s", f.Channel)

	default:
		// Ignore unknown frame types.
	}
}

func (p *Program) handleSnapshot(f previewFrame) {
	var items []LiveDataEntity
	if err := json.Unmarshal(f.Data, &items); err != nil {
		log.Printf("websocket: failed to parse snapshot: %v", err)
		return
	}

	receivedAt := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()

	createPending := p.wsBaselineDone
	for _, item := range items {
		attr, ok := convertLiveDataEntity(item, f.Ts)
		if !ok {
			continue
		}
		p.applyWSLocked(attr, receivedAt, createPending)
	}

	if !p.wsBaselineDone {
		if p.snapshotDebounce != nil {
			p.snapshotDebounce.Stop()
		}
		p.snapshotDebounce = time.AfterFunc(snapshotDebounce, func() {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.markWSBaselineLocked()
		})
	}
}

func (p *Program) handleUpdate(f previewFrame) {
	var item LiveDataEntity
	if err := json.Unmarshal(f.Data, &item); err != nil {
		log.Printf("websocket: failed to parse update: %v", err)
		return
	}
	attr, ok := convertLiveDataEntity(item, f.Ts)
	if !ok {
		return
	}

	receivedAt := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	p.markWSBaselineLocked()
	p.applyWSLocked(attr, receivedAt, true)
}

func (p *Program) markWSBaselineLocked() {
	if p.wsBaselineDone {
		return
	}
	p.wsBaselineDone = true
	if p.snapshotDebounce != nil {
		p.snapshotDebounce.Stop()
		p.snapshotDebounce = nil
	}
	log.Printf("websocket snapshot baseline complete (%d attractions)", len(p.wsState))
}
