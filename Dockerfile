# Build stage
FROM golang:1.24.3-alpine AS builder

WORKDIR /app

# Install git, gcc, and SQLite development files
RUN apk add --no-cache git gcc musl-dev sqlite-dev

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled
RUN CGO_ENABLED=1 GOOS=linux go build -o main .

# Final stage
FROM alpine:latest

WORKDIR /app

# Install SQLite runtime
RUN apk add --no-cache sqlite-libs

# Copy the binary from builder
COPY --from=builder /app/main .

# Copy any additional necessary files (like .env if needed)
COPY .env* ./

# Create directory for SQLite database
RUN mkdir -p /app/data && chmod 777 /app/data

# Copy APNS key file to root level
COPY AuthKey_MU2W4LLRSY.p8 .

# List contents to verify the file was copied
RUN ls -la

# Expose the port your application runs on
EXPOSE 8080

# Run the application
CMD ["./main"] 