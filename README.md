# WhatThePooh Server

A Go-based server application for managing theme park attraction data and notifications.

## Recent Changes

### API Simplification (Latest)
- **Simplified subscription format**: Both `/api/register-device` and `/api/update-ride-subscriptions` now use a flat subscription format instead of grouping by park
- **Before**: `{"parkId": "...", "entityIds": ["ride1", "ride2"]}`
- **After**: `{"parkId": "...", "entityId": "ride1"}`, `{"parkId": "...", "entityId": "ride2"}`
- This change simplifies both client and server code while maintaining the same functionality

## Prerequisites

- Go 1.24.3 or later
- Git (for cloning the repository)
- Docker (optional, for containerized deployment)

## Local Development Setup

To run the application on your local machine, follow these steps.

1.  **Clone the Repository**
    ```bash
    git clone https://github.com/brentlyjr/WhatThePooh-Server.git
    cd WhatThePooh-Server
    ```

2.  **Install Dependencies**
    ```bash
    go mod tidy
    ```

3.  **Configure Environment Variables**
    The project uses a `.env` file for local configuration. An example file is provided.

    *   **Create your personal `.env` file:**
        ```bash
        cp .env.example .env
        ```

    *   **Place your APNS Key:**
        Put your `AuthKey_YOURKEYID.p8` file into the `/keys` directory.

    *   **Update your `.env` file:**
        Open the `.env` file and set the required values. The `APNS_KEY_BASE64` will be automatically generated when you run the application. Fill in the other required values like your `APNS_KEY_ID`, `APNS_TEAM_ID`, etc.

4.  **Run the Application**
    ```bash
    go run ./source
    ```
    The server will start on `http://localhost:8080`.

## Building and Running with Docker

The project can also be built and run as a Docker container.

1.  **Build the Docker Image**
    ```bash
    docker build -t whatthepooh-server .
    ```

2.  **Run the Container**
    When running with Docker, you must pass your environment variables to the container. The recommended way is to use the `--env-file` flag with your configured `.env` file.
    ```bash
    docker run --env-file ./.env -p 8080:8080 whatthepooh-server
    ```

## API Endpoints

### Device Management

- **Register Device** (`POST /api/register-device`)
  Register a device with optional initial subscriptions. This is the primary endpoint for device onboarding.
  
  ```json
  {
    "deviceToken": "your_device_token",
    "appVersion": "1.0.0",
    "environment": "development",
    "notificationsOn": true,
    "iosVersion": "17.0",
    "deviceName": "Brent's iPhone",
    "systemName": "iOS",
    "language": "en",
    "region": "US",
    "timeZone": "America/Los_Angeles",
    "deviceModel": "iPhone 15 Pro",
    "deviceModelIdentifier": "iPhone16,1",
    "subscriptions": [
      {
        "parkId": "7340550b-c14d-4def-80bb-acdb51d49a66",
        "entityId": "Disneyland_SpaceMountain"
      }
    ]
  }
  ```
  
  **Field Descriptions:**
  - `deviceToken` (required): The APNS device token for push notifications
  - `appVersion` (optional): The version of the client app
  - `environment` (optional): APNS environment - must be "development" or "production". Defaults to "development" if not provided
  - `notificationsOn` (optional): Whether notifications are enabled. Defaults to true
  - `subscriptions` (optional): Initial list of subscriptions to set up
  
  **Optional Device Information Fields:**
  - `iosVersion` (optional): iOS version running on the device
  - `deviceName` (optional): User-defined device name
  - `systemName` (optional): Operating system name (e.g., "iOS")
  - `language` (optional): Device language setting (e.g., "en", "es")
  - `region` (optional): Device region setting (e.g., "US", "CA")
  - `timeZone` (optional): Device timezone (e.g., "America/Los_Angeles")
  - `deviceModel` (optional): Human-readable device model (e.g., "iPhone 15 Pro")
  - `deviceModelIdentifier` (optional): Device model identifier (e.g., "iPhone16,1")

  **Success Response:**
  ```json
  {
    "status": "Device registered successfully",
    "subscriptionsCount": 1
  }
  ```

- **Get All Devices** (`GET /api/devices`)
  Returns a list of all registered devices (admin endpoint)

- **Check Device Exists** (`GET /api/devices/:token/exists`)
  Checks if a specific device token is registered
  
  **Response when device exists:**
  ```json
  {
    "exists": true,
    "device": {
      "deviceToken": "your_device_token",
      "appVersion": "1.0.0",
      "environment": "development",
      "notificationsOn": true,
      "lastUpdated": "2025-06-21T01:48:25Z",
      "iosVersion": "17.0",
      "deviceName": "Brent's iPhone",
      "systemName": "iOS",
      "language": "en",
      "region": "US",
      "timeZone": "America/Los_Angeles",
      "deviceModel": "iPhone 15 Pro",
      "deviceModelIdentifier": "iPhone16,1"
    }
  }
  ```
  
  **Response when device doesn't exist:**
  ```json
  {
    "exists": false,
    "message": "Device not found"
  }
  ```

