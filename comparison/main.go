package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
)

func main() {
	log.SetFlags(log.LstdFlags)

	_ = godotenv.Load()
	_ = godotenv.Load("../.env")

	apiKey := os.Getenv("THEMEPARK_API_KEY")
	if apiKey == "" {
		log.Fatal("THEMEPARK_API_KEY is required")
	}

	program := NewProgram(
		apiKey,
		envOrDefault("WEBSOCKET_URL", defaultWebSocketURL),
		envOrDefault("REST_URL", defaultRESTURL),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	program.Run(ctx)
}
