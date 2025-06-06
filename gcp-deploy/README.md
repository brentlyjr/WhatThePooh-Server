# Google Cloud Platform (GCP) Deployment

This directory contains the script and configuration for deploying the WhatThePooh Server to Google Cloud Run, using Artifact Registry for container storage and Secret Manager for handling credentials.

## Prerequisites

1.  **Google Cloud SDK (`gcloud`)**: Install the SDK from [here](https://cloud.google.com/sdk/docs/install).
2.  **A GCP Project**: You need an active GCP project with billing enabled.
3.  **Authentication**: You must be authenticated with GCP. Run `gcloud auth login` and `gcloud auth application-default login`.
4.  **APNS Key**: Your Apple Push Notification service key (`.p8` file) must be in the parent `keys/` directory.

## Setup

### 1. Configure GCP Project Settings

First, ensure your local `gcloud` CLI is pointing to the correct project:

```bash
gcloud config set project YOUR-PROJECT-ID
```

### 2. Configure Deployment Settings

The deployment script uses a configuration file for all your secrets and application settings.

*   **Copy the example config:**
    ```bash
    cp gcp-deploy/gcp_config.sh.example gcp-deploy/gcp_config.sh
    ```

*   **Edit `gcp-deploy/gcp_config.sh`:**
    Open the newly created `gcp_config.sh` file and replace all placeholder values (e.g., `"your-apns-key-id"`) with your actual credentials and settings. The script will use these values to configure secrets in Secret Manager.

### 3. Make the Script Executable

You only need to do this once.

```bash
chmod +x gcp-deploy/deploy.sh
```

## Deployment

To deploy the application, simply run the main deployment script from the **root directory of the project**:

```bash
./gcp-deploy/deploy.sh
```

This single script handles the entire deployment pipeline:

1.  **Enables APIs**: Activates Cloud Run, Artifact Registry, Cloud Build, and Secret Manager.
2.  **Manages Secrets**: Creates or updates secrets in Google Secret Manager using the values from `gcp_config.sh`.
3.  **Grants Permissions**: Ensures the Cloud Run service has permission to access the necessary secrets.
4.  **Creates Registry**: Creates a new Artifact Registry repository if one doesn't already exist.
5.  **Builds & Pushes Image**: Uses Cloud Build to build the Docker image and push it to your Artifact Registry.
6.  **Deploys to Cloud Run**: Deploys the new container image to Cloud Run, applying all environment variables and mounting the secrets.

At the end of the script, it will print the stable **Service URL** for your application.

## Monitoring

### View Logs in the Console

The easiest way to view logs is in the Google Cloud Console.

1.  Go to the [Cloud Run](https://console.cloud.google.com/run) section of the console.
2.  Click on your `what-the-pooh-server` service.
3.  Navigate to the **LOGS** tab.

### View Logs with `gcloud`

You can also tail logs from your terminal using the `gcloud` CLI.

```bash
gcloud run services logs tail what-the-pooh-server --project YOUR-PROJECT-ID --region us-west1
```

## How It Works

*   **`deploy.sh`**: This is the all-in-one script that orchestrates the entire deployment. It is designed to be idempotent, meaning you can run it repeatedly without causing errors. It will simply update existing resources.
*   **`gcp_config.sh`**: This file contains your secrets and configuration. **It is ignored by Git and should never be committed to source control.**
*   **Secret Manager**: All sensitive values (API keys, etc.) are stored securely in Google Secret Manager, not in environment variables directly. The Cloud Run service is granted secure access to these secrets at runtime.
*   **Cloud Build**: Builds are performed server-side by Cloud Build, which is faster and more consistent than building on a local machine.
*   **Cloud Run**: The serverless platform that runs your container, automatically handling scaling (with a minimum of 1 instance as currently configured). 