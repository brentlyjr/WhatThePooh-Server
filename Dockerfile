# Build stage
FROM golang:1.24.3-alpine AS builder

WORKDIR /app

# Install git and build dependencies
RUN apk add --no-cache git

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Final stage
FROM alpine:latest

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/main .

# Copy any additional necessary files (like .env if needed)
COPY .env* ./

# Expose the port your application runs on
EXPOSE 8080

# Run the application
CMD ["./main"] 