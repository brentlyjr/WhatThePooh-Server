#!/bin/bash

# Azure Container Instance Logs and Status Script
# This script provides easy access to container logs and status information

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

# Function to show usage
show_usage() {
    echo "Usage: $0 [COMMAND]"
    echo ""
    echo "Commands:"
    echo "  logs     Show container logs (default)"
    echo "  status   Show container status"
    echo "  follow   Follow logs in real-time"
    echo "  restart  Restart the container"
    echo "  help     Show this help message"
    echo ""
}

# Default command
COMMAND="${1:-logs}"

case "$COMMAND" in
    "logs")
        print_status "Fetching logs for container '$ACI_NAME'..."
        az container logs \
            --resource-group "$AZURE_RESOURCE_GROUP" \
            --name "$ACI_NAME"
        ;;
    
    "status")
        print_status "Fetching status for container '$ACI_NAME'..."
        az container show \
            --resource-group "$AZURE_RESOURCE_GROUP" \
            --name "$ACI_NAME" \
            --output table
        echo ""
        print_status "Container state details:"
        az container show \
            --resource-group "$AZURE_RESOURCE_GROUP" \
            --name "$ACI_NAME" \
            --query "containers[0].instanceView.currentState" \
            --output table
        ;;
    
    "follow")
        print_status "Following logs for container '$ACI_NAME' (Ctrl+C to stop)..."
        az container logs \
            --resource-group "$AZURE_RESOURCE_GROUP" \
            --name "$ACI_NAME" \
            --follow
        ;;
    
    "restart")
        print_status "Restarting container '$ACI_NAME'..."
        az container restart \
            --resource-group "$AZURE_RESOURCE_GROUP" \
            --name "$ACI_NAME" \
            --output none
        print_status "Container restart initiated."
        ;;
    
    "help")
        show_usage
        ;;
    
    *)
        print_error "Unknown command: $COMMAND"
        show_usage
        exit 1
        ;;
esac 