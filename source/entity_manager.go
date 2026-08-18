package main

import (
	"sort"
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
	// WaitTimeReported distinguishes a genuine 0-minute STANDBY wait from the
	// absence of a STANDBY queue (shows, closed rides, virtual-queue-only
	// attractions), which convertLiveDataEntity also renders as 0.
	WaitTimeReported  bool         `json:"waitTimeReported"`
	Status            EntityStatus `json:"status"`
	LastUpdated       time.Time    `json:"lastUpdated"` // API-reported event time; zero if unknown
	LastStatusChange  time.Time    `json:"lastStatusChange"`
	LastWaitTimeChange time.Time    `json:"lastWaitTimeChange"`
}

// eventTimeOrNow returns the entity's API-reported event time, falling back to
// time.Now() when it is missing and clamping to now if the API clock is ahead
// of ours.
func eventTimeOrNow(e Entity) time.Time {
	now := time.Now()
	if e.LastUpdated.IsZero() || e.LastUpdated.After(now) {
		return now
	}
	return e.LastUpdated
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

// GetAttractionsByPark returns every ATTRACTION currently known for a park,
// sorted by name so the client's board does not reshuffle between polls
// (sync.Map.Range yields keys in unspecified order).
//
// Deliberately does not take em.mu: Range is safe on its own, and that mutex
// serialises the read-modify-write in ProcessEntity — holding it for a full-map
// scan on every poll would stall entity ingestion.
//
// The EntityType check doubles as a "has been seen on the stream" test. Rows
// hydrated from entity_status have an empty ParkID and EntityType, so retired
// rides no longer in the feed are excluded automatically.
func (em *EntityManager) GetAttractionsByPark(parkID string) []Entity {
	var attractions []Entity
	em.entities.Range(func(_, value interface{}) bool {
		entity := value.(Entity)
		if entity.ParkID == parkID && entity.EntityType == "ATTRACTION" {
			attractions = append(attractions, entity)
		}
		return true
	})

	sort.Slice(attractions, func(i, j int) bool {
		if attractions[i].Name != attractions[j].Name {
			return attractions[i].Name < attractions[j].Name
		}
		return attractions[i].EntityID < attractions[j].EntityID
	})

	return attractions
}

// ProcessEntity processes a live entity update from the WebSocket stream.
// Startup ingestion goes through BulkLoad + ReconcileAgainst instead.
func (em *EntityManager) ProcessEntity(entity Entity) {
	em.mu.Lock()
	defer em.mu.Unlock()

	existing, exists := em.entities.Load(entity.EntityID)
	if !exists {
		// First observation: a stale-but-truthful API time is preferable to
		// time.Now() — no notification fires on this path, and it makes later
		// "in this status since" math more accurate.
		eventTime := eventTimeOrNow(entity)
		entity.LastStatusChange = eventTime
		entity.LastWaitTimeChange = eventTime
		em.entities.Store(entity.EntityID, entity)
		// Persist this new entity to the database
		err := em.db.StoreEntityStatus(entity.EntityID, entity.Name, entity.Status, eventTime)
		if err != nil {
			log.Printf("Failed to store new entity status for %s: %v", entity.EntityID, err)
		}
		return
	}

	existingEntity := existing.(Entity)

	// Check for status change
	if entity.Status != existingEntity.Status {
		eventTime := eventTimeOrNow(entity)
		// Monotonic guard: never let a stale API time trail the previous
		// change (pre-migration rows were stamped with time.Now()), which
		// would produce negative "was down for" durations downstream.
		if eventTime.Before(existingEntity.LastStatusChange) {
			eventTime = time.Now()
		}
		messageBus.PublishStatus(StatusChangeMessage{
			EntityID:         entity.EntityID,
			EntityName:       entity.Name,
			ParkID:           entity.ParkID,
			OldStatus:        existingEntity.Status,
			NewStatus:        entity.Status,
			OldWaitTime:      existingEntity.WaitTime,
			NewWaitTime:      entity.WaitTime,
			Timestamp:        eventTime,
			TimeOfLastStatus: existingEntity.LastStatusChange,
		})

		existingEntity.Status = entity.Status
		existingEntity.LastStatusChange = eventTime

		// Persist the new status to the database
		err := em.db.StoreEntityStatus(existingEntity.EntityID, existingEntity.Name, existingEntity.Status, existingEntity.LastStatusChange)
		if err != nil {
			log.Printf("Failed to store entity status for %s: %v", existingEntity.EntityID, err)
		}
	}

	// Check for wait time change. Currently log-only (see message_processor.go)
	// while we size up the volume of these events before pushing them to clients.
	if entity.WaitTime != existingEntity.WaitTime || entity.WaitTimeReported != existingEntity.WaitTimeReported {
		eventTime := eventTimeOrNow(entity)
		messageBus.PublishWaitTime(WaitTimeMessage{
			EntityID:        entity.EntityID,
			EntityName:      entity.Name,
			ParkID:          entity.ParkID,
			Status:          entity.Status,
			OldWaitTime:     existingEntity.WaitTime,
			OldWaitReported: existingEntity.WaitTimeReported,
			NewWaitTime:     entity.WaitTime,
			NewWaitReported: entity.WaitTimeReported,
			Timestamp:       eventTime,
		})
		existingEntity.WaitTime = entity.WaitTime
		existingEntity.WaitTimeReported = entity.WaitTimeReported
		existingEntity.LastWaitTimeChange = eventTime
	}

	// Live data is authoritative for identity fields, and until now nothing
	// refreshed them: rows hydrated from entity_status carry an empty ParkID and
	// EntityType (the table has no such columns), and BulkLoad — the only path
	// that overwrote them — runs once at startup. Reconnect snapshots arrive
	// through this function like any update, so without this copy a DB-hydrated
	// entity stayed unidentifiable for the life of the process, and LastUpdated
	// stayed frozen at snapshot time for every entity.
	existingEntity.Name = entity.Name
	existingEntity.ParkID = entity.ParkID
	existingEntity.EntityType = entity.EntityType
	existingEntity.LastUpdated = entity.LastUpdated

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

// ReconcileAgainst compares the manager's current state (loaded from the WS
// bootstrap snapshot via BulkLoad) against a snapshot of the database's
// last-known state and emits a StatusChangeMessage for each entity whose
// status differs. New entities (in the live snapshot but not in the DB) are
// persisted without notification. For unchanged entities, the DB snapshot's
// LastStatusChange is copied forward so the "time in current status" duration
// survives the restart.
func (em *EntityManager) ReconcileAgainst(snapshot map[string]Entity) {
	em.mu.Lock()
	defer em.mu.Unlock()

	var discrepancies, newEntities, unchanged int

	em.entities.Range(func(key, value interface{}) bool {
		current := value.(Entity)
		prior, existedInDB := snapshot[current.EntityID]

		if !existedInDB {
			eventTime := eventTimeOrNow(current)
			current.LastStatusChange = eventTime
			current.LastWaitTimeChange = eventTime
			em.entities.Store(current.EntityID, current)
			if err := em.db.StoreEntityStatus(current.EntityID, current.Name, current.Status, eventTime); err != nil {
				log.Printf("Failed to persist new entity %s during reconciliation: %v", current.EntityID, err)
			}
			newEntities++
			return true
		}

		if current.Status != prior.Status {
			eventTime := eventTimeOrNow(current)
			// Monotonic guard, same as ProcessEntity: a stale API time must
			// not trail the DB's previous change time.
			if eventTime.Before(prior.LastStatusChange) {
				eventTime = time.Now()
			}
			messageBus.PublishStatus(StatusChangeMessage{
				EntityID:         current.EntityID,
				EntityName:       current.Name,
				ParkID:           current.ParkID,
				OldStatus:        prior.Status,
				NewStatus:        current.Status,
				OldWaitTime:      prior.WaitTime,
				NewWaitTime:      current.WaitTime,
				Timestamp:        eventTime,
				TimeOfLastStatus: prior.LastStatusChange,
			})
			current.LastStatusChange = eventTime
			em.entities.Store(current.EntityID, current)
			if err := em.db.StoreEntityStatus(current.EntityID, current.Name, current.Status, eventTime); err != nil {
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
