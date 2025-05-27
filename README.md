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

### Running Locally from Terminal

1. **Clone the repository** (if you haven't already):
   ```bash
   git clone https://github.com/brentlyjr/WhatThePooh-Server.git
   cd WhatThePooh-Server
   ```

2. **Set up environment variables**:
   Create a `.env` file in the project root:
   ```bash
   touch .env
   ```
   Add your required environment variables to the `.env` file.

3. **Install dependencies**:
   ```bash
   go mod download
   ```

4. **Run the application**:
   ```bash
   go run .
   ```
   
   Or build and run the binary:
   ```bash
   go build
   ./WhatThePooh-Server
   ```

5. **Verify the application is running**:
   The server should start and listen on port 8080 by default. You can test it by opening:
   ```
   http://localhost:8080
   ```

6. **Stopping the application**:
   Press `Ctrl+C` in the terminal to stop the server.

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