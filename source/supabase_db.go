package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SupabaseDB implements the Database interface using direct PostgreSQL connection
type SupabaseDB struct {
	pool *pgxpool.Pool
}

// NewSupabaseDB creates a new Supabase database connection using direct PostgreSQL
func NewSupabaseDB() (*SupabaseDB, error) {
	// For direct connection, we need the database connection string
	// This should be in format: postgresql://postgres:[password]@[host]:[port]/postgres
	dbURL := os.Getenv("SUPABASE_DB_URL")
	
	if dbURL == "" {
		return nil, fmt.Errorf("SUPABASE_DB_URL environment variable is required for direct connection")
	}

	// Create connection pool
	pool, err := pgxpool.New(context.Background(), dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %v", err)
	}

	// Test the connection
	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping database: %v", err)
	}

	log.Printf("Successfully connected to Supabase PostgreSQL database")

	return &SupabaseDB{pool: pool}, nil
}

// StoreDeviceToken saves or updates a device token in the database
func (s *SupabaseDB) StoreDeviceToken(registration DeviceRegistration) error {
	// Always use server time for last_updated
	now := time.Now().UTC()

	query := `
		INSERT INTO devices (device_token, app_version, device_type, environment, last_updated)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (device_token) DO UPDATE SET
			app_version = EXCLUDED.app_version,
			device_type = EXCLUDED.device_type,
			environment = EXCLUDED.environment,
			last_updated = $5
	`

	_, err := s.pool.Exec(context.Background(), query,
		registration.DeviceToken,
		registration.AppVersion,
		registration.DeviceType,
		registration.Environment,
		now,
	)

	if err != nil {
		return fmt.Errorf("failed to store device token: %v", err)
	}

	return nil
}

// GetDeviceToken retrieves a specific device token
func (s *SupabaseDB) GetDeviceToken(token string) (*DeviceRegistration, error) {
	query := `
		SELECT device_token, app_version, device_type, environment, last_updated
		FROM devices
		WHERE device_token = $1
	`

	var device DeviceRegistration
	err := s.pool.QueryRow(context.Background(), query, token).Scan(
		&device.DeviceToken,
		&device.AppVersion,
		&device.DeviceType,
		&device.Environment,
		&device.LastUpdated,
	)

	if err == pgx.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query device: %v", err)
	}

	return &device, nil
}

// GetAllDevices returns all registered devices
func (s *SupabaseDB) GetAllDevices() ([]DeviceRegistration, error) {
	query := `
		SELECT device_token, app_version, device_type, environment, last_updated
		FROM devices
		ORDER BY last_updated DESC
	`

	rows, err := s.pool.Query(context.Background(), query)
	if err != nil {
		return nil, fmt.Errorf("failed to query devices: %v", err)
	}
	defer rows.Close()

	var devices []DeviceRegistration
	for rows.Next() {
		var device DeviceRegistration
		err := rows.Scan(
			&device.DeviceToken,
			&device.AppVersion,
			&device.DeviceType,
			&device.Environment,
			&device.LastUpdated,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan device row: %v", err)
		}
		devices = append(devices, device)
	}

	return devices, nil
}

// DeleteDeviceToken removes a device token from the database
func (s *SupabaseDB) DeleteDeviceToken(token string) error {
	query := `DELETE FROM devices WHERE device_token = $1`

	_, err := s.pool.Exec(context.Background(), query, token)
	if err != nil {
		return fmt.Errorf("failed to delete device token: %v", err)
	}

	return nil
}

// CleanupOldDevices removes devices that haven't been updated in a while
func (s *SupabaseDB) CleanupOldDevices(maxAge time.Duration) error {
	cutoff := time.Now().UTC().Add(-maxAge)
	query := `DELETE FROM devices WHERE last_updated < $1`

	_, err := s.pool.Exec(context.Background(), query, cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup old devices: %v", err)
	}

	return nil
}

