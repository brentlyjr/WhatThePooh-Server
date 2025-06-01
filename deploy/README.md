# Azure Container Instance Deployment

This directory contains scripts and configuration for deploying the WhatThePooh Server to Azure Container Instances (ACI) using Azure Container Registry (ACR).

## Prerequisites

1. **Azure CLI**: Install the Azure CLI from [here](https://docs.microsoft.com/en-us/cli/azure/install-azure-cli)
2. **Docker**: Ensure Docker is installed and running
3. **Azure Subscription**: You need an active Azure subscription
4. **APNS Certificate**: Your Apple Push Notification service certificate should be in the `keys/` directory

## Setup

### 1. Configure Azure Settings

Edit the `azure-config.env` file and replace the placeholder values with your actual Azure and application settings:

```bash
# Required Azure settings
AZURE_SUBSCRIPTION_ID=your-actual-subscription-id
AZURE_RESOURCE_GROUP=whatthepooh-rg  # or your preferred name
AZURE_LOCATION=eastus  # or your preferred region

# Required application settings
APNS_KEY_ID=your-actual-apns-key-id
APNS_TEAM_ID=your-actual-team-id
APNS_BUNDLE_ID=your-actual-bundle-id
THEMEPARK_API_KEY=your-actual-api-key
```

### 2. Make Scripts Executable

```bash
chmod +x deploy/*.sh
```

## Deployment

### Initial Deployment

Run the main deployment script to create all Azure resources and deploy your application:

```bash
./deploy/deploy.sh
```

This script will:
1. Create an Azure Resource Group
2. Create an Azure Container Registry (ACR)
3. Build your Docker image
4. Push the image to ACR
5. Create an Azure Container Instance
6. Start your application

### Updating the Deployment

To deploy a new version of your application:

```bash
./deploy/update.sh
```

Or with a specific image tag:

```bash
./deploy/update.sh v1.1.0
```

## Monitoring

### View Logs

```bash
# View current logs
./deploy/logs.sh logs

# Follow logs in real-time
./deploy/logs.sh follow

# View container status
./deploy/logs.sh status

# Restart the container
./deploy/logs.sh restart
```

### Direct Azure CLI Commands

```bash
# View logs
az container logs --resource-group whatthepooh-rg --name whatthepooh-server

# View status
az container show --resource-group whatthepooh-rg --name whatthepooh-server

# Restart container
az container restart --resource-group whatthepooh-rg --name whatthepooh-server
```

## Cleanup

To remove all Azure resources:

```bash
./deploy/cleanup.sh
```

This will ask for confirmation before deleting:
- The Container Instance
- The Container Registry
- Optionally, the entire Resource Group

## Configuration Files

- `azure-config.env`: Main configuration file with Azure and application settings
- `deploy.sh`: Initial deployment script
- `update.sh`: Update/redeploy script
- `logs.sh`: Log viewing and container management script
- `cleanup.sh`: Resource cleanup script

## Environment Variables

The following environment variables are automatically passed to your container:

- `APNS_KEY_PATH`: Path to the APNS certificate in the container
- `APNS_KEY_ID`: Your APNS Key ID
- `APNS_TEAM_ID`: Your Apple Team ID
- `APNS_BUNDLE_ID`: Your app's bundle identifier
- `APNS_ENV`: Environment (development/production)
- `WEBSOCKET_URL`: WebSocket API URL
- `THEMEPARK_API_KEY`: API key for external services

## Troubleshooting

### Common Issues

1. **ACR Name Already Exists**: Azure Container Registry names must be globally unique. Change the `ACR_NAME` in `azure-config.env`.

2. **Authentication Issues**: Make sure you're logged in to Azure CLI:
   ```bash
   az login
   az account set --subscription YOUR_SUBSCRIPTION_ID
   ```

3. **Docker Build Fails**: Ensure Docker is running and you have the necessary files in the project root.

4. **Container Won't Start**: Check the logs:
   ```bash
   ./deploy/logs.sh logs
   ```

5. **Environment Variables**: Verify all required environment variables are set in `azure-config.env`.

### Getting Help

- View container logs: `./deploy/logs.sh logs`
- Check container status: `./deploy/logs.sh status`
- Azure CLI documentation: https://docs.microsoft.com/en-us/cli/azure/
- Azure Container Instances documentation: https://docs.microsoft.com/en-us/azure/container-instances/

## Costs

Azure Container Instances charges based on:
- vCPU usage (per second)
- Memory usage (per second)
- Data transfer

The default configuration (1 vCPU, 1.5GB memory) typically costs a few dollars per month for moderate usage.

## Security Notes

- The `azure-config.env` file contains sensitive information. Never commit it to version control.
- Consider using Azure Key Vault for production secrets.
- The deployment uses admin credentials for ACR. For production, consider using service principals. 