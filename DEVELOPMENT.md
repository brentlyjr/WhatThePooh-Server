# Development Guide

This guide explains how to run the WhatThePooh Server in different environments.

## Local Development (Sandbox APNS)

For local development, use the sandbox APNS environment to avoid affecting production users.

### Option 1: Using the run script (Recommended)
```bash
./run-local.sh
```

This script automatically:
- Sets all environment variables for sandbox APNS
- Uses the sandbox key (`keys/AuthKey_MU2W4LLRSY.p8`)
- Starts the server with `go run .`

### Option 2: Manual environment setup
```bash
# Set environment variables
export APNS_ENV="development"
export APNS_KEY_ID="MU2W4LLRSY"
export APNS_TEAM_ID="SVFXRTGAKU"
export APNS_BUNDLE_ID="com.brentlyjr.WhatThePooh"
export WEBSOCKET_URL="wss://themeparkswiki.herokuapp.com/v1/live"
export THEMEPARK_API_KEY="519dd9c1-cc1e-4d4a-906d-d628cf0250bc"
export APNS_KEY_BASE64=$(base64 -i "keys/AuthKey_MU2W4LLRSY.p8" | tr -d '\n')

# Run the server
go run .
```

## Production Deployment (Production APNS)

For production deployment to GCP, use the production APNS environment.

### Deploy to GCP
```bash
cd gcp-deploy
./gcp-deploy.sh
```

This automatically:
- Uses the production key (`keys/AuthKey_AY6CCB64CG.p8`)
- Sets `APNS_ENV="production"`
- Deploys to Google Cloud Run

## Key Differences

| Environment | APNS Key | APNS Environment | Use Case |
|-------------|----------|------------------|----------|
| Local | `AuthKey_MU2W4LLRSY.p8` | Sandbox | Development & Testing |
| GCP | `AuthKey_AY6CCB64CG.p8` | Production | Live Users |

## Troubleshooting

### Device tokens disappearing quickly
- **Local**: This is normal in sandbox environment
- **Production**: Check APNS certificate validity

### APNS connection issues
- Verify key files exist in `keys/` directory
- Check Apple Developer Portal for key validity
- Ensure bundle ID matches your app configuration 