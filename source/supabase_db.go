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
		INSERT INTO devices (device_token, app_version, device_type, environment, last_updated, 
			ios_version, device_name, system_name, language, region, time_zone, 
			device_model, device_model_identifier)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (device_token) DO UPDATE SET
			app_version = EXCLUDED.app_version,
			device_type = EXCLUDED.device_type,
			environment = EXCLUDED.environment,
			last_updated = $5,
			ios_version = EXCLUDED.ios_version,
			device_name = EXCLUDED.device_name,
			system_name = EXCLUDED.system_name,
			language = EXCLUDED.language,
			region = EXCLUDED.region,
			time_zone = EXCLUDED.time_zone,
			device_model = EXCLUDED.device_model,
			device_model_identifier = EXCLUDED.device_model_identifier
	`

	_, err := s.pool.Exec(context.Background(), query,
		registration.DeviceToken,
		registration.AppVersion,
		registration.DeviceType,
		registration.Environment,
		now,
		registration.IOSVersion,
		registration.DeviceName,
		registration.SystemName,
		registration.Language,
		registration.Region,
		registration.TimeZone,
		registration.DeviceModel,
		registration.DeviceModelIdentifier,
	)

	if err != nil {
		return fmt.Errorf("failed to store device token: %v", err)
	}

	return nil
}

// GetDeviceToken retrieves a specific device token
func (s *SupabaseDB) GetDeviceToken(token string) (*DeviceRegistration, error) {
	query := `
		SELECT device_token, app_version, device_type, environment, last_updated,
			ios_version, device_name, system_name, language, region, time_zone,
			device_model, device_model_identifier
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
		&device.IOSVersion,
		&device.DeviceName,
		&device.SystemName,
		&device.Language,
		&device.Region,
		&device.TimeZone,
		&device.DeviceModel,
		&device.DeviceModelIdentifier,
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
		SELECT device_token, app_version, device_type, environment, last_updated,
			ios_version, device_name, system_name, language, region, time_zone,
			device_model, device_model_identifier
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
			&device.IOSVersion,
			&device.DeviceName,
			&device.SystemName,
			&device.Language,
			&device.Region,
			&device.TimeZone,
			&device.DeviceModel,
			&device.DeviceModelIdentifier,
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
			device_token, timestamp, entity_id, entity_name, park_id, old_status, new_status,
			old_wait_time, new_wait_time, success, error_reason, notification_id, websocket_timestamp
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	_, err := s.pool.Exec(context.Background(), query,
		message.DeviceToken,
		message.Timestamp,
		message.EntityID,
		message.EntityName,
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
		SELECT id, device_token, timestamp, entity_id, entity_name, park_id, old_status, new_status,
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
			&message.EntityName,
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

// GetSubscriptions retrieves all subscriptions for a specific device token
func (s *SupabaseDB) GetSubscriptions(deviceToken string) ([]NotificationSubscription, error) {
	query := `
		SELECT device_token, entity_id, park_id, timestamp
		FROM notification_subscriptions
		WHERE device_token = $1
		ORDER BY park_id, entity_id
	`

	rows, err := s.pool.Query(context.Background(), query, deviceToken)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscriptions: %v", err)
	}
	defer rows.Close()

	var subscriptions []NotificationSubscription
	for rows.Next() {
		var sub NotificationSubscription
		err := rows.Scan(
			&sub.DeviceToken,
			&sub.EntityID,
			&sub.ParkID,
			&sub.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan subscription row: %v", err)
		}
		subscriptions = append(subscriptions, sub)
	}

	return subscriptions, nil
}

// UpdateSubscriptions updates subscriptions for a device using smart diffing
func (s *SupabaseDB) UpdateSubscriptions(deviceToken string, newSubscriptions []NotificationSubscription) error {
	ctx := context.Background()

	// Start transaction
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %v", err)
	}
	defer tx.Rollback(ctx)

	// Get current subscriptions within transaction
	currentSubs, err := s.getSubscriptionsInTx(tx, deviceToken)
	if err != nil {
		return err
	}

	// Calculate what needs to be added and removed
	toAdd, toRemove := calculateSubscriptionDiff(currentSubs, newSubscriptions)

	// Remove obsolete subscriptions
	if len(toRemove) > 0 {
		err = s.removeSubscriptionsInTx(tx, deviceToken, toRemove)
		if err != nil {
			return err
		}
	}

	// Add new subscriptions
	if len(toAdd) > 0 {
		err = s.addSubscriptionsInTx(tx, toAdd)
		if err != nil {
			return err
		}
	}

	// Commit transaction
	err = tx.Commit(ctx)
	if err != nil {
		return fmt.Errorf("failed to commit transaction: %v", err)
	}

	log.Printf("Updated subscriptions for device %s: +%d -%d (total: %d)", 
		deviceToken, len(toAdd), len(toRemove), len(newSubscriptions))

	return nil
}

