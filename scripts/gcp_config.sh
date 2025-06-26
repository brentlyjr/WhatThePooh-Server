#!/bin/bash

# -----------------------------------------------------------------------------
# Google Cloud Project Configuration
#
# Copy this file to gcp_config.sh and fill in your project-specific values.
# The gcp_config.sh file is ignored by git so you can safely store secrets.
# -----------------------------------------------------------------------------

# --- Project Details ---
# Your Google Cloud Project ID
PROJECT_ID="whatthepooh"

# The service name for Cloud Run
SERVICE_NAME="what-the-pooh-server"

# The region to deploy the service in
REGION="us-west1"


# --- APNS Configuration ---
# The path to your local APNS .p8 key file (PRODUCTION for GCP)
LOCAL_APNS_KEY_PATH="keys/AuthKey_9Q5H58H8GX.p8"
APNS_KEY_ID="9Q5H58H8GX"
APNS_ENV="development"
APNS_TEAM_ID="SVFXRTGAKU"
APNS_BUNDLE_ID="com.brentlyjr.WhatThePooh"


# --- Theme Park API Key ---
# Your API key for the Theme Park Wiki
# Replace with your actual key.
THEMEPARK_API_KEY="519dd9c1-cc1e-4d4a-906d-d628cf0250bc"

# The WebSocket URL for the live entity feed.
# WEBSOCKET_URL="wss://api.themeparks.wiki/v1/entity/live"
WEBSOCKET_URL="wss://themeparkswiki.herokuapp.com/v1/live"
