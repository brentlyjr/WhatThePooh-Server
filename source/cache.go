package main

import (
	"log"
	"sync"
)

// CachedDB implements the Database interface with local caching
type CachedDB struct {
	db    Database
	cache sync.Map
	mu    sync.RWMutex
}

// NewCachedDB creates a new cached database instance
func NewCachedDB(db Database) *CachedDB {
	cachedDB := &CachedDB{
		db: db,
	}
	
	// Pre-fill cache from database
	if err := cachedDB.LoadFromDatabase(true); err != nil {
		// Log error but don't fail startup
		log.Printf("Warning: Failed to pre-fill cache from database: %v", err)
	}
	
	return cachedDB
}

// LoadFromDatabase loads all devices from the database into the cache
// If shouldLock is true, it will acquire the lock. If false, it assumes the caller already holds the lock.
func (c *CachedDB) LoadFromDatabase(shouldLock bool) error {
	devices, err := c.db.GetAllDevices()
	if err != nil {
		return err
	}

	if shouldLock {
		c.mu.Lock()
		defer c.mu.Unlock()
	}

	for _, device := range devices {
		c.cache.Store(device.DeviceToken, device)
	}

	return nil
}

// StoreDeviceToken saves or updates a device token in both cache and database
func (c *CachedDB) StoreDeviceToken(registration DeviceRegistration) error {
	// Store in database first (this will set the server timestamp)
	if err := c.db.StoreDeviceToken(registration); err != nil {
		return err
	}

	// Get the updated registration from database to get the correct timestamp
	updatedDevice, err := c.db.GetDeviceToken(registration.DeviceToken)
	if err != nil {
		return err
	}

	// Update cache with the device that has the correct server timestamp
	if updatedDevice != nil {
		c.mu.Lock()
		c.cache.Store(registration.DeviceToken, *updatedDevice)
		c.mu.Unlock()
	}

	return nil
}

// GetDeviceToken retrieves a device token from cache first, then database if not found
func (c *CachedDB) GetDeviceToken(token string) (*DeviceRegistration, error) {
	// Try cache first
	c.mu.RLock()
	if value, ok := c.cache.Load(token); ok {
		c.mu.RUnlock()
		device := value.(DeviceRegistration)
		return &device, nil
	}
	c.mu.RUnlock()

	// If not in cache, get from database
	device, err := c.db.GetDeviceToken(token)
	if err != nil {
		return nil, err
	}

	// If found in database, update cache
	if device != nil {
		c.mu.Lock()
		c.cache.Store(token, *device)
		c.mu.Unlock()
	}

	return device, nil
}

// GetAllDevices returns all devices from cache if available, otherwise from database
func (c *CachedDB) GetAllDevices() ([]DeviceRegistration, error) {
	// Check if we have a full cache
	c.mu.RLock()
	var devices []DeviceRegistration
	c.cache.Range(func(key, value interface{}) bool {
		devices = append(devices, value.(DeviceRegistration))
		return true
	})
	c.mu.RUnlock()

	// If cache is empty, load from database
	if len(devices) == 0 {
		var err error
		devices, err = c.db.GetAllDevices()
		if err != nil {
			return nil, err
		}

		// Update cache with all devices
		c.mu.Lock()
		for _, device := range devices {
			c.cache.Store(device.DeviceToken, device)
		}
		c.mu.Unlock()
	}

	return devices, nil
}

// DeleteDeviceToken removes a device token from both cache and database
func (c *CachedDB) DeleteDeviceToken(token string) error {
	// Remove from cache
	c.mu.Lock()
	c.cache.Delete(token)
	c.mu.Unlock()

	// Remove from database
	return c.db.DeleteDeviceToken(token)
}

// UpdateSubscriptions updates subscriptions in the database (no caching for subscriptions)
func (c *CachedDB) UpdateSubscriptions(deviceToken string, subscriptions []NotificationSubscription) error {
	return c.db.UpdateSubscriptions(deviceToken, subscriptions)
} 

// GetDevicesSubscribedToEntity returns devices subscribed to a specific entity/park
func (c *CachedDB) GetDevicesSubscribedToEntity(entityID, parkID string) ([]DeviceRegistration, error) {
	// This method requires database query with joins, so bypass cache
	return c.db.GetDevicesSubscribedToEntity(entityID, parkID)
}

// SetDeviceNotificationState sets the notification state for a device
func (c *CachedDB) SetDeviceNotificationState(deviceToken string, notificationsOn bool) error {
	// Update database
	if err := c.db.SetDeviceNotificationState(deviceToken, notificationsOn); err != nil {
		return err
	}

	// Update cache with the new device state
	device, err := c.db.GetDeviceToken(deviceToken)
	if err != nil {
		return err
	}
	if device != nil {
		c.mu.Lock()
		c.cache.Store(deviceToken, *device)
		c.mu.Unlock()
	}

	return nil
}

// ExpireCache clears all cached data and reloads from the database
func (c *CachedDB) ExpireCache() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Clear all cached data
	c.cache = sync.Map{}
	
	// Reload from database
	return c.LoadFromDatabase(false)
} 