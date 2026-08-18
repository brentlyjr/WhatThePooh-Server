# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

WhatThePooh-Server is a Go service that pushes APNS notifications to iOS clients when theme-park attractions change status. It ingests live data from `themeparks.wiki` via the preview WebSocket protocol (bootstrapping state from snapshot-on-subscribe at startup), tracks per-entity status, and fans out push notifications to devices subscribed to specific (park, entity) pairs.

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
- `WEBSOCKET_URL` — optional, defaults to `wss://api.themeparks.wiki/v1/live`
- `BOOTSTRAP_TIMEOUT` — optional Go duration (default `60s`); how long startup waits for the WebSocket snapshot bootstrap before proceeding with partial data

A `.env` file at the repo root is loaded automatically via `godotenv` if present; in GCP these are injected as Cloud Run env vars / Secret Manager mounts.

## Architecture

The runtime is a pipeline of goroutines connected by buffered channels and a small pub/sub bus. Read these together — the flow spans multiple files:

```
WebSocket client (websocket_client.go, preview protocol)
   ├── startup: snapshot frames → bootstrap gate → BulkLoad + ReconcileAgainst
   └── live: update (and reconnect snapshot) frames
        │ Entity (via convertLiveDataEntity)
        ▼
EntityQueue chan (queue.go, buffer 5000)
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
- **Preview WebSocket protocol** (`websocket_client.go`). Connects with the `preview` subprotocol and `X-API-Key` header; every server frame is the envelope `{type, channel, seq, ts, data}` routed by `handleFrame` (`welcome`, `subscribed`, `unsubscribed`, `snapshot`, `update`, `ping`, `error`; unknown types are counted and ignored). Subscriptions are sent **after** the `welcome` frame: one `{type:"subscribe", channel:<resortID>, filter:"ATTRACTION", snapshot:true}` per hard-coded resort (Disney + Universal). Snapshot/update payloads reuse the REST `LiveDataEntity` shape. At startup, snapshot entities accumulate behind a bootstrap gate (a channel completes on update-after-snapshot, a 2s snapshot debounce, or subscribe failure); `main.go` waits on `BootstrapDone()` (capped by `BOOTSTRAP_TIMEOUT`), then runs `BulkLoad` + `ReconcileAgainst`. Post-bootstrap, snapshot and update frames all flow through `EntityQueue` → `ProcessEntity`, which no-ops on unchanged status, so reconnect snapshots never double-notify. Server `ping` frames get an app-level `{type:"pong"}` reply; the read deadline is `max(90s, 3×heartbeatIntervalMs)`. Reconnects use exponential backoff (1s→30s + jitter) and append to `reconnectionTimestamps` (exposed via `/api/metrics`). Per-channel `seq` is tracked for gap metrics (`seq_gaps`); gaps are log/metrics-only (filtered-out events consume seq numbers, so gaps carry no loss signal under `filter:"ATTRACTION"`) — only a backwards cursor (server channel restart) triggers the rate-limited per-channel unsubscribe/resubscribe resync.
- **Subscription updates use smart diffing.** `SupabaseDB.UpdateSubscriptions` computes added/removed `(park_id, entity_id)` pairs in a transaction and only INSERTs/DELETEs the differences. An empty `subscriptions` array means "unsubscribe from everything" and is valid.
- **Per-device APNS environment.** `apns_worker.go` initializes both dev (`apnsDevClient`) and prod (`apnsProdClient`) clients. Each push uses the client matching the device's `environment` column, so dev and prod devices can coexist in the same database.
- **Auto-disable on bad tokens.** When APNS returns `BadDeviceToken` or `Unregistered`, `SendPushNotification` calls `db.SetDeviceNotificationState(token, false)` — the device row and its subscriptions are preserved so re-registering restores everything.
- **Entity state survives restarts.** On startup, `EntityManager.loadInitialStatuses` reads the `entity_status` table so prior statuses are known. The WebSocket bootstrap snapshot is then loaded silently via `BulkLoad`, and `ReconcileAgainst(dbSnapshot)` emits notifications only for entities whose status changed while the server was offline (new entities are persisted without notifying; unchanged entities keep the DB's `LastStatusChange` so duration counters survive the restart).
- **Timestamps come from the API.** `convertLiveDataEntity` (`rest_client.go`) parses each entity's `lastUpdated` (falling back to the frame's `ts`, then `time.Now()`); `ProcessEntity`/`ReconcileAgainst` use it (via `eventTimeOrNow`, with a monotonic guard against going backwards) for `LastStatusChange`, `entity_status.last_updated`, and the APNS `timestamp`/"Was Down X ago" math.
- **Wait times are polled, not pushed, and scoped to a park.** APNS is reserved for status changes; the client polls `POST /api/wait-times` with `{deviceToken, parkId}` and gets every ATTRACTION in that park (`wait_times.go`), sorted by name, full list every time — there is no delta/cursor. It serves from `EntityManager.GetAttractionsByPark`, which ranges the `sync.Map` **without taking `em.mu`** (that mutex serialises `ProcessEntity`; holding it for a full scan would stall ingestion). `deviceToken` is a registered-app gate, not park authorization. `parkNames` in `constants.go` doubles as the park allowlist, so an unlisted park 400s. Wait-time changes are still published on the message bus and logged (`⏰ WAIT TIME CHANGE`) but never enqueued to `PushQueue` — doing so would starve status pushes, since `PushQueue` is only 100 deep and drops on overflow. `Entity.WaitTimeReported` distinguishes "no STANDBY queue reported" from a genuine 0-minute wait (`convertLiveDataEntity` renders both as `0`); the endpoint emits `waitTime: null` for the former. Wait time is **not** persisted — `entity_status` has no column for it, so it re-bootstraps from the WS snapshot on restart.
- **`ProcessEntity` refreshes identity fields from every live frame.** `entity_status` has no `park_id` or `entity_type` column, so rows rehydrated by `loadInitialStatuses` start with empty `ParkID`/`EntityType` and cannot match a park filter. `BulkLoad` (startup only) used to be the sole path that wrote those fields, and reconnect snapshots arrive through `ProcessEntity` like any update — so without the copy of `Name`/`ParkID`/`EntityType`/`LastUpdated` in `ProcessEntity`, a resort that missed its bootstrap snapshot would stay invisible to `/api/wait-times` for the life of the process, and `LastUpdated` would stay frozen at snapshot time for every entity.
- **Responses are gzipped.** `main.go` installs `compress.New()` globally, motivated by the park board (~10-50KB polled every 60s per device) but applying to `/api/entities` and `/api/metrics` too.
- **Queues drop on overflow.** Both `QueueEntity` and `Push` use non-blocking `select`/`default` and log a drop if the channel is full — they never block the producer.

## Database Schema

Initial schema in `supabase/001_initial_schema.sql`:

- `devices` — keyed by `device_token`; `notifications_on` gates whether the device receives pushes; many optional iOS metadata columns
- `notification_subscriptions` — `(device_token, entity_id, park_id)` composite PK; cascades on device delete
- `user_feedback` — append-only feedback log
- `entity_status` — last-known status per entity, keyed by `entity_id`; rehydrated into `EntityManager` on startup

RLS is enabled on all tables with a permissive "anonymous access" policy because the server connects as a trusted service via `SUPABASE_DB_URL`.

## Park ID Map

`source/constants.go` hard-codes the `parkID → human-readable name` map used in APNS body copy (`Now … in {parkName}`). `websocket_client.go` separately hard-codes the **resort** IDs (parent of parks) to subscribe to. These two lists are intentionally different — resorts are subscription channels, parks are what come back in each entity's `parkId` field.

Note: the separate `comparison/` module still speaks the legacy (`v1` subprotocol, `event`-discriminated) WebSocket shape and will stop working when the legacy protocol is removed upstream.

## API Surface

Routes are wired in `source/handlers.go:SetupRoutes`. Full request/response examples live in `README.md`. Endpoints fall into these groups:

- Device lifecycle: `POST /api/register-device`, `GET /api/devices`, `GET /api/devices/:token/exists`, `DELETE /api/devices/:token`, `POST /api/enable-notifications`, `POST /api/disable-subscriptions`
- Subscriptions: `POST /api/update-ride-subscriptions` (state replacement via smart diff)
- Push: `POST /api/notifications/send` (direct send, for testing/admin)
- Entity data: `GET /api/entities`, `GET /api/entities/:id`, `POST /api/wait-times` (park-scoped poll endpoint; see below)
- Ops: `GET /health`, `GET /api/metrics`, `POST /api/cache/expire`, `POST /api/feedback`

Fiber's body limit is raised to 10MB for the feedback endpoint (which accepts attached client logs up to 5MB).

## Local vs Production APNS

Two `.p8` keys live in `keys/`. `scripts/run-local.sh` uses the sandbox key (`AuthKey_MU2W4LLRSY.p8`) and sets `APNS_ENV=development`; `scripts/gcp-deploy.sh` uses the production key (`AuthKey_AY6CCB64CG.p8`) and sets `APNS_ENV=production`. The keys directory is committed but should be treated as sensitive.