// StoreAPNSMessage saves an APNS message in the database
func (s *SupabaseDB) StoreAPNSMessage(message APNSMessage) error {
	query := `
		INSERT INTO apns_messages (
			device_token, timestamp, entity_id, park_id, old_status, new_status,
			old_wait_time, new_wait_time, success, error_reason, notification_id, websocket_timestamp
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
	`

	_, err := s.pool.Exec(context.Background(), query,
		message.DeviceToken,
		message.Timestamp,
		message.EntityID,
		message.ParkID,
		message.OldStatus,
		message.NewStatus,
		message.OldWaitTime,
		message.NewWaitTime,
		message.Success,
		message.ErrorReason,
		message.NotificationID,
		message.WebsocketTimestamp,
	)

	if err != nil {
		return fmt.Errorf("failed to store APNS message: %v", err)
	}

	return nil
}

// GetAPNSMessages retrieves a limited number of APNS messages from the database
func (s *SupabaseDB) GetAPNSMessages(limit int) ([]APNSMessage, error) {
	query := `
		SELECT id, device_token, timestamp, entity_id, park_id, old_status, new_status,
		       old_wait_time, new_wait_time, success, error_reason, notification_id, websocket_timestamp
		FROM apns_messages
		ORDER BY timestamp DESC
		LIMIT $1
	`

	rows, err := s.pool.Query(context.Background(), query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query APNS messages: %v", err)
	}
	defer rows.Close()

	var messages []APNSMessage
	for rows.Next() {
		var message APNSMessage
		err := rows.Scan(
			&message.ID,
			&message.DeviceToken,
			&message.Timestamp,
			&message.EntityID,
			&message.ParkID,
			&message.OldStatus,
			&message.NewStatus,
			&message.OldWaitTime,
			&message.NewWaitTime,
			&message.Success,
			&message.ErrorReason,
			&message.NotificationID,
			&message.WebsocketTimestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan APNS message row: %v", err)
		}
		messages = append(messages, message)
	}

	return messages, nil
}

// StoreAPNSReceipt saves an APNS receipt in the database
func (s *SupabaseDB) StoreAPNSReceipt(receipt APNSReceipt) error {
	query := `
		INSERT INTO apns_receipts (
			device_token, client_time, server_time, entity_id, park_id,
			old_status, new_status, old_wait_time, new_wait_time, notification_id
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	_, err := s.pool.Exec(context.Background(), query,
		receipt.DeviceToken,
		receipt.ClientTime,
		receipt.ServerTime,
		receipt.EntityID,
		receipt.ParkID,
		receipt.OldStatus,
		receipt.NewStatus,
		receipt.OldWaitTime,
		receipt.NewWaitTime,
		receipt.NotificationID,
	)

	if err != nil {
		return fmt.Errorf("failed to store APNS receipt: %v", err)
	}

	return nil
}

// GetAPNSReceipts retrieves a limited number of APNS receipts from the database
func (s *SupabaseDB) GetAPNSReceipts(limit int) ([]APNSReceipt, error) {
	query := `
		SELECT id, device_token, client_time, server_time, entity_id, park_id,
		       old_status, new_status, old_wait_time, new_wait_time, notification_id
		FROM apns_receipts
		ORDER BY server_time DESC
		LIMIT $1
	`

	rows, err := s.pool.Query(context.Background(), query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query APNS receipts: %v", err)
	}
	defer rows.Close()

	var receipts []APNSReceipt
	for rows.Next() {
		var receipt APNSReceipt
		err := rows.Scan(
			&receipt.ID,
			&receipt.DeviceToken,
			&receipt.ClientTime,
			&receipt.ServerTime,
			&receipt.EntityID,
			&receipt.ParkID,
			&receipt.OldStatus,
			&receipt.NewStatus,
			&receipt.OldWaitTime,
			&receipt.NewWaitTime,
			&receipt.NotificationID,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan APNS receipt row: %v", err)
		}
		receipts = append(receipts, receipt)
	}

	return receipts, nil
}

// Close closes the database connection pool
func (s *SupabaseDB) Close() error {
	if s.pool != nil {
		s.pool.Close()
	}
	return nil
} 