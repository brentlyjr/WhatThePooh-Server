package main

import (
	"time"
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

// DeviceRegistration represents a registered device in the database
type DeviceRegistration struct {
	DeviceToken string    `json:"deviceToken"`
	AppVersion  string    `json:"appVersion"`
	DeviceType  string    `json:"deviceType"`
	Environment string    `json:"environment"` // "development" or "production"
	LastUpdated time.Time `json:"lastUpdated"`
	
	// Optional device information fields
	IOSVersion              *string `json:"iosVersion,omitempty"`
	DeviceName              *string `json:"deviceName,omitempty"`
	SystemName              *string `json:"systemName,omitempty"`
	Language                *string `json:"language,omitempty"`
	Region                  *string `json:"region,omitempty"`
	TimeZone                *string `json:"timeZone,omitempty"`
	DeviceModel             *string `json:"deviceModel,omitempty"`
	DeviceModelIdentifier   *string `json:"deviceModelIdentifier,omitempty"`
}

// APNSMessage represents a tracked APNS message in the database
type APNSMessage struct {
	ID             int64     `json:"id"`
	DeviceToken    string    `json:"deviceToken"`
	Timestamp      time.Time `json:"timestamp"`
	EntityID       string    `json:"entityId"`
	EntityName     string    `json:"entityName"`
	ParkID         string    `json:"parkId"`
	OldStatus      string    `json:"oldStatus"`
	NewStatus      string    `json:"newStatus"`
	OldWaitTime    int       `json:"oldWaitTime"`
	NewWaitTime    int       `json:"newWaitTime"`
	Success        bool      `json:"success"`
	ErrorReason    string    `json:"errorReason,omitempty"`
	NotificationID string    `json:"notificationId"`
	WebsocketTimestamp time.Time `json:"websocketTimestamp"` // UTC timestamp when websocket message was received
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
	NotificationID string `json:"notificationId"`
} 