package main

import (
	"sync"
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
	EntityID   string       `json:"entityId"`
	Name       string       `json:"name"`
	EntityType string       `json:"entityType"`
	ParkID     string       `json:"parkId"`
	WaitTime   int          `json:"waitTime"`
	Status     EntityStatus `json:"status"`
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
	em.UpdateEntity(entity)
} 