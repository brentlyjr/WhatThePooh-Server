#!/bin/bash

# Azure Container Instance Update Script
# This script builds a new image and updates the existing ACI deployment

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Load configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="$SCRIPT_DIR/azure-config.env"

if [ ! -f "$CONFIG_FILE" ]; then
    print_error "Configuration file not found: $CONFIG_FILE"
    exit 1
fi

source "$CONFIG_FILE"

# Allow override of image tag via command line argument
if [ ! -z "$1" ]; then
    IMAGE_TAG="$1"
    print_status "Using image tag: $IMAGE_TAG"
fi

print_status "Starting Azure Container Instance update..."

# Get ACR login server
ACR_LOGIN_SERVER=$(az acr show --name "$ACR_NAME" --resource-group "$AZURE_RESOURCE_GROUP" --query "loginServer" --output tsv)

# Login to ACR
print_status "Logging in to Azure Container Registry..."
az acr login --name "$ACR_NAME"

# Build and push new Docker image
FULL_IMAGE_NAME="$ACR_LOGIN_SERVER/$IMAGE_NAME:$IMAGE_TAG"
print_status "Building Docker image with tag '$IMAGE_TAG'..."
cd "$SCRIPT_DIR/.."
docker build -t "$FULL_IMAGE_NAME" .

print_status "Pushing image to ACR..."
docker push "$FULL_IMAGE_NAME"

# Delete existing container instance
print_status "Deleting existing container instance..."
az container delete \
    --resource-group "$AZURE_RESOURCE_GROUP" \
    --name "$ACI_NAME" \
    --yes \
    --output none

# Wait a moment for cleanup
print_status "Waiting for cleanup..."
sleep 10

# Get ACR credentials
ACR_USERNAME=$(az acr credential show --name "$ACR_NAME" --resource-group "$AZURE_RESOURCE_GROUP" --query "username" --output tsv)
ACR_PASSWORD=$(az acr credential show --name "$ACR_NAME" --resource-group "$AZURE_RESOURCE_GROUP" --query "passwords[0].value" --output tsv)

# Create new container instance with updated image
print_status "Creating new container instance with updated image..."
az container create \
    --resource-group "$AZURE_RESOURCE_GROUP" \
    --name "$ACI_NAME" \
    --image "$FULL_IMAGE_NAME" \
    --cpu "$ACI_CPU" \
    --memory "$ACI_MEMORY" \
    --registry-login-server "$ACR_LOGIN_SERVER" \
    --registry-username "$ACR_USERNAME" \
    --registry-password "$ACR_PASSWORD" \
    --dns-name-label "$ACI_NAME" \
    --ports "$ACI_PORT" \
    --environment-variables \
        APNS_KEY_PATH="$APNS_KEY_PATH" \
        APNS_KEY_ID="$APNS_KEY_ID" \
        APNS_TEAM_ID="$APNS_TEAM_ID" \
        APNS_BUNDLE_ID="$APNS_BUNDLE_ID" \
        APNS_ENV="$APNS_ENV" \
        WEBSOCKET_URL="$WEBSOCKET_URL" \
        THEMEPARK_API_KEY="$THEMEPARK_API_KEY" \
    --output none

# Get the container instance details
print_status "Getting updated container instance details..."
ACI_FQDN=$(az container show --resource-group "$AZURE_RESOURCE_GROUP" --name "$ACI_NAME" --query "ipAddress.fqdn" --output tsv)
ACI_IP=$(az container show --resource-group "$AZURE_RESOURCE_GROUP" --name "$ACI_NAME" --query "ipAddress.ip" --output tsv)

print_status "Update completed successfully!"
echo ""
print_status "Updated Container Instance Details:"
echo "  Name: $ACI_NAME"
echo "  Image: $FULL_IMAGE_NAME"
echo "  FQDN: $ACI_FQDN"
echo "  IP Address: $ACI_IP"
echo "  Port: $ACI_PORT"
echo ""
print_status "Your application should be accessible at:"
echo "  http://$ACI_FQDN:$ACI_PORT"
echo "  http://$ACI_IP:$ACI_PORT" 