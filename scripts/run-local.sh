#!/bin/bash

# Local Development Script for WhatThePooh Server
# This script sets up environment variables for local development with sandbox APNS

set -e

# Determine the project root directory
# This script is in scripts/, so project root is one level up
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

echo "🚀 Starting WhatThePooh Server in LOCAL DEVELOPMENT mode (Sandbox APNS)..."
echo "📁 Project root: $PROJECT_ROOT"

# Load local configuration if it exists
LOCAL_CONFIG_FILE="$SCRIPT_DIR/local_config.sh"
if [ -f "$LOCAL_CONFIG_FILE" ]; then
    echo "📋 Loading local configuration from $LOCAL_CONFIG_FILE"
    source "$LOCAL_CONFIG_FILE"
else
    echo "⚠️  No local_config.sh found. Using default values."
    echo "   Copy scripts/local_config.sh.example to scripts/local_config.sh and customize as needed."
fi

# Set default environment variables for local development (can be overridden by local_config.sh)
export APNS_ENV=${APNS_ENV:-"development"}

# DEBUG ONLY — logs every /api/wait-times poll. Remove once wait-time polling
# is proven working (see the matching block in source/wait_times.go).
export LOG_WAIT_TIME_POLLS=${LOG_WAIT_TIME_POLLS:-"true"}

# Set the APNS key path for local development (sandbox)
export APNS_KEY_BASE64=$(base64 -i "$PROJECT_ROOT/keys/AuthKey_MU2W4LLRSY.p8" | tr -d '\n')

echo "📱 APNS Environment: $APNS_ENV (Sandbox)"
echo "🔑 APNS Key ID: $APNS_KEY_ID"
echo "🏢 APNS Team ID: $APNS_TEAM_ID"
echo "📦 Bundle ID: $APNS_BUNDLE_ID"
echo "🌐 WebSocket URL: $WEBSOCKET_URL"
echo ""

# Check if the sandbox key file exists
if [ ! -f "$PROJECT_ROOT/keys/AuthKey_MU2W4LLRSY.p8" ]; then
    echo "❌ Error: Sandbox APNS key file not found at $PROJECT_ROOT/keys/AuthKey_MU2W4LLRSY.p8"
    exit 1
fi

echo "✅ Sandbox APNS key file found"
echo ""

# Run the server
echo "🚀 Starting server with 'go run $PROJECT_ROOT/source'..."
cd "$PROJECT_ROOT"
go run ./source 