- **Delete Device** (`DELETE /api/devices/:token`)
  Removes a device token from the database

### Subscription Management

- **Update Ride Subscriptions** (`POST /api/update-ride-subscriptions`)
  Updates the complete subscription list for a device. This endpoint uses smart diffing to only change what's actually different, making it highly efficient. Designed for client-side batching where changes are aggregated before sending.
  
  ```json
  {
    "deviceToken": "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456",
    "schemaVersion": 1,
    "timestamp": "2025-01-27T15:42:33.123Z",
    "subscriptions": [
      {
        "parkId": "7340550b-c14d-4def-80bb-acdb51d49a66",
        "entityId": "Disneyland_DisneylandRailroad"
      },
      {
        "parkId": "7340550b-c14d-4def-80bb-acdb51d49a66",
        "entityId": "Disneyland_HauntedMansion"
      },
      {
        "parkId": "7340550b-c14d-4def-80bb-acdb51d49a66",
        "entityId": "Disneyland_PiratesoftheCaribbean"
      },
      {
        "parkId": "7340550b-c14d-4def-80bb-acdb51d49a66",
        "entityId": "Disneyland_SpaceMountain"
      },
      {
        "parkId": "832fcd51-ea19-4e77-85c7-75d5843b127c",
        "entityId": "DisneyCaliforniaAdventure_GuardiansoftheGalaxyMissionBreakout"
      },
      {
        "parkId": "832fcd51-ea19-4e77-85c7-75d5843b127c",
        "entityId": "DisneyCaliforniaAdventure_RadiatorSpringsRacers"
      }
    ]
  }
  ```
  
  **Field Descriptions:**
  - `deviceToken` (required): The APNS device token (must be registered first)
  - `schemaVersion` (required): API schema version (currently 1)
  - `timestamp` (required): ISO 8601 UTC timestamp when snapshot was taken
  - `subscriptions` (required): Array of individual subscription objects (empty array = unsubscribe from all)
    - `parkId` (required): Unique identifier for the theme park
    - `entityId` (required): Individual attraction/entity ID within that park
  
  **Success Response:**
  ```json
  {
    "status": "Subscriptions updated successfully",
    "totalSubscriptions": 6,
    "timestamp": "2025-01-27T15:42:33.123Z"
  }
  ```
  
  **Error Responses:**
  - `400 Bad Request`: Invalid request body or missing required fields
  - `404 Not Found`: Device token not registered (register device first)
  - `500 Internal Server Error`: Database error during subscription update
  
  **Performance Notes:**
  - Uses smart diffing algorithm with O(m + n) complexity
  - Only executes database operations for actual changes
  - Example: Adding 1 ride to 30 existing subscriptions = 1 INSERT operation (~1-5ms)
  - Handles complete state replacement safely (network dead zones, missed updates)
  - Designed for client-side batching (10-second idle timer)

- **Disable Subscriptions** (`POST /api/disable-subscriptions`)
  Disables notifications for a device while preserving subscription data.
  
  ```json
  {
    "deviceToken": "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456"
  }
  ```
  
  **Success Response:**
  ```json
  {
    "status": "Notifications disabled successfully",
    "deviceToken": "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456"
  }
  ```

### Push Notifications

