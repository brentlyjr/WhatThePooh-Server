#!/bin/bash

# Test script for environment-based APNS functionality
# This script tests device registration and token validation with different environments

SERVER_URL="http://localhost:8080"

echo "🧪 Testing Environment-Based APNS Functionality"
echo "================================================"

# Test 1: Register a development device
echo ""
echo "📱 Test 1: Registering development device..."
DEV_TOKEN_DEV="1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef"
curl -X POST "$SERVER_URL/api/register-device" \
  -H "Content-Type: application/json" \
  -d "{
    \"deviceToken\": \"$DEV_TOKEN_DEV\",
    \"appVersion\": \"1.0.0\",
    \"deviceType\": \"iPhone\",
    \"environment\": \"development\"
  }"

echo ""
echo ""

# Test 2: Register a production device
echo "📱 Test 2: Registering production device..."
DEV_TOKEN_PROD="abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
curl -X POST "$SERVER_URL/api/register-device" \
  -H "Content-Type: application/json" \
  -d "{
    \"deviceToken\": \"$DEV_TOKEN_PROD\",
    \"appVersion\": \"1.0.0\",
    \"deviceType\": \"iPhone\",
    \"environment\": \"production\"
  }"

echo ""
echo ""

# Test 3: Register a device without environment (should default to development)
echo "📱 Test 3: Registering device without environment (should default to development)..."
DEV_TOKEN_DEFAULT="fedcba0987654321fedcba0987654321fedcba0987654321fedcba0987654321"
curl -X POST "$SERVER_URL/api/register-device" \
  -H "Content-Type: application/json" \
  -d "{
    \"deviceToken\": \"$DEV_TOKEN_DEFAULT\",
    \"appVersion\": \"1.0.0\",
    \"deviceType\": \"iPhone\"
  }"

echo ""
echo ""

# Test 4: Check all registered devices
echo "📱 Test 4: Checking all registered devices..."
curl -X GET "$SERVER_URL/api/devices"

echo ""
echo ""

# Test 5: Test device token with development environment
echo "📱 Test 5: Testing device token with development environment..."
curl -X POST "$SERVER_URL/api/test/device-token" \
  -H "Content-Type: application/json" \
  -d "{
    \"deviceToken\": \"$DEV_TOKEN_DEV\",
    \"environment\": \"development\"
  }"

echo ""
echo ""

# Test 6: Test device token with production environment
echo "📱 Test 6: Testing device token with production environment..."
curl -X POST "$SERVER_URL/api/test/device-token" \
  -H "Content-Type: application/json" \
  -d "{
    \"deviceToken\": \"$DEV_TOKEN_PROD\",
    \"environment\": \"production\"
  }"

echo ""
echo ""

# Test 7: Test invalid environment
echo "📱 Test 7: Testing invalid environment..."
curl -X POST "$SERVER_URL/api/register-device" \
  -H "Content-Type: application/json" \
  -d "{
    \"deviceToken\": \"invalid_token_123\",
    \"appVersion\": \"1.0.0\",
    \"deviceType\": \"iPhone\",
    \"environment\": \"invalid\"
  }"

echo ""
echo ""
echo "✅ Environment-based APNS testing completed!"
echo "================================================" 