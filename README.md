# WhatThePooh Server

A Go-based server application for managing theme park attraction data and notifications.

## Prerequisites

- Docker installed on your system
- Git (for cloning the repository)

## Building and Running with Docker

### Build the Docker Image

```bash
# Build the Docker image
docker build -t whatthepooh-server .
```

### Run the Container

```bash
# Run the container
docker run -p 8080:8080 whatthepooh-server
```

### Environment Variables

The application uses environment variables for configuration. Make sure to create a `.env` file in the project root before building the Docker image. The Dockerfile will automatically copy any `.env` files into the container.

## Development

### Local Development

To run the application locally without Docker:

1. Install Go 1.24.3 or later
2. Install dependencies:
   ```bash
   go mod download
   ```
3. Run the application:
   ```bash
   go run .
   ```

## Project Structure

- `main.go` - Main application entry point
- `entity_manager.go` - Manages theme park attraction data
- `websocket_client.go` - WebSocket client implementation
- `queue.go` - Queue management
- `apns_worker.go` - Apple Push Notification Service worker

## Dependencies

The project uses the following main dependencies:
- github.com/gofiber/fiber/v2
- github.com/gorilla/websocket
- github.com/joho/godotenv
- github.com/sideshow/apns2

## License

[Add your license information here] 