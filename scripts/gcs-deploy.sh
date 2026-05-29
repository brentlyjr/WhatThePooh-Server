#!/bin/bash
# Sync the local static_assets/ directory to a public GCS bucket so iOS clients
# can fetch manifest.json (and related JSON configs) directly from Cloud Storage,
# bypassing the Cloud Run server.
set -e

# --- Script Logic ---
# Find the directory where the script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"
ASSET_DIR="$PROJECT_ROOT/static_assets"

# Load configuration from gcp_config.sh
CONFIG_FILE="$SCRIPT_DIR/gcp_config.sh"
if [ ! -f "$CONFIG_FILE" ]; then
    echo "Configuration file not found: $CONFIG_FILE"
    echo "Please copy gcp_config.sh.example to gcp_config.sh and fill in your values."
    exit 1
fi
source "$CONFIG_FILE"

if [ -z "$PROJECT_ID" ]; then
    echo "PROJECT_ID is not set in $CONFIG_FILE."
    exit 1
fi

if [ -z "$STATIC_BUCKET_NAME" ]; then
    echo "STATIC_BUCKET_NAME is not set in $CONFIG_FILE."
    exit 1
fi

if [ -z "$REGION" ]; then
    echo "REGION is not set in $CONFIG_FILE."
    exit 1
fi

if [ ! -d "$ASSET_DIR" ]; then
    echo "Static asset directory not found: $ASSET_DIR"
    echo "Create the directory and add your JSON files (e.g. manifest.json) before running this script."
    exit 1
fi

echo "--- Starting Static Asset Deployment ---"
echo "Project: $PROJECT_ID"
echo "Bucket:  gs://$STATIC_BUCKET_NAME"
echo "Region:  $REGION"
echo "Source:  $ASSET_DIR"

# 1. Enable the Cloud Storage API (idempotent)
echo "Enabling Cloud Storage API..."
gcloud services enable storage.googleapis.com --project="$PROJECT_ID"

# 2. Provision the bucket if it does not already exist
if gcloud storage buckets describe "gs://$STATIC_BUCKET_NAME" --project="$PROJECT_ID" >/dev/null 2>&1; then
    echo "Bucket gs://$STATIC_BUCKET_NAME already exists."
else
    echo "Creating bucket gs://$STATIC_BUCKET_NAME in $REGION..."
    gcloud storage buckets create "gs://$STATIC_BUCKET_NAME" \
        --project="$PROJECT_ID" \
        --location="$REGION" \
        --uniform-bucket-level-access
fi

# 3. Ensure the bucket is publicly readable (idempotent — gcloud no-ops if binding exists)
echo "Granting allUsers Storage Object Viewer..."
gcloud storage buckets add-iam-policy-binding "gs://$STATIC_BUCKET_NAME" \
    --project="$PROJECT_ID" \
    --member="allUsers" \
    --role="roles/storage.objectViewer" > /dev/null

# 4. Upload the assets. --gzip-local="json" gzips .json files locally and
#    stores them with Content-Encoding: gzip so GCS serves them compressed.
#    (rsync is avoided here because this gcloud version doesn't accept a
#    store-gzipped flag on rsync — only in-flight, which wouldn't tag the
#    stored object with Content-Encoding: gzip.)
echo "Uploading $ASSET_DIR to gs://$STATIC_BUCKET_NAME..."
gcloud storage cp "$ASSET_DIR"/* "gs://$STATIC_BUCKET_NAME" \
    --project="$PROJECT_ID" \
    --recursive \
    --gzip-local="json"

# 5. Force iOS clients to always fetch a fresh manifest on launch.
if [ -f "$ASSET_DIR/manifest.json" ]; then
    echo "Setting Cache-Control on manifest.json..."
    gcloud storage objects update "gs://$STATIC_BUCKET_NAME/manifest.json" \
        --project="$PROJECT_ID" \
        --cache-control="no-cache, max-age=0"
else
    echo "No manifest.json found in $ASSET_DIR — skipping Cache-Control update."
fi

echo "--- Deployment Complete ---"
echo "Manifest URL: https://storage.googleapis.com/$STATIC_BUCKET_NAME/manifest.json"
