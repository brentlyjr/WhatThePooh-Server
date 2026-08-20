# WhatThePooh Comparison Program

Standalone Disneyland Resort tool that measures how long after a WebSocket status or wait-time change the same values appear on the REST live endpoint.

It does **not** talk to the WhatThePooh server. Both transports hit themeparks.wiki directly.

## What it compares

1. **WebSocket** — preview protocol, subscribe to Disneyland Resort (`filter: ATTRACTION`, snapshot on subscribe).
2. **REST** — `GET /v1/entity/{resortId}/live?entityType=ATTRACTION` every minute.

Only `ATTRACTION` entities are considered (Disneyland + Disney California Adventure). Wait time is `queue.STANDBY.waitTime`; a missing STANDBY queue is treated as distinct from a genuine 0-minute wait.

When a WebSocket update arrives, it is recorded. Each REST poll looks for an **exact match** of that latest value. If this poll misses, the next one is tried, up to 5 polls (~5 minutes). A newer WebSocket value for the same field overwrites the pending target.

## Prerequisites

- Go 1.24.3 or later
- `THEMEPARK_API_KEY` (same key as the main server)

Optional:

- `WEBSOCKET_URL` — default `wss://api.themeparks.wiki/v1/live`
- `REST_URL` — default `https://api.themeparks.wiki/v1/entity`

A `.env` in this directory or the repo root is loaded automatically if present.

## Running

```bash
cd comparison
go mod tidy
export THEMEPARK_API_KEY="..."   # or rely on ../.env
go run .
```

Press Ctrl+C to stop. On shutdown the program dumps any still-pending WebSocket updates and run totals.

## Output

```
websocket update for Space Mountain wait time 45 -> 50 at 21:04:01 (lastUpdated 21:03:58)
polling update for Space Mountain wait time 50 at 21:05:00 (59s after, lastUpdated 21:03:58)

websocket update for Haunted Mansion status OPERATING -> DOWN at 21:06:12 (lastUpdated 21:06:11)
polling update for Haunted Mansion status DOWN at 21:07:00 (48s after, lastUpdated 21:06:11)
```

If a later WebSocket update replaces a value REST never showed:

```
websocket superseded Space Mountain wait time 5 -> 10 (5 never seen on REST)
```

If REST changes, but not to the pending WebSocket value:

```
REST DIVERGED for Space Mountain wait time: websocket wants 50, poll now shows 45 (still waiting)
```

If REST still has not matched after 5 polls:

```
UNMATCHED for Space Mountain wait time 50 (websocket at 21:04:01, last REST was 45 after 5 polls / 5m0s)
```

Every poll prints a heartbeat so a quiet terminal still proves the matcher is running:

```
poll complete: matched 2, pending 3, diverged 0, unmatched 1
```

Lag is local receive time (`REST poll time − WebSocket receive time`). Payload `lastUpdated` is logged for context only.

## Notes

- Initial WebSocket snapshot and the first REST poll are baselines; they do not create pending latency rows.
- Reconnect snapshots only create pending rows when status or wait time differs from the last WebSocket state.
- REST-only changes (never seen on WebSocket) are ignored for latency.
- The program reconnects to the WebSocket with exponential backoff if the connection drops.
