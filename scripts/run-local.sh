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

# Set environment variables for local development
export APNS_ENV="development"
export APNS_KEY_ID="MU2W4LLRSY"
export APNS_TEAM_ID="SVFXRTGAKU"
export APNS_BUNDLE_ID="com.brentlyjr.WhatThePooh"
export WEBSOCKET_URL="wss://themeparkswiki.herokuapp.com/v1/live"
export THEMEPARK_API_KEY="519dd9c1-cc1e-4d4a-906d-d628cf0250bc"

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