package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed data/ride_emojis.json
var rideEmojisJSON []byte

var rideEmojis map[string]string

func loadRideEmojis() error {
	if err := json.Unmarshal(rideEmojisJSON, &rideEmojis); err != nil {
		return fmt.Errorf("parse ride_emojis.json: %w", err)
	}
	return nil
}

func getRideEmoji(entityID string) string {
	if rideEmojis == nil {
		return ""
	}
	return rideEmojis[entityID]
}
