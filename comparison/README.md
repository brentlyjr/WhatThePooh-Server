# WhatThePooh Comparison Program

This program compares the performance of websocket vs polling methods for collecting theme park attraction data from Disneyland Resort.

## Purpose

The comparison program runs two data collection methods simultaneously:

1. **Websocket Connection**: Real-time updates via websocket subscription
2. **Polling**: REST API calls every minute

This allows you to compare:
- How quickly each method detects changes
- Whether any updates are missed by either method
- The timing differences between real-time and polling approaches

## Usage

### Prerequisites

- Go 1.24.3 or later
- Internet connection

### Running the Program

1. **Navigate to the comparison directory:**
   ```bash
   cd comparison
   ```

2. **Install dependencies:**
   ```bash
   go mod tidy
   ```

3. **Run the program:**
   ```bash
   go run main.go
   ```

### Output

The program will output:

- **Websocket updates**: `Websocket connection updated ride XXX from blah to blah`
- **Polling updates**: `Polling updated ride XXX from blah to blah`
- **New rides**: When either method discovers a new attraction
- **Log messages**: Connection status, API calls, and errors

### Example Output

```
2024/01/15 10:30:00 Starting comparison program for Disneyland Resort...
2024/01/15 10:30:01 Connecting to websocket: wss://themeparkswiki.herokuapp.com/v1/live
2024/01/15 10:30:01 Websocket connected successfully
2024/01/15 10:30:01 Subscribed to Disneyland Resort (ID: bfc89fd6-314d-44b4-b89e-df1a89cf991e)
2024/01/15 10:30:01 Fetching data via REST API...
2024/01/15 10:30:02 Polling update complete. Total entities: 45
Websocket connection added new ride Space Mountain (Status: OPERATING, Wait Time: 45)
Polling added new ride Space Mountain (Status: OPERATING, Wait Time: 45)
Websocket connection updated ride Space Mountain wait time from 45 to 50
```

## Configuration

The program is configured to:
- **Park**: Disneyland Resort (ID: bfc89fd6-314d-44b4-b89e-df1a89cf991e)
- **Polling Interval**: 1 minute
- **Websocket URL**: wss://themeparkswiki.herokuapp.com/v1/live
- **REST API**: https://api.themeparks.wiki/v1/entity

## Stopping the Program

Press `Ctrl+C` to stop the program gracefully.

## Notes

- The program runs indefinitely until interrupted
- Both methods maintain separate entity arrays
- Only ATTRACTION entities are processed
- Wait times are extracted from the "STANDBY" queue data
- The program automatically reconnects to websocket if disconnected 