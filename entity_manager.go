package main

import (
	"sync"
	"time"
)

// EntityStatus represents the possible states of an entity
type EntityStatus string

const (
	StatusClosed        EntityStatus = "CLOSED"
	StatusOperating     EntityStatus = "OPERATING"
	StatusDown          EntityStatus = "DOWN"
	StatusRefurbishment EntityStatus = "REFURBISHMENT"
)

// Entity represents a theme park attraction or other entity
type Entity struct {
	EntityID           string       `json:"entityId"`
	Name              string       `json:"name"`
	EntityType        string       `json:"entityType"`
	ParkID            string       `json:"parkId"`
	WaitTime          int          `json:"waitTime"`
	Status            EntityStatus `json:"status"`
	LastStatusChange  time.Time    `json:"lastStatusChange"`
	LastWaitTimeChange time.Time    `json:"lastWaitTimeChange"`
}

// EntityManager handles the thread-safe storage and updates of entities
type EntityManager struct {
	entities sync.Map
}

// NewEntityManager creates a new EntityManager
func NewEntityManager() *EntityManager {
	return &EntityManager{}
}

// UpdateEntity updates or creates an entity in the manager
func (em *EntityManager) UpdateEntity(entity Entity) {
	em.entities.Store(entity.EntityID, entity)
}

// GetEntity retrieves an entity by its ID
func (em *EntityManager) GetEntity(entityID string) (Entity, bool) {
	if value, ok := em.entities.Load(entityID); ok {
		return value.(Entity), true
	}
	return Entity{}, false
}

// GetAllEntities returns a map of all entities
func (em *EntityManager) GetAllEntities() map[string]Entity {
	result := make(map[string]Entity)
	em.entities.Range(func(key, value interface{}) bool {
		result[key.(string)] = value.(Entity)
		return true
	})
	return result
}

// ProcessEntity processes an entity update from the queue
func (em *EntityManager) ProcessEntity(entity Entity) {
	// Get the current entity if it exists
	currentEntity, exists := em.GetEntity(entity.EntityID)
	
	// Set initial timestamps if this is a new entity
	if !exists {
		entity.LastStatusChange = time.Now().UTC()
		entity.LastWaitTimeChange = time.Now().UTC()
	} else {
		// Copy timestamps from current entity
		entity.LastStatusChange = currentEntity.LastStatusChange
		entity.LastWaitTimeChange = currentEntity.LastWaitTimeChange
		
		// Update timestamps if values have changed
		if currentEntity.Status != entity.Status {
			entity.LastStatusChange = time.Now().UTC()
		}
		if currentEntity.WaitTime != entity.WaitTime {
			entity.LastWaitTimeChange = time.Now().UTC()
		}
	}
	
	em.UpdateEntity(entity)
} 