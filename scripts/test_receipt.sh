#!/bin/bash

# Test script for APNS receipt endpoint
# This simulates what your iOS app would send when it receives a push notification

echo "Testing APNS Receipt Endpoint..."
echo "=================================="

# Test data that your iOS app would send
curl -X POST http://localhost:8080/api/apns-receipt \
  -H "Content-Type: application/json" \
  -d '{
    "deviceToken": "test-device-token-1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
    "clientTime": "2024-01-15T10:30:00Z",
    "entityId": "f0d4b531-e291-471b-9527-00410c2bbd65",
    "parkId": "ca888437-ebb4-4d50-aed2-d227f7096968",
    "oldStatus": "DOWN",
    "newStatus": "OPERATING",
    "oldWaitTime": 0,
    "newWaitTime": 45
  }'

echo ""
echo ""
echo "Now checking the receipts..."
echo "============================"

# Check the receipts endpoint
curl -X GET "http://localhost:8080/api/apns-receipts?limit=5"

echo ""
echo ""
echo "Test completed!" 