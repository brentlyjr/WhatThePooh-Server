# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

WhatThePooh-Server is a Go service that pushes APNS notifications to iOS clients when theme-park attractions change status. It ingests live data from `themeparks.wiki` via WebSocket (with a REST pre-population pass at startup), tracks per-entity status, and fans out push notifications to devices subscribed to specific (park, entity) pairs.

## Commands

All Go code lives under `./source` (package `main`). The `comparison/` directory is a **separate Go module** unrelated to the server.

```bash
# Run locally (sandbox APNS) — sets all required env vars, loads sandbox .p8 key
./scripts/run-local.sh

# Run directly (requires env vars already set, see below)
go run ./source

# Build production binary
go build -o main ./source

# Install / sync dependencies
go mod tidy

# Docker
docker build -t whatthepooh-server .
docker run --env-file ./.env -p 8080:8080 whatthepooh-server

# Deploy to GCP Cloud Run (uses scripts/gcp_config.sh; production .p8 key)
cd scripts && ./gcp-deploy.sh

# Tail GCP logs
./scripts/gcp-logs.sh
```

There is no test suite — `go test ./...` is a no-op. There is no linter configured.

## Required Environment Variables

The server fails fast on missing values via `getEnvOrExit` in `source/main.go`:

- `APNS_KEY_BASE64` — base64 of the `.p8` file (run-local.sh generates this from `keys/AuthKey_MU2W4LLRSY.p8`)
- `APNS_KEY_ID`, `APNS_TEAM_ID`, `APNS_BUNDLE_ID`
- `APNS_ENV` — `development` or `production` (selects the default APNS client; per-device environment overrides this at send time)
- `THEMEPARK_API_KEY`
- `SUPABASE_DB_URL` — full Postgres connection string (`postgresql://...`)
- `WEBSOCKET_URL` — optional, defaults to `wss://api.themeparks.wiki/v1/entity/live`

A `.env` file at the repo root is loaded automatically via `godotenv` if present; in GCP these are injected as Cloud Run env vars / Secret Manager mounts.

## Architecture

The runtime is a pipeline of goroutines connected by buffered channels and a small pub/sub bus. Read these together — the flow spans multiple files:

```
REST pre-populate (rest_client.go)
        │
        ▼
WebSocket client (websocket_client.go)
        │ Entity (parsed from livedata events)
        ▼
EntityQueue chan (queue.go, buffer 1000)
        │
        ▼
EntityManager.ProcessEntity (entity_manager.go)
   ├── persists status to entity_status table (supabase_db.go)
   └── on status change → messageBus.PublishStatus(...)
                              │
                              ▼
              StartMessageProcessors (message_processor.go)
                              │ looks up subscribed devices
                              ▼
                       Push() → PushQueue chan (queue.go, buffer 100)
                              │
                              ▼
                  StartAPNSWorkers (apns_worker.go, 5 workers)
                              │
                              ▼
                   SendPushNotification → APNS
```

Key architectural points:

