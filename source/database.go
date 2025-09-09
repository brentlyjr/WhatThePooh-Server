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
	UpdateSubscriptions(deviceToken string, subscriptions []NotificationSubscription) error
	GetDevicesSubscribedToEntity(entityID, parkID string) ([]DeviceRegistration, error)
	SetDeviceNotificationState(deviceToken string, notificationsOn bool) error
	StoreUserFeedback(feedback UserFeedback) error
	ExpireCache() error
}

// DeviceRegistration represents a registered device in the database
type DeviceRegistration struct {
	DeviceToken string    `json:"deviceToken"`
	AppVersion  string    `json:"appVersion"`
	Environment string    `json:"environment"` // "development" or "production"
	NotificationsOn bool  `json:"notificationsOn"`
	LastUpdated time.Time `json:"lastUpdated"`
	IOSVersion              *string `json:"iosVersion,omitempty"`
	DeviceName              *string `json:"deviceName,omitempty"`
	SystemName              *string `json:"systemName,omitempty"`
	Language                *string `json:"language,omitempty"`
	Region                  *string `json:"region,omitempty"`
	TimeZone                *string `json:"timeZone,omitempty"`
	DeviceModel             *string `json:"deviceModel,omitempty"`
	DeviceModelIdentifier   *string `json:"deviceModelIdentifier,omitempty"`
}





// RideSubscriptionUpdate represents the incoming JSON payload for updating subscriptions
type RideSubscriptionUpdate struct {
	DeviceToken   string                    `json:"deviceToken"`
	SchemaVersion int                       `json:"schemaVersion"`
	Timestamp     time.Time                 `json:"timestamp"`
	Subscriptions []NotificationSubscription `json:"subscriptions"`
}

// NotificationSubscription represents a database subscription record
type NotificationSubscription struct {
	DeviceToken string    `json:"deviceToken"`
	EntityID    string    `json:"entityId"`
	ParkID      string    `json:"parkId"`
	Timestamp   time.Time `json:"timestamp"`
} 

// UserFeedback represents user feedback data
type UserFeedback struct {
	ID          int       `json:"id,omitempty"`
	CreatedDate time.Time `json:"createdDate,omitempty"`
	Name        *string   `json:"name,omitempty"`
	Email       *string   `json:"email,omitempty"`
	Feedback    *string   `json:"feedback,omitempty"`
	Logs        *string   `json:"logs,omitempty"`
}

 