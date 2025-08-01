-- Create devices table
CREATE TABLE IF NOT EXISTS devices (
    device_token TEXT PRIMARY KEY,
    app_version TEXT,
    device_type TEXT,
    environment TEXT DEFAULT 'development',
    last_updated TIMESTAMPTZ DEFAULT NOW(),
    ios_version TEXT,
    device_name TEXT,
    system_name TEXT,
    language TEXT,
    region TEXT,
    time_zone TEXT,
    device_model TEXT,
    device_model_identifier TEXT
);

-- Create apns_messages table
CREATE TABLE IF NOT EXISTS apns_messages (
    id BIGSERIAL PRIMARY KEY,
    device_token TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    entity_id TEXT,
    entity_name TEXT,
    park_id TEXT,
    old_status TEXT,
    new_status TEXT,
    old_wait_time INTEGER,
    new_wait_time INTEGER,
    success BOOLEAN NOT NULL,
    error_reason TEXT,
    notification_id TEXT,
    websocket_timestamp TIMESTAMPTZ,
    FOREIGN KEY (device_token) REFERENCES devices(device_token) ON DELETE CASCADE
);

-- Create apns_receipts table
CREATE TABLE IF NOT EXISTS apns_receipts (
    id BIGSERIAL PRIMARY KEY,
    device_token TEXT NOT NULL,
    client_time TIMESTAMPTZ NOT NULL,
    server_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    entity_id TEXT,
    park_id TEXT,
    old_status TEXT,
    new_status TEXT,
    old_wait_time INTEGER,
    new_wait_time INTEGER,
    notification_id TEXT,
    FOREIGN KEY (device_token) REFERENCES devices(device_token) ON DELETE CASCADE
);

-- Create notification_subscriptions table
-- Tracks which rides/entities each device wants to receive APNS notifications for
CREATE TABLE IF NOT EXISTS notification_subscriptions (
    device_token TEXT NOT NULL,
    entity_id TEXT NOT NULL,
    park_id TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (device_token, entity_id, park_id),
    FOREIGN KEY (device_token) REFERENCES devices(device_token) ON DELETE CASCADE
);

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_devices_last_updated ON devices(last_updated DESC);
CREATE INDEX IF NOT EXISTS idx_apns_messages_timestamp ON apns_messages(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_apns_messages_device_token ON apns_messages(device_token);
CREATE INDEX IF NOT EXISTS idx_apns_receipts_server_time ON apns_receipts(server_time DESC);
CREATE INDEX IF NOT EXISTS idx_apns_receipts_device_token ON apns_receipts(device_token);
CREATE INDEX IF NOT EXISTS idx_notification_subscriptions_device_token ON notification_subscriptions(device_token);
CREATE INDEX IF NOT EXISTS idx_notification_subscriptions_entity_id ON notification_subscriptions(entity_id);
CREATE INDEX IF NOT EXISTS idx_notification_subscriptions_park_id ON notification_subscriptions(park_id);

-- Enable Row Level Security (RLS) for security
ALTER TABLE devices ENABLE ROW LEVEL SECURITY;
ALTER TABLE apns_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE apns_receipts ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_subscriptions ENABLE ROW LEVEL SECURITY;

-- Create policies for anonymous access (since this is a server-to-server connection)
CREATE POLICY "Allow anonymous access to devices" ON devices
    FOR ALL USING (true);

CREATE POLICY "Allow anonymous access to apns_messages" ON apns_messages
    FOR ALL USING (true);

CREATE POLICY "Allow anonymous access to apns_receipts" ON apns_receipts
    FOR ALL USING (true);

CREATE POLICY "Allow anonymous access to notification_subscriptions" ON notification_subscriptions
    FOR ALL USING (true); 