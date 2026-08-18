package main

import (
	"context"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

var (
	shutdownDone       = make(chan struct{})
	entityConsumerWg   sync.WaitGroup
	messageProcessorWg sync.WaitGroup
	apnsWorkersWg      sync.WaitGroup
)

type shutdownDeps struct {
	App        *fiber.App
	WSClient   *WebSocketClient
	Reconciler *Reconciler
	SupabaseDB *SupabaseDB
}

func parseShutdownTimeout() time.Duration {
	const defaultTimeout = 8 * time.Second
	raw := os.Getenv("SHUTDOWN_TIMEOUT")
	if raw == "" {
		log.Printf("SHUTDOWN_TIMEOUT not set, using default %s", defaultTimeout)
		return defaultTimeout
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		log.Fatalf("Invalid SHUTDOWN_TIMEOUT %q: %v", raw, err)
	}
	log.Printf("SHUTDOWN_TIMEOUT=%s", d)
	return d
}

func waitWithCtx(ctx context.Context, wg *sync.WaitGroup, phase string) error {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		log.Printf("[SHUTDOWN] %s complete", phase)
		return nil
	case <-ctx.Done():
		log.Printf("[SHUTDOWN] phase timed out: %s (%v)", phase, ctx.Err())
		return ctx.Err()
	}
}

var shutdownOnce sync.Once

func gracefulShutdown(ctx context.Context, deps shutdownDeps) {
	shutdownOnce.Do(func() {
		log.Println("[SHUTDOWN] Starting graceful shutdown...")

		// Early cleanup in parallel. HTTP must be fully drained before
		// close(PushQueue); otherwise a handler could panic on send-to-closed-channel.
		var httpWg sync.WaitGroup
		httpWg.Add(1)
		go func() {
			defer httpWg.Done()
			if err := deps.App.ShutdownWithContext(ctx); err != nil {
				log.Printf("[SHUTDOWN] HTTP: %v", err)
			} else {
				log.Println("[SHUTDOWN] HTTP complete")
			}
		}()
		deps.WSClient.Close()
		if deps.Reconciler != nil {
			deps.Reconciler.Stop()
		}
		httpWg.Wait()

		log.Println("[SHUTDOWN] Draining entity queue...")
		close(EntityQueue)
		waitWithCtx(ctx, &entityConsumerWg, "entity consumer")

		log.Println("[SHUTDOWN] Draining message processor...")
		messageBus.CloseStatusSubscribers()
		messageBus.CloseWaitTimeSubscribers()
		waitWithCtx(ctx, &messageProcessorWg, "message processor")

		log.Println("[SHUTDOWN] Abandoning push enqueue...")
		close(shutdownDone)

		log.Println("[SHUTDOWN] Draining push queue...")
		close(PushQueue)
		waitWithCtx(ctx, &apnsWorkersWg, "APNS workers")

		log.Println("[SHUTDOWN] Closing database...")
		if err := deps.SupabaseDB.Close(); err != nil {
			log.Printf("[SHUTDOWN] database close: %v", err)
		}

		log.Println("[SHUTDOWN] Graceful shutdown complete")
	})
}
