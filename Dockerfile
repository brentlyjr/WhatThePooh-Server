# Build stage
FROM golang:1.24.3-alpine AS builder

WORKDIR /app

# Install git and gcc for building
RUN apk add --no-cache git gcc musl-dev

# Copy go mod and sum files from root
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code from source/ directory
COPY source/ ./source/

# Build the application
RUN GOOS=linux go build -o main ./source/

# Final stage
FROM alpine:3.21

WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /app/main .

# Expose the port the app runs on
EXPOSE 8080

# Run the binary
CMD ["./main"] 