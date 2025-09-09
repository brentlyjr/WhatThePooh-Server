-- Create devices table
CREATE TABLE IF NOT EXISTS devices (
    device_token TEXT PRIMARY KEY,
    app_version TEXT,
    environment TEXT DEFAULT 'development',
    notifications_on BOOLEAN NOT NULL DEFAULT FALSE,
    created_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
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
CREATE INDEX IF NOT EXISTS idx_devices_created_date ON devices(created_date DESC);
CREATE INDEX IF NOT EXISTS idx_devices_last_updated ON devices(last_updated DESC);
CREATE INDEX IF NOT EXISTS idx_devices_notifications_on ON devices(notifications_on) WHERE notifications_on = true;
CREATE INDEX IF NOT EXISTS idx_notification_subscriptions_device_token ON notification_subscriptions(device_token);
CREATE INDEX IF NOT EXISTS idx_notification_subscriptions_entity_id ON notification_subscriptions(entity_id);
CREATE INDEX IF NOT EXISTS idx_notification_subscriptions_park_id ON notification_subscriptions(park_id);
CREATE INDEX IF NOT EXISTS idx_notification_subscriptions_entity_park ON notification_subscriptions(entity_id, park_id);
CREATE INDEX IF NOT EXISTS idx_notification_subscriptions_device_entity_park ON notification_subscriptions(device_token, entity_id, park_id);

-- Enable Row Level Security (RLS) for security
ALTER TABLE devices ENABLE ROW LEVEL SECURITY;
ALTER TABLE notification_subscriptions ENABLE ROW LEVEL SECURITY;

-- Create user_feedback table
CREATE TABLE IF NOT EXISTS user_feedback (
    id SERIAL PRIMARY KEY,
    created_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    device_token TEXT,
    name TEXT,
    email TEXT,
    feedback TEXT CHECK (feedback IS NULL OR length(feedback) <= 1000),
    logs TEXT,
    FOREIGN KEY (device_token) REFERENCES devices(device_token) ON DELETE SET NULL
);

-- Create indexes for user_feedback table
CREATE INDEX IF NOT EXISTS idx_user_feedback_created_date ON user_feedback(created_date DESC);
CREATE INDEX IF NOT EXISTS idx_user_feedback_device_token ON user_feedback(device_token) WHERE device_token IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_user_feedback_email ON user_feedback(email) WHERE email IS NOT NULL;

-- Create policies for anonymous access (since this is a server-to-server connection)
CREATE POLICY "Allow anonymous access to devices" ON devices
    FOR ALL USING (true);

CREATE POLICY "Allow anonymous access to notification_subscriptions" ON notification_subscriptions
    FOR ALL USING (true);

-- Enable RLS and create policy for user_feedback
ALTER TABLE user_feedback ENABLE ROW LEVEL SECURITY;

CREATE POLICY "Allow anonymous access to user_feedback" ON user_feedback
    FOR ALL USING (true); 