- **Send Push Notification** (`POST /api/notifications/send`)
  Send a direct APNS notification to a specific device. Useful for testing and admin notifications.
  
  ```json
  {
    "deviceToken": "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456",
    "title": "Space Mountain Status",
    "message": "Space Mountain is now OPEN with a 45 minute wait!",
    "entityId": "Disneyland_SpaceMountain",
    "entityName": "Space Mountain",
    "parkId": "7340550b-c14d-4def-80bb-acdb51d49a66",
    "parkName": "Disneyland",
    "oldStatus": "CLOSED",
    "newStatus": "OPEN",
    "oldWaitTime": 0,
    "newWaitTime": 45,
    "environment": "development",
    "timestamp": "2025-01-27T15:42:33.123Z",
    "notificationId": "test-123"
  }
  ```
  
  **Field Descriptions:**
  - `deviceToken` (required): The APNS device token
  - `title` (optional): Notification title (defaults to park name)
  - `message` (optional): Notification message (defaults to entity status change)
  - `entityId` (optional): Entity identifier
  - `entityName` (optional): Human-readable entity name
  - `parkId` (optional): Park identifier
  - `parkName` (optional): Human-readable park name
  - `oldStatus` (optional): Previous entity status
  - `newStatus` (optional): New entity status
  - `oldWaitTime` (optional): Previous wait time
  - `newWaitTime` (optional): New wait time
  - `environment` (optional): APNS environment (defaults to device's environment)
  - `timestamp` (optional): Notification timestamp (defaults to current time)
  - `notificationId` (optional): Custom notification ID (auto-generated if not provided)
  
  **Success Response:**
  ```json
  {
    "status": "Notification sent successfully",
    "notificationId": "test-123",
    "deviceToken": "a1b2c3d4e5f6789012345678901234567890abcdef1234567890abcdef123456"
  }
  ```

### Theme Park Data

- **Get All Entities** (`GET /api/entities`)
  Returns all theme park attractions and their current status

- **Get Entity by ID** (`GET /api/entities/:id`)
  Returns a specific attraction's status

### System

- **Health Check** (`GET /health`)
  Returns server health status

- **Metrics** (`GET /api/metrics`)
  Returns server metrics including queue length, entity count, and device count

## Project Structure

- `source/` - Go source code directory
  - `main.go` - Main application entry point
  - `entity_manager.go` - Manages theme park attraction data
  - `websocket_client.go` - WebSocket client implementation
  - `queue.go` - Queue management
  - `apns_worker.go` - Apple Push Notification Service worker
  - `database.go` - Database operations for device management
  - `cache.go` - Caching layer for database operations
  - `message_bus.go` - Message bus implementation
  - `message_processor.go` - Message processing logic
- `go.mod` - Go module definition (root level)
- `go.sum` - Go module checksums (root level)
- `keys/` - Directory for APNS key files (e.g., `AuthKey_YOURKEYID.p8`)
- `scripts/` - Deployment and development scripts
  - `run-local.sh` - Local development script
  - `gcp-deploy.sh` - GCP deployment script
  - `gcp-destroy.sh` - GCP cleanup script
  - `gcp-logs.sh` - GCP logs script
  - `gcp_config.sh` - GCP configuration
- `Dockerfile` - Docker container configuration

## Dependencies

The project uses the following main dependencies:
- github.com/gofiber/fiber/v2
- github.com/gorilla/websocket
- github.com/joho/godotenv
- github.com/sideshow/apns2
- github.com/jackc/pgx/v5 (for Supabase/PostgreSQL)

## License

[Add your license information here]

## Development Guide

This guide explains how to run the WhatThePooh Server in different environments.

### Local Development (Sandbox APNS)

For local development, use the sandbox APNS environment to avoid affecting production users.

#### Option 1: Using the run script (Recommended)
```bash
./scripts/run-local.sh
```

This script automatically:
- Sets all environment variables for sandbox APNS
- Uses the sandbox key (`keys/AuthKey_MU2W4LLRSY.p8`)
- Starts the server with `go run ./source`

#### Option 2: Manual environment setup
```bash
# Set environment variables
export APNS_ENV="development"
export APNS_KEY_ID="MU2W4LLRSY"
export APNS_TEAM_ID="SVFXRTGAKU"
export APNS_BUNDLE_ID="com.brentlyjr.WhatThePooh"
export WEBSOCKET_URL="wss://themeparkswiki.herokuapp.com/v1/live"
export THEMEPARK_API_KEY="519dd9c1-cc1e-4d4a-906d-d628cf0250bc"
export APNS_KEY_BASE64=$(base64 -i "keys/AuthKey_MU2W4LLRSY.p8" | tr -d '\n')

# Run the server
go run ./source
```

### Production Deployment (Production APNS)

For production deployment to GCP, use the production APNS environment.

#### Deploy to GCP
```bash
cd scripts
./gcp-deploy.sh
```

This automatically:
- Uses the production key (`keys/AuthKey_AY6CCB64CG.p8`)
- Sets `APNS_ENV="production"`
- Deploys to Google Cloud Run

### Key Differences

| Environment | APNS Key | APNS Environment | Use Case |
|-------------|----------|------------------|----------|
| Local | `AuthKey_MU2W4LLRSY.p8` | Sandbox | Development & Testing |
| GCP | `AuthKey_AY6CCB64CG.p8` | Production | Live Users |

### Troubleshooting

#### Device tokens disappearing quickly
- **Local**: This is normal in sandbox environment
- **Production**: Check APNS certificate validity

#### APNS connection issues
- Verify key files exist in `keys/` directory
- Check Apple Developer Portal for key validity
- Ensure bundle ID matches your app configuration 