- **Single Go package.** All `source/*.go` files are `package main`; symbols like `db` (global), `messageBus`, `EntityQueue`, `PushQueue` are shared package-level state. There is no `internal/` layout.
- **Ride emojis.** Per-entity emoji strings live in `source/data/ride_emojis.json`, are embedded at build time via `//go:embed` in `ride_emojis.go`, and are loaded into memory at startup with `loadRideEmojis()` (lookup via `getRideEmoji(entityID)`).
- **APNS alert copy.** Status-change pushes use title `{rideEmoji} {entityName}` (name only if no emoji in the map) and body `{statusEmoji}Now {NEW_STATUS} in {parkName}`, plus optional wait-time and was-down lines when operating. Custom payload includes `rideEmoji` when mapped.
- **`Database` interface (`database.go`) with two implementations.** `SupabaseDB` (`supabase_db.go`) is the real Postgres-backed store using `pgx`. `CachedDB` (`cache.go`) wraps it with an in-memory device cache (`sync.Map`) populated on startup and refreshed via `POST /api/cache/expire`. The cache holds only device rows — subscriptions and entity status calls pass through. The global `db` variable points to a `CachedDB` wrapping a `SupabaseDB`.
- **WebSocket reconnect loop** (`websocket_client.go`) sends pings every 30s; the read deadline (60s) is extended on inbound messages, received pongs, and successful ping sends. On disconnect it appends a timestamp to a bounded `reconnectionTimestamps` slice exposed via `/api/metrics`. A 5s watchdog logs `[WS] Pong overdue` once if no pong arrives within 45s of the last ping; routine pongs are not logged. Disconnects emit a single consolidated `[WS] Connection lost` line with keepalive context (`pingOutstanding`, read-deadline hint on timeout). It subscribes to a hard-coded list of resort IDs (Disney + Universal) on every (re)connect.
- **Subscription updates use smart diffing.** `SupabaseDB.UpdateSubscriptions` computes added/removed `(park_id, entity_id)` pairs in a transaction and only INSERTs/DELETEs the differences. An empty `subscriptions` array means "unsubscribe from everything" and is valid.
- **Per-device APNS environment.** `apns_worker.go` initializes both dev (`apnsDevClient`) and prod (`apnsProdClient`) clients. Each push uses the client matching the device's `environment` column, so dev and prod devices can coexist in the same database.
- **Auto-disable on bad tokens.** When APNS returns `BadDeviceToken` or `Unregistered`, `SendPushNotification` calls `db.SetDeviceNotificationState(token, false)` — the device row and its subscriptions are preserved so re-registering restores everything.
- **Entity state survives restarts.** On startup, `EntityManager.loadInitialStatuses` reads the `entity_status` table so prior statuses are known. The REST pre-population pass then calls `ProcessEntity(entity, true)` — passing `isInitial=true` suppresses notifications for the first observation of an entity but still emits them for discrepancies between the DB and the REST snapshot.
- **Queues drop on overflow.** Both `QueueEntity` and `Push` use non-blocking `select`/`default` and log a drop if the channel is full — they never block the producer.

## Database Schema

Initial schema in `supabase/001_initial_schema.sql`:

- `devices` — keyed by `device_token`; `notifications_on` gates whether the device receives pushes; many optional iOS metadata columns
- `notification_subscriptions` — `(device_token, entity_id, park_id)` composite PK; cascades on device delete
- `user_feedback` — append-only feedback log
- `entity_status` — last-known status per entity, keyed by `entity_id`; rehydrated into `EntityManager` on startup

RLS is enabled on all tables with a permissive "anonymous access" policy because the server connects as a trusted service via `SUPABASE_DB_URL`.

## Park ID Map

`source/constants.go` hard-codes the `parkID → human-readable name` map used in APNS body copy (`Now … in {parkName}`). `websocket_client.go` separately hard-codes the **resort** IDs (parent of parks) to subscribe to. These two lists are intentionally different — resorts are subscription targets, parks are what come back in `livedata` messages.

## API Surface

Routes are wired in `source/handlers.go:SetupRoutes`. Full request/response examples live in `README.md`. Endpoints fall into these groups:

- Device lifecycle: `POST /api/register-device`, `GET /api/devices`, `GET /api/devices/:token/exists`, `DELETE /api/devices/:token`, `POST /api/enable-notifications`, `POST /api/disable-subscriptions`
- Subscriptions: `POST /api/update-ride-subscriptions` (state replacement via smart diff)
- Push: `POST /api/notifications/send` (direct send, for testing/admin)
- Entity data: `GET /api/entities`, `GET /api/entities/:id`
- Ops: `GET /health`, `GET /api/metrics`, `POST /api/cache/expire`, `POST /api/feedback`

Fiber's body limit is raised to 10MB for the feedback endpoint (which accepts attached client logs up to 5MB).

## Local vs Production APNS

Two `.p8` keys live in `keys/`. `scripts/run-local.sh` uses the sandbox key (`AuthKey_MU2W4LLRSY.p8`) and sets `APNS_ENV=development`; `scripts/gcp-deploy.sh` uses the production key (`AuthKey_AY6CCB64CG.p8`) and sets `APNS_ENV=production`. The keys directory is committed but should be treated as sensitive.
