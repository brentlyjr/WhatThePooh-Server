# Build stage
FROM golang:1.24.3-alpine AS builder

WORKDIR /app

# Install git, gcc, and SQLite development files
RUN apk add --no-cache git gcc musl-dev sqlite-dev

# Copy go mod and sum files from root
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code from source directory
COPY source/ .

# Build the application with CGO enabled
RUN CGO_ENABLED=1 GOOS=linux go build -o main .

# Final stage
FROM alpine:latest

WORKDIR /app

# Install SQLite libraries needed for the final binary
RUN apk add --no-cache sqlite-libs

# Copy the compiled binary from the builder stage
COPY --from=builder /app/main .

# Database file will be created by the application
# We can create a data directory
RUN mkdir -p /app/data

# Expose the port the app runs on
EXPOSE 8080

# Run the binary
CMD ["./main"] 