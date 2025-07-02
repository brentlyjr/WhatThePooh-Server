package main

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Database defines the interface for database operations
type Database interface {
	StoreDeviceToken(registration DeviceRegistration) error
	GetDeviceToken(token string) (*DeviceRegistration, error)
	GetAllDevices() ([]DeviceRegistration, error)
	DeleteDeviceToken(token string) error
	CleanupOldDevices(maxAge time.Duration) error
	StoreAPNSMessage(message APNSMessage) error
	GetAPNSMessages(limit int) ([]APNSMessage, error)
	StoreAPNSReceipt(receipt APNSReceipt) error
	GetAPNSReceipts(limit int) ([]APNSReceipt, error)
}

// SQLiteDB implements the Database interface using SQLite
type SQLiteDB struct {
	db *sql.DB
}

// NewSQLiteDB creates a new SQLite database connection
func NewSQLiteDB() (*SQLiteDB, error) {
	// Use /app/data directory in container, fallback to local directory
	dbPath := "./devices.db"
	if _, err := os.Stat("/app/data"); err == nil {
		dbPath = "/app/data/devices.db"
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	// Create devices table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS devices (
			device_token TEXT PRIMARY KEY,
			app_version TEXT,
			device_type TEXT,
			environment TEXT,
			last_updated TIMESTAMP
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create devices table: %v", err)
	}

	// Add environment column if it doesn't exist (for existing databases)
	_, err = db.Exec(`ALTER TABLE devices ADD COLUMN environment TEXT DEFAULT 'development'`)
	if err != nil {
		// Column might already exist, which is fine
		log.Printf("Note: environment column may already exist: %v", err)
	}

	// Create apns_messages table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS apns_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_token TEXT NOT NULL,
			timestamp TIMESTAMP NOT NULL,
			entity_id TEXT,
			park_id TEXT,
			old_status TEXT,
			new_status TEXT,
			old_wait_time INTEGER,
			new_wait_time INTEGER,
			success BOOLEAN NOT NULL,
			error_reason TEXT,
			FOREIGN KEY (device_token) REFERENCES devices(device_token)
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create apns_messages table: %v", err)
	}

	// Create apns_receipts table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS apns_receipts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_token TEXT NOT NULL,
			client_time TIMESTAMP NOT NULL,
			server_time TIMESTAMP NOT NULL,
			entity_id TEXT,
			park_id TEXT,
			old_status TEXT,
			new_status TEXT,
			old_wait_time INTEGER,
			new_wait_time INTEGER,
			FOREIGN KEY (device_token) REFERENCES devices(device_token)
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create apns_receipts table: %v", err)
	}

	return &SQLiteDB{db: db}, nil
}

// DeviceRegistration represents a registered device in the database
type DeviceRegistration struct {
	DeviceToken string    `json:"deviceToken"`
	AppVersion  string    `json:"appVersion"`
	DeviceType  string    `json:"deviceType"`
	Environment string    `json:"environment"` // "development" or "production"
	LastUpdated time.Time `json:"lastUpdated"`
}

// APNSMessage represents a tracked APNS message in the database
type APNSMessage struct {
	ID          int64     `json:"id"`
	DeviceToken string    `json:"deviceToken"`
	Timestamp   time.Time `json:"timestamp"`
	EntityID    string    `json:"entityId"`
	ParkID      string    `json:"parkId"`
	OldStatus   string    `json:"oldStatus"`
	NewStatus   string    `json:"newStatus"`
	OldWaitTime int       `json:"oldWaitTime"`
	NewWaitTime int       `json:"newWaitTime"`
	Success     bool      `json:"success"`
	ErrorReason string    `json:"errorReason,omitempty"`
}

// APNSReceipt represents a client receipt of an APNS message
type APNSReceipt struct {
	ID          int64     `json:"id"`
	DeviceToken string    `json:"deviceToken"`
	ClientTime  time.Time `json:"clientTime"`
	ServerTime  time.Time `json:"serverTime"`
	EntityID    string    `json:"entityId"`
	ParkID      string    `json:"parkId"`
	OldStatus   string    `json:"oldStatus"`
	NewStatus   string    `json:"newStatus"`
	OldWaitTime int       `json:"oldWaitTime"`
	NewWaitTime int       `json:"newWaitTime"`
}

// StoreDeviceToken saves or updates a device token in the database
func (s *SQLiteDB) StoreDeviceToken(registration DeviceRegistration) error {
	// Always use server time for last_updated
	now := time.Now().UTC()

	_, err := s.db.Exec(`
		INSERT INTO devices (device_token, app_version, device_type, environment, last_updated)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(device_token) DO UPDATE SET
			app_version = excluded.app_version,
			device_type = excluded.device_type,
			environment = excluded.environment,
			last_updated = ?
	`, registration.DeviceToken, registration.AppVersion, registration.DeviceType, registration.Environment, now, now)

	if err != nil {
		return fmt.Errorf("failed to store device token: %v", err)
	}

	return nil
}

// GetDeviceToken retrieves a specific device token
func (s *SQLiteDB) GetDeviceToken(token string) (*DeviceRegistration, error) {
	var device DeviceRegistration
	err := s.db.QueryRow(`
		SELECT device_token, app_version, device_type, environment, last_updated
		FROM devices
		WHERE device_token = ?
	`, token).Scan(&device.DeviceToken, &device.AppVersion, &device.DeviceType, &device.Environment, &device.LastUpdated)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query device: %v", err)
	}

	return &device, nil
}

// GetAllDevices returns all registered devices
func (s *SQLiteDB) GetAllDevices() ([]DeviceRegistration, error) {
	rows, err := s.db.Query(`
		SELECT device_token, app_version, device_type, environment, last_updated
		FROM devices
		ORDER BY last_updated DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query devices: %v", err)
	}
	defer rows.Close()

	var devices []DeviceRegistration
	for rows.Next() {
		var device DeviceRegistration
		err := rows.Scan(&device.DeviceToken, &device.AppVersion, &device.DeviceType, &device.Environment, &device.LastUpdated)
		if err != nil {
			return nil, fmt.Errorf("failed to scan device row: %v", err)
		}
		devices = append(devices, device)
	}

	return devices, nil
}

// DeleteDeviceToken removes a device token from the database
func (s *SQLiteDB) DeleteDeviceToken(token string) error {
	_, err := s.db.Exec("DELETE FROM devices WHERE device_token = ?", token)
	if err != nil {
		return fmt.Errorf("failed to delete device token: %v", err)
	}
	return nil
}

// CleanupOldDevices removes devices that haven't been updated in a while
func (s *SQLiteDB) CleanupOldDevices(maxAge time.Duration) error {
	cutoff := time.Now().UTC().Add(-maxAge)
	_, err := s.db.Exec("DELETE FROM devices WHERE last_updated < ?", cutoff)
	if err != nil {
		return fmt.Errorf("failed to cleanup old devices: %v", err)
	}
	return nil
}

// StoreAPNSMessage saves an APNS message in the database
func (s *SQLiteDB) StoreAPNSMessage(message APNSMessage) error {
	_, err := s.db.Exec(`
		INSERT INTO apns_messages (device_token, timestamp, entity_id, park_id, old_status, new_status, old_wait_time, new_wait_time, success, error_reason)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, message.DeviceToken, message.Timestamp, message.EntityID, message.ParkID, message.OldStatus, message.NewStatus, message.OldWaitTime, message.NewWaitTime, message.Success, message.ErrorReason)

	if err != nil {
		return fmt.Errorf("failed to store APNS message: %v", err)
	}

	return nil
}

// GetAPNSMessages retrieves a limited number of APNS messages from the database
func (s *SQLiteDB) GetAPNSMessages(limit int) ([]APNSMessage, error) {
	rows, err := s.db.Query(`
		SELECT id, device_token, timestamp, entity_id, park_id, old_status, new_status, old_wait_time, new_wait_time, success, error_reason
		FROM apns_messages
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query APNS messages: %v", err)
	}
	defer rows.Close()

	var messages []APNSMessage
	for rows.Next() {
		var message APNSMessage
		err := rows.Scan(&message.ID, &message.DeviceToken, &message.Timestamp, &message.EntityID, &message.ParkID, &message.OldStatus, &message.NewStatus, &message.OldWaitTime, &message.NewWaitTime, &message.Success, &message.ErrorReason)
		if err != nil {
			return nil, fmt.Errorf("failed to scan APNS message row: %v", err)
		}
		messages = append(messages, message)
	}

	return messages, nil
}

// StoreAPNSReceipt saves an APNS receipt in the database
func (s *SQLiteDB) StoreAPNSReceipt(receipt APNSReceipt) error {
	_, err := s.db.Exec(`
		INSERT INTO apns_receipts (device_token, client_time, server_time, entity_id, park_id, old_status, new_status, old_wait_time, new_wait_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, receipt.DeviceToken, receipt.ClientTime, receipt.ServerTime, receipt.EntityID, receipt.ParkID, receipt.OldStatus, receipt.NewStatus, receipt.OldWaitTime, receipt.NewWaitTime)

	if err != nil {
		return fmt.Errorf("failed to store APNS receipt: %v", err)
	}

	return nil
}

// GetAPNSReceipts retrieves a limited number of APNS receipts from the database
func (s *SQLiteDB) GetAPNSReceipts(limit int) ([]APNSReceipt, error) {
	rows, err := s.db.Query(`
		SELECT id, device_token, client_time, server_time, entity_id, park_id, old_status, new_status, old_wait_time, new_wait_time
		FROM apns_receipts
		ORDER BY server_time DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query APNS receipts: %v", err)
	}
	defer rows.Close()

	var receipts []APNSReceipt
	for rows.Next() {
		var receipt APNSReceipt
		err := rows.Scan(&receipt.ID, &receipt.DeviceToken, &receipt.ClientTime, &receipt.ServerTime, &receipt.EntityID, &receipt.ParkID, &receipt.OldStatus, &receipt.NewStatus, &receipt.OldWaitTime, &receipt.NewWaitTime)
		if err != nil {
			return nil, fmt.Errorf("failed to scan APNS receipt row: %v", err)
		}
		receipts = append(receipts, receipt)
	}

	return receipts, nil
} 