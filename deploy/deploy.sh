#!/bin/bash

# Azure Container Instance Deployment Script
# This script creates an ACR, builds and pushes the Docker image, and deploys to ACI

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
    print_error "Please create the configuration file with your Azure settings."
    exit 1
fi

source "$CONFIG_FILE"

# Validate required variables
required_vars=(
    "AZURE_SUBSCRIPTION_ID"
    "AZURE_RESOURCE_GROUP" 
    "AZURE_LOCATION"
    "ACR_NAME"
    "ACI_NAME"
    "IMAGE_NAME"
    "APNS_KEY_ID"
    "APNS_TEAM_ID"
    "APNS_BUNDLE_ID"
    "THEMEPARK_API_KEY"
)

for var in "${required_vars[@]}"; do
    if [ -z "${!var}" ] || [[ "${!var}" == *"your-"* ]]; then
        print_error "Required variable $var is not set or contains placeholder value"
        exit 1
    fi
done

print_status "Starting Azure deployment process..."

# Check if Azure CLI is installed
if ! command -v az &> /dev/null; then
    print_error "Azure CLI is not installed. Please install it first."
    exit 1
fi

# Login to Azure (if not already logged in)
print_status "Checking Azure login status..."
if ! az account show &> /dev/null; then
    print_status "Logging in to Azure..."
    az login
fi

# Set the subscription
print_status "Setting Azure subscription..."
az account set --subscription "$AZURE_SUBSCRIPTION_ID"

# Create resource group if it doesn't exist
print_status "Creating resource group '$AZURE_RESOURCE_GROUP'..."
az group create \
    --name "$AZURE_RESOURCE_GROUP" \
    --location "$AZURE_LOCATION" \
    --output none

# Create Azure Container Registry
print_status "Creating Azure Container Registry '$ACR_NAME'..."
az acr create \
    --resource-group "$AZURE_RESOURCE_GROUP" \
    --name "$ACR_NAME" \
    --sku "$ACR_SKU" \
    --admin-enabled true \
    --output none

# Get ACR login server
ACR_LOGIN_SERVER=$(az acr show --name "$ACR_NAME" --resource-group "$AZURE_RESOURCE_GROUP" --query "loginServer" --output tsv)
print_status "ACR Login Server: $ACR_LOGIN_SERVER"

# Login to ACR
print_status "Logging in to Azure Container Registry..."
az acr login --name "$ACR_NAME"

# Build and push Docker image
FULL_IMAGE_NAME="$ACR_LOGIN_SERVER/$IMAGE_NAME:$IMAGE_TAG"
print_status "Building Docker image..."
cd "$SCRIPT_DIR/.."
docker build -t "$FULL_IMAGE_NAME" .

print_status "Pushing image to ACR..."
docker push "$FULL_IMAGE_NAME"

# Get ACR credentials
print_status "Getting ACR credentials..."
ACR_USERNAME=$(az acr credential show --name "$ACR_NAME" --resource-group "$AZURE_RESOURCE_GROUP" --query "username" --output tsv)
ACR_PASSWORD=$(az acr credential show --name "$ACR_NAME" --resource-group "$AZURE_RESOURCE_GROUP" --query "passwords[0].value" --output tsv)

# Create Azure Container Instance
print_status "Creating Azure Container Instance '$ACI_NAME'..."
az container create \
    --resource-group "$AZURE_RESOURCE_GROUP" \
    --name "$ACI_NAME" \
    --image "$FULL_IMAGE_NAME" \
    --os-type Linux \
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
print_status "Getting container instance details..."
ACI_FQDN=$(az container show --resource-group "$AZURE_RESOURCE_GROUP" --name "$ACI_NAME" --query "ipAddress.fqdn" --output tsv)
ACI_IP=$(az container show --resource-group "$AZURE_RESOURCE_GROUP" --name "$ACI_NAME" --query "ipAddress.ip" --output tsv)

print_status "Deployment completed successfully!"
echo ""
print_status "Container Instance Details:"
echo "  Name: $ACI_NAME"
echo "  FQDN: $ACI_FQDN"
echo "  IP Address: $ACI_IP"
echo "  Port: $ACI_PORT"
echo ""
print_status "Your application should be accessible at:"
echo "  http://$ACI_FQDN:$ACI_PORT"
echo "  http://$ACI_IP:$ACI_PORT"
echo ""
print_status "To check logs, run:"
echo "  az container logs --resource-group $AZURE_RESOURCE_GROUP --name $ACI_NAME"
echo ""
print_status "To check status, run:"
echo "  az container show --resource-group $AZURE_RESOURCE_GROUP --name $ACI_NAME" 