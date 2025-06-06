#!/bin/bash
# Exit immediately if a command exits with a non-zero status.
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

# --- Secret Configuration ---
# These are the names of the secrets that will be deleted from Google Secret Manager
SECRETS=(
  "APNS_KEY_BASE64"
  "APNS_KEY_ID"
  "APNS_TEAM_ID"
  "THEMEPARK_API_KEY"
)

# --- Script Logic ---
if [ -z "$PROJECT_ID" ]; then
    echo "Google Cloud project ID not set."
    echo "Please set it using 'gcloud config set project YOUR_PROJECT_ID'"
    exit 1
fi

echo "--- Starting Teardown ---"
echo "Project: $PROJECT_ID"
echo "Service: $SERVICE_NAME"
echo "Region:  $REGION"

# 1. Delete the Cloud Run service
echo "Deleting Cloud Run service..."
gcloud run services delete "$SERVICE_NAME" \
    --platform="managed" \
    --region="$REGION" \
    --quiet \
    --project=$PROJECT_ID

# 2. Delete the secrets
read -p "Do you want to delete the secrets from Secret Manager? (y/N) " -n 1 -r
echo # Move to a new line
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Deleting secrets..."
    for SECRET_NAME in "${SECRETS[@]}"; do
        if gcloud secrets describe "$SECRET_NAME" --project="$PROJECT_ID" &>/dev/null; then
            echo "Deleting secret: $SECRET_NAME"
            gcloud secrets delete "$SECRET_NAME" --project="$PROJECT_ID" --quiet
        else
            echo "Secret '$SECRET_NAME' not found. Skipping deletion."
        fi
    done
    echo "Secrets deleted."
else
    echo "Skipping secret deletion."
fi

# 3. Ask to delete the container repository
read -p "Do you want to delete the Artifact Registry repository? (y/N) " -n 1 -r
echo # Move to a new line
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Deleting Artifact Registry repository..."
    gcloud artifacts repositories delete "$SERVICE_NAME" \
        --location="$REGION" \
        --quiet \
        --project=$PROJECT_ID
    echo "Repository deleted."
else
    echo "Skipping repository deletion."
fi

echo "--- Teardown Complete ---" 