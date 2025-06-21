#!/bin/bash
# A script to fetch the latest logs from the Cloud Run service.
set -e

# --- Script Logic ---
# Find the directory where the script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Load configuration from gcp_config.sh
CONFIG_FILE="$SCRIPT_DIR/gcp_config.sh"
if [ ! -f "$CONFIG_FILE" ]; then
    echo "Configuration file not found: $CONFIG_FILE"
    echo "Please copy gcp_config.sh.example to gcp_config.sh and fill in your values."
    exit 1
fi
source "$CONFIG_FILE"

echo "--- Fetching Logs ---"
echo "Project: $PROJECT_ID"
echo "Service: $SERVICE_NAME"
echo "Region:  $REGION"
echo "------------------------------------------------------"

# Fetch and display the last 100 log entries for the Cloud Run service
gcloud logging read "resource.type=\"cloud_run_revision\" AND resource.labels.service_name=\"$SERVICE_NAME\" AND resource.labels.location=\"$REGION\"" \
    --project="$PROJECT_ID" \
    --limit=100 \
    --format="value(timestamp.date(format='%Y-%m-%d %H:%M:%S'), textPayload)" 