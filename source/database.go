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
			last_updated TIMESTAMP
		)
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to create devices table: %v", err)
	}

	return &SQLiteDB{db: db}, nil
}

// DeviceRegistration represents a registered device in the database
type DeviceRegistration struct {
	DeviceToken string    `json:"deviceToken"`
	AppVersion  string    `json:"appVersion"`
	DeviceType  string    `json:"deviceType"`
	LastUpdated time.Time `json:"lastUpdated"`
}

// StoreDeviceToken saves or updates a device token in the database
func (s *SQLiteDB) StoreDeviceToken(registration DeviceRegistration) error {
	registration.LastUpdated = time.Now().UTC()

	_, err := s.db.Exec(`
		INSERT INTO devices (device_token, app_version, device_type, last_updated)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(device_token) DO UPDATE SET
			app_version = excluded.app_version,
			device_type = excluded.device_type,
			last_updated = excluded.last_updated
	`, registration.DeviceToken, registration.AppVersion, registration.DeviceType, registration.LastUpdated)

	if err != nil {
		return fmt.Errorf("failed to store device token: %v", err)
	}

	return nil
}

// GetDeviceToken retrieves a specific device token
func (s *SQLiteDB) GetDeviceToken(token string) (*DeviceRegistration, error) {
	var device DeviceRegistration
	err := s.db.QueryRow(`
		SELECT device_token, app_version, device_type, last_updated
		FROM devices
		WHERE device_token = ?
	`, token).Scan(&device.DeviceToken, &device.AppVersion, &device.DeviceType, &device.LastUpdated)

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
		SELECT device_token, app_version, device_type, last_updated
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
		err := rows.Scan(&device.DeviceToken, &device.AppVersion, &device.DeviceType, &device.LastUpdated)
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