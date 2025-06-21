#!/bin/bash
# Exit immediately if a command exits with a non-zero status.
set -e

# --- Script Logic ---
# Find the directory where the script is located
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$SCRIPT_DIR")"

# Load configuration from gcp_config.sh
CONFIG_FILE="$SCRIPT_DIR/gcp_config.sh"
if [ ! -f "$CONFIG_FILE" ]; then
    echo "Configuration file not found: $CONFIG_FILE"
    echo "Please copy gcp_config.sh.example to gcp_config.sh and fill in your values."
    exit 1
fi
source "$CONFIG_FILE"

# --- Configuration ---
# You can change these variables
SERVICE_NAME="what-the-pooh-server"
REGION="us-west1"
PROJECT_ID="whatthepooh"
LOCAL_APNS_KEY_PATH="$PROJECT_ROOT/keys/AuthKey_AY6CCB64CG.p8"

# --- Secret Configuration ---
# Define secrets and their values from the loaded config
SECRETS=(
  "APNS_KEY_ID"
  "APNS_TEAM_ID"
  "THEMEPARK_API_KEY"
  "APNS_KEY_BASE64" # This one is handled specially below
)
SECRET_VALUES=(
  "$APNS_KEY_ID"
  "$APNS_TEAM_ID"
  "$THEMEPARK_API_KEY"
  "" # This will be populated from the key file
)

# --- Script Logic ---
if [ -z "$PROJECT_ID" ]; then
    echo "Google Cloud project ID not set."
    echo "Please set it using 'gcloud config set project YOUR_PROJECT_ID'"
    exit 1
fi

echo "--- Starting Deployment ---"
echo "Project: $PROJECT_ID"
echo "Service: $SERVICE_NAME"
echo "Region:  $REGION"

# 1. Enable necessary APIs
echo "Enabling Google Cloud services..."
gcloud services enable run.googleapis.com \
    artifactregistry.googleapis.com \
    cloudbuild.googleapis.com \
    secretmanager.googleapis.com \
    --project=$PROJECT_ID

# 2. Check for APNS key file for the base64 secret
if [ ! -f "$LOCAL_APNS_KEY_PATH" ]; then
    echo "APNS key file not found at: $LOCAL_APNS_KEY_PATH"
    echo "Please make sure the path is correct in your gcp_config.sh file."
    exit 1
fi
# Find the index of APNS_KEY_BASE64 and set its value
for i in "${!SECRETS[@]}"; do
   if [[ "${SECRETS[$i]}" == "APNS_KEY_BASE64" ]]; then
       SECRET_VALUES[$i]=$(base64 -i "$LOCAL_APNS_KEY_PATH" | tr -d '\n')
       break
   fi
done

# 3. Create secrets and grant access
echo "Checking and creating/updating secrets in Secret Manager..."
for i in "${!SECRETS[@]}"; do
  SECRET_NAME=${SECRETS[$i]}
  SECRET_VALUE=${SECRET_VALUES[$i]}

  # If a value is empty or a placeholder, prompt the user.
  if [ -z "$SECRET_VALUE" ] || [[ "$SECRET_VALUE" == *"your-"* ]]; then
    echo "The value for secret '$SECRET_NAME' is not set in gcp_config.sh."
    echo -n "Please enter the value now: "
    read -s SECRET_VALUE # -s makes input silent (for passwords/keys)
    echo
  fi

  # Update the array with the (potentially new) value before creating the secret
  SECRET_VALUES[$i]=$SECRET_VALUE

  if ! gcloud secrets describe "$SECRET_NAME" --project="$PROJECT_ID" &>/dev/null; then
    echo "Creating secret: $SECRET_NAME"
    # Create the secret (without a value initially)
    gcloud secrets create "$SECRET_NAME" \
      --replication-policy="automatic" \
      --project="$PROJECT_ID"

    # Add the first version of the secret with the value
    echo -n "$SECRET_VALUE" | gcloud secrets versions add "$SECRET_NAME" --data-file=- --project="$PROJECT_ID"
  else
    # Check if the current value is different from the new value
    CURRENT_VALUE=$(gcloud secrets versions access latest --secret="$SECRET_NAME" --project="$PROJECT_ID" 2>/dev/null || echo "")
    
    if [ "$CURRENT_VALUE" = "$SECRET_VALUE" ]; then
      echo "Secret '$SECRET_NAME' already exists with the same value. Skipping update."
    else
      echo "Secret '$SECRET_NAME' already exists. Updating with new value..."
      # Add a new version of the secret with the updated value
      echo -n "$SECRET_VALUE" | gcloud secrets versions add "$SECRET_NAME" --data-file=- --project="$PROJECT_ID"
    fi
  fi
done

# 4. Grant Cloud Run service account access to secrets
echo "Granting access to secrets for the Cloud Run service..."
# Get the project number and construct the default service account email
PROJECT_NUMBER=$(gcloud projects describe "$PROJECT_ID" --format="value(projectNumber)")
SERVICE_ACCOUNT="${PROJECT_NUMBER}-compute@developer.gserviceaccount.com"

for SECRET_NAME in "${SECRETS[@]}"; do
    echo "Granting service account access to secret: $SECRET_NAME"
    # The command is idempotent; it won't add a duplicate binding.
    gcloud secrets add-iam-policy-binding "$SECRET_NAME" \
        --member="serviceAccount:$SERVICE_ACCOUNT" \
        --role="roles/secretmanager.secretAccessor" \
        --project="$PROJECT_ID" > /dev/null # Suppress verbose output
done

# 5. Create Artifact Registry repository if it doesn't exist
echo "Checking for Artifact Registry repository..."
if ! gcloud artifacts repositories describe "$SERVICE_NAME" --location="$REGION" --project="$PROJECT_ID" &>/dev/null; then
    echo "Creating Artifact Registry repository: $SERVICE_NAME"
    gcloud artifacts repositories create "$SERVICE_NAME" \
        --repository-format="docker" \
        --location="$REGION" \
        --description="Docker repository for $SERVICE_NAME" \
        --project="$PROJECT_ID"
else
    echo "Artifact Registry repository '$SERVICE_NAME' already exists."
fi

# 6. Build the container image using Cloud Build
echo "Building container image with Cloud Build..."
gcloud builds submit --tag "$REGION-docker.pkg.dev/$PROJECT_ID/$SERVICE_NAME/$SERVICE_NAME" --project=$PROJECT_ID "$PROJECT_ROOT/"

# 7. Deploy to Cloud Run
echo "Deploying to Cloud Run..."

# Prepare the secret environment variable arguments for the gcloud command
SECRET_ARGS_STRING=""
for SECRET_NAME in "${SECRETS[@]}"; do
  SECRET_ARGS_STRING+="$SECRET_NAME=$SECRET_NAME:latest,"
done
# Remove the trailing comma from the string
SECRET_ARGS_STRING=${SECRET_ARGS_STRING%,}

# Prepare the environment variable arguments
ENV_VARS="GIN_MODE=release,APNS_ENV=$APNS_ENV,WEBSOCKET_URL=$WEBSOCKET_URL,APNS_BUNDLE_ID=$APNS_BUNDLE_ID"

gcloud run deploy "$SERVICE_NAME" \
    --image="$REGION-docker.pkg.dev/$PROJECT_ID/$SERVICE_NAME/$SERVICE_NAME" \
    --platform="managed" \
    --region="$REGION" \
    --port="8080" \
    --min-instances=1 \
    --allow-unauthenticated \
    --set-secrets="$SECRET_ARGS_STRING" \
    --set-env-vars="$ENV_VARS" \
    --project=$PROJECT_ID

echo "--- Deployment Complete ---"
SERVICE_URL=$(gcloud run services describe "$SERVICE_NAME" --platform="managed" --region="$REGION" --project="$PROJECT_ID" --format="value(status.url)")
echo "Service URL: $SERVICE_URL" 