// Helper function to get subscriptions within a transaction
func (s *SupabaseDB) getSubscriptionsInTx(tx pgx.Tx, deviceToken string) ([]NotificationSubscription, error) {
	query := `
		SELECT device_token, entity_id, park_id, timestamp
		FROM notification_subscriptions
		WHERE device_token = $1
		ORDER BY park_id, entity_id
	`

	rows, err := tx.Query(context.Background(), query, deviceToken)
	if err != nil {
		return nil, fmt.Errorf("failed to query subscriptions in transaction: %v", err)
	}
	defer rows.Close()

	var subscriptions []NotificationSubscription
	for rows.Next() {
		var sub NotificationSubscription
		err := rows.Scan(
			&sub.DeviceToken,
			&sub.EntityID,
			&sub.ParkID,
			&sub.Timestamp,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan subscription row in transaction: %v", err)
		}
		subscriptions = append(subscriptions, sub)
	}

	return subscriptions, nil
}

// Helper function to remove subscriptions within a transaction
func (s *SupabaseDB) removeSubscriptionsInTx(tx pgx.Tx, deviceToken string, toRemove []NotificationSubscription) error {
	if len(toRemove) == 0 {
		return nil
	}

	// Build query to remove specific subscriptions
	query := `
		DELETE FROM notification_subscriptions 
		WHERE device_token = $1 AND (park_id, entity_id) IN (
	`
	
	args := []interface{}{deviceToken}
	for i, sub := range toRemove {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("($%d, $%d)", i*2+2, i*2+3)
		args = append(args, sub.ParkID, sub.EntityID)
	}
	query += ")"

	_, err := tx.Exec(context.Background(), query, args...)
	if err != nil {
		return fmt.Errorf("failed to remove subscriptions: %v", err)
	}

	return nil
}

// Helper function to add subscriptions within a transaction
func (s *SupabaseDB) addSubscriptionsInTx(tx pgx.Tx, toAdd []NotificationSubscription) error {
	if len(toAdd) == 0 {
		return nil
	}

	// Build batch insert query
	query := `
		INSERT INTO notification_subscriptions (device_token, entity_id, park_id, timestamp)
		VALUES 
	`
	
	args := []interface{}{}
	for i, sub := range toAdd {
		if i > 0 {
			query += ", "
		}
		query += fmt.Sprintf("($%d, $%d, $%d, $%d)", i*4+1, i*4+2, i*4+3, i*4+4)
		args = append(args, sub.DeviceToken, sub.EntityID, sub.ParkID, sub.Timestamp)
	}

	_, err := tx.Exec(context.Background(), query, args...)
	if err != nil {
		return fmt.Errorf("failed to add subscriptions: %v", err)
	}

	return nil
}

// calculateSubscriptionDiff determines what subscriptions need to be added and removed
func calculateSubscriptionDiff(current, new []NotificationSubscription) (toAdd, toRemove []NotificationSubscription) {
	// Convert current subscriptions to a set for fast lookup
	currentSet := make(map[string]bool)
	for _, sub := range current {
		key := sub.ParkID + "|" + sub.EntityID
		currentSet[key] = true
	}

	// Convert new subscriptions to a set and track what's new
	newSet := make(map[string]NotificationSubscription)
	for _, sub := range new {
		key := sub.ParkID + "|" + sub.EntityID
		newSet[key] = sub
	}

	// Find additions (in new but not in current)
	for key, sub := range newSet {
		if !currentSet[key] {
			toAdd = append(toAdd, sub)
		}
	}

	// Find removals (in current but not in new)
	for _, sub := range current {
		key := sub.ParkID + "|" + sub.EntityID
		if _, exists := newSet[key]; !exists {
			toRemove = append(toRemove, sub)
		}
	}

	return toAdd, toRemove
}

// Close closes the database connection pool
func (s *SupabaseDB) Close() error {
	if s.pool != nil {
		s.pool.Close()
	}
	return nil
} 