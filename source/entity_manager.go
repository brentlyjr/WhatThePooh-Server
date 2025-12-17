package main

import (
	"sync"
	"time"
	"log"
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
	db       Database
}

// NewEntityManager creates a new EntityManager
func NewEntityManager(db Database) *EntityManager {
	em := &EntityManager{
		db: db,
	}
	em.loadInitialStatuses()
	return em
}

// loadInitialStatuses loads the last known statuses from the database on startup
func (em *EntityManager) loadInitialStatuses() {
	statuses, err := em.db.GetAllEntityStatuses()
	if err != nil {
		log.Printf("Failed to load initial entity statuses from database: %v", err)
		return
	}

	for entityID, entity := range statuses {
		em.entities.Store(entityID, entity)
	}
	log.Printf("Loaded %d entity statuses from the database", len(statuses))
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

// ProcessEntity processes an entity update.
// isInitial specifies if this is part of the initial data load.
func (em *EntityManager) ProcessEntity(entity Entity, isInitial bool) {
	em.mu.Lock()
	defer em.mu.Unlock()

	existing, exists := em.entities.Load(entity.EntityID)
	if !exists {
		now := time.Now()
		entity.LastStatusChange = now
		entity.LastWaitTimeChange = now
		em.entities.Store(entity.EntityID, entity)
		// Persist this new entity to the database
		err := em.db.StoreEntityStatus(entity.EntityID, entity.Name, entity.Status, now)
		if err != nil {
			log.Printf("Failed to store new entity status for %s: %v", entity.EntityID, err)
		}
		return
	}

	existingEntity := existing.(Entity)

	// Check for status change
	if entity.Status != existingEntity.Status {
		now := time.Now()
		// Only send notifications for changes that happen after initial population,
		// or for discrepancies found during initial population.
		if !isInitial || (isInitial && exists) {

			messageBus.PublishStatus(StatusChangeMessage{
				EntityID:         entity.EntityID,
				EntityName:       entity.Name,
				ParkID:           entity.ParkID,
				OldStatus:        existingEntity.Status,
				NewStatus:        entity.Status,
				OldWaitTime:      existingEntity.WaitTime,
				NewWaitTime:      entity.WaitTime,
				Timestamp:        now,
				TimeOfLastStatus: existingEntity.LastStatusChange,
			})
		}

		existingEntity.Status = entity.Status
		existingEntity.LastStatusChange = now

		// Persist the new status to the database
		err := em.db.StoreEntityStatus(existingEntity.EntityID, existingEntity.Name, existingEntity.Status, existingEntity.LastStatusChange)
		if err != nil {
			log.Printf("Failed to store entity status for %s: %v", existingEntity.EntityID, err)
		}
	} else if isInitial {
		// If it's the initial load and status is the same, ensure other details are up-to-date.
		// We trust the database's LastStatusChange timestamp.
		existingEntity.Name = entity.Name
		existingEntity.EntityType = entity.EntityType
		existingEntity.ParkID = entity.ParkID
	}

	// Check for wait time change
	if entity.WaitTime != existingEntity.WaitTime {
		// TODO: Re-enable when working on wait time functionality
		// messageBus.PublishWaitTime(WaitTimeMessage{
		// 	EntityID:    entity.EntityID,
		// 	OldWaitTime: existingEntity.WaitTime,
		// 	NewWaitTime: entity.WaitTime,
		// 	Timestamp:   time.Now(),
		// })
		existingEntity.WaitTime = entity.WaitTime
		existingEntity.LastWaitTimeChange = time.Now()
	}

	em.entities.Store(entity.EntityID, existingEntity)
} 