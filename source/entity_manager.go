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

// ProcessEntity processes a live entity update from the WebSocket stream.
// Startup ingestion goes through BulkLoad + ReconcileAgainst instead.
func (em *EntityManager) ProcessEntity(entity Entity) {
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

		existingEntity.Status = entity.Status
		existingEntity.LastStatusChange = now

		// Persist the new status to the database
		err := em.db.StoreEntityStatus(existingEntity.EntityID, existingEntity.Name, existingEntity.Status, existingEntity.LastStatusChange)
		if err != nil {
			log.Printf("Failed to store entity status for %s: %v", existingEntity.EntityID, err)
		}
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

// BulkLoad stores REST entities into the manager without publishing notifications
// or writing to the database. Use during startup, before subscribers attach to
// the message bus. ReconcileAgainst is responsible for emitting status-change
// notifications and persisting any new state.
func (em *EntityManager) BulkLoad(entities []Entity) {
	em.mu.Lock()
	defer em.mu.Unlock()

	for _, entity := range entities {
		em.entities.Store(entity.EntityID, entity)
	}
}

// ReconcileAgainst compares the manager's current state (loaded from REST via
// BulkLoad) against a snapshot of the database's last-known state and emits a
// StatusChangeMessage for each entity whose status differs. New entities (in
// REST but not in the snapshot) are persisted without notification. For
// unchanged entities, the snapshot's LastStatusChange is copied forward so the
// "time in current status" duration survives the restart.
func (em *EntityManager) ReconcileAgainst(snapshot map[string]Entity) {
	em.mu.Lock()
	defer em.mu.Unlock()

	var discrepancies, newEntities, unchanged int
	now := time.Now()

	em.entities.Range(func(key, value interface{}) bool {
		current := value.(Entity)
		prior, existedInDB := snapshot[current.EntityID]

		if !existedInDB {
			current.LastStatusChange = now
			current.LastWaitTimeChange = now
			em.entities.Store(current.EntityID, current)
			if err := em.db.StoreEntityStatus(current.EntityID, current.Name, current.Status, now); err != nil {
				log.Printf("Failed to persist new entity %s during reconciliation: %v", current.EntityID, err)
			}
			newEntities++
			return true
		}

		if current.Status != prior.Status {
			messageBus.PublishStatus(StatusChangeMessage{
				EntityID:         current.EntityID,
				EntityName:       current.Name,
				ParkID:           current.ParkID,
				OldStatus:        prior.Status,
				NewStatus:        current.Status,
				OldWaitTime:      prior.WaitTime,
				NewWaitTime:      current.WaitTime,
				Timestamp:        now,
				TimeOfLastStatus: prior.LastStatusChange,
			})
			current.LastStatusChange = now
			em.entities.Store(current.EntityID, current)
			if err := em.db.StoreEntityStatus(current.EntityID, current.Name, current.Status, now); err != nil {
				log.Printf("Failed to persist reconciled status for %s: %v", current.EntityID, err)
			}
			discrepancies++
			return true
		}

		// Status unchanged: preserve the DB's LastStatusChange so the duration counter stays accurate.
		current.LastStatusChange = prior.LastStatusChange
		em.entities.Store(current.EntityID, current)
		unchanged++
		return true
	})

	log.Printf("[STARTUP] Reconciliation complete: %d checked, %d discrepancies published, %d new entities persisted, %d unchanged",
		discrepancies+newEntities+unchanged, discrepancies, newEntities, unchanged)
}
