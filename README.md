# WhatThePooh Server

A Go-based server application for managing theme park attraction data and notifications.

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

    *   **Generate the Base64 Key:**
        The application requires your APNS key to be base64 encoded for security. Run the following command, making sure to replace `AuthKey_YOURKEYID.p8` with your actual filename.
        ```bash
        base64 -i keys/AuthKey_YOURKEYID.p8 | tr -d '\n'
        ```

    *   **Update your `.env` file:**
        Open the `.env` file and paste the output from the previous command as the value for `APNS_KEY_BASE64`. Fill in the other required values like your `APNS_KEY_ID`, `APNS_TEAM_ID`, etc.

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
  ```json
  {
    "deviceToken": "your_device_token",
    "appVersion": "1.0.0",
    "deviceType": "iPhone"
  }
  ```

- **Get All Devices** (`GET /api/devices`)
  Returns a list of all registered devices

- **Delete Device** (`DELETE /api/devices/:token`)
  Removes a device token from the database

### Push Notifications

- **Send Push Notification** (`POST /api/push`)
  ```json
  {
    "deviceToken": "your_device_token",
    "title": "Notification Title",
    "message": "Notification Message",
    "badge": 1
  }
  ```

### Theme Park Data

- **Get All Entities** (`GET /api/entities`)
  Returns all theme park attractions and their current status

- **Get Entity by ID** (`GET /api/entities/:id`)
  Returns a specific attraction's status

- **Health Check** (`GET /health`)
  Returns server health status

- **Metrics** (`GET /api/metrics`)
  Returns server metrics including queue length and entity count

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
- `gcp-deploy/` - Google Cloud Platform deployment scripts
- `run-local.sh` - Local development script
- `Dockerfile` - Docker container configuration

## Dependencies

The project uses the following main dependencies:
- github.com/gofiber/fiber/v2
- github.com/gorilla/websocket
- github.com/joho/godotenv
- github.com/sideshow/apns2
- github.com/mattn/go-sqlite3

## License

[Add your license information here] 