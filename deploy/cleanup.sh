#!/bin/bash

# Azure Resource Cleanup Script
# This script removes the Azure Container Instance, Container Registry, and Resource Group

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

print_warning "This script will DELETE the following Azure resources:"
echo "  - Container Instance: $ACI_NAME"
echo "  - Container Registry: $ACR_NAME"
echo "  - Resource Group: $AZURE_RESOURCE_GROUP (and ALL resources within it)"
echo ""

# Confirmation prompt
read -p "Are you sure you want to continue? (type 'yes' to confirm): " confirm
if [ "$confirm" != "yes" ]; then
    print_status "Cleanup cancelled."
    exit 0
fi

print_status "Starting cleanup process..."

# Delete Container Instance
print_status "Deleting Container Instance '$ACI_NAME'..."
if az container show --resource-group "$AZURE_RESOURCE_GROUP" --name "$ACI_NAME" &> /dev/null; then
    az container delete \
        --resource-group "$AZURE_RESOURCE_GROUP" \
        --name "$ACI_NAME" \
        --yes \
        --output none
    print_status "Container Instance deleted."
else
    print_warning "Container Instance '$ACI_NAME' not found or already deleted."
fi

# Delete Container Registry
print_status "Deleting Container Registry '$ACR_NAME'..."
if az acr show --name "$ACR_NAME" --resource-group "$AZURE_RESOURCE_GROUP" &> /dev/null; then
    az acr delete \
        --name "$ACR_NAME" \
        --resource-group "$AZURE_RESOURCE_GROUP" \
        --yes \
        --output none
    print_status "Container Registry deleted."
else
    print_warning "Container Registry '$ACR_NAME' not found or already deleted."
fi

# Ask if user wants to delete the entire resource group
echo ""
read -p "Do you also want to delete the entire resource group '$AZURE_RESOURCE_GROUP'? (type 'yes' to confirm): " confirm_rg
if [ "$confirm_rg" = "yes" ]; then
    print_status "Deleting Resource Group '$AZURE_RESOURCE_GROUP'..."
    if az group show --name "$AZURE_RESOURCE_GROUP" &> /dev/null; then
        az group delete \
            --name "$AZURE_RESOURCE_GROUP" \
            --yes \
            --no-wait \
            --output none
        print_status "Resource Group deletion initiated (running in background)."
    else
        print_warning "Resource Group '$AZURE_RESOURCE_GROUP' not found or already deleted."
    fi
else
    print_status "Resource Group preserved."
fi

print_status "Cleanup completed!"
echo ""
print_status "Remaining resources (if any) can be viewed in the Azure Portal:"
echo "  https://portal.azure.com" 