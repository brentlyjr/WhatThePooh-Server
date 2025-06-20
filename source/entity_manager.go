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
	mu       sync.Mutex
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
	em.mu.Lock()
	defer em.mu.Unlock()

	existing, exists := em.entities.Load(entity.EntityID)
	if !exists {
		now := time.Now()
		entity.LastStatusChange = now
		entity.LastWaitTimeChange = now
		em.entities.Store(entity.EntityID, entity)
		return
	}

	// Convert existing to Entity type
	existingEntity := existing.(Entity)

	// Check for status change
	if entity.Status != existingEntity.Status {
		messageBus.PublishStatus(StatusChangeMessage{
			EntityID:    entity.EntityID,
			ParkID:      entity.ParkID,
			OldStatus:   existingEntity.Status,
			NewStatus:   entity.Status,
			OldWaitTime: existingEntity.WaitTime,
			NewWaitTime: entity.WaitTime,
			Timestamp:   time.Now(),
		})
		existingEntity.Status = entity.Status
		existingEntity.LastStatusChange = time.Now()
	}

	// Check for wait time change
	if entity.WaitTime != existingEntity.WaitTime {
		messageBus.PublishWaitTime(WaitTimeMessage{
			EntityID:    entity.EntityID,
			OldWaitTime: existingEntity.WaitTime,
			NewWaitTime: entity.WaitTime,
			Timestamp:   time.Now(),
		})
		existingEntity.WaitTime = entity.WaitTime
		existingEntity.LastWaitTimeChange = time.Now()
	}

	em.entities.Store(entity.EntityID, existingEntity)
} 