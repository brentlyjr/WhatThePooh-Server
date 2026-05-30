package main

import (
	"log"
	"sync"
	"time"
)

// Reconciler periodically compares the live REST snapshot against the DB's
// last-known entity status and logs discrepancies. It is read-only — it does
// not write to the database or publish status-change notifications. The intent
// is to detect cases where the WebSocket stream dropped a status change while
// disconnected (or otherwise) and we never updated our state.
type Reconciler struct {
	rc       *RestClient
	db       Database
	interval time.Duration
	stop     chan struct{}
	stopOnce sync.Once
}

// NewReconciler constructs a Reconciler. interval must be > 0.
func NewReconciler(rc *RestClient, db Database, interval time.Duration) *Reconciler {
	return &Reconciler{
		rc:       rc,
		db:       db,
		interval: interval,
		stop:     make(chan struct{}),
	}
}

// Start runs the reconciliation loop until Stop is called. Intended to be
// launched as a goroutine.
func (r *Reconciler) Start() {
	log.Printf("[RECONCILE] Starting reconciliation loop, interval=%s", r.interval)
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.runOnce()
		case <-r.stop:
			log.Printf("[RECONCILE] Stopped")
			return
		}
	}
}

// Stop signals the reconciliation loop to exit.
func (r *Reconciler) Stop() {
	r.stopOnce.Do(func() {
		close(r.stop)
	})
}

// runOnce performs a single REST vs DB comparison and logs any discrepancies.
func (r *Reconciler) runOnce() {
	start := time.Now()

	restEntities, err := r.rc.FetchAllEntities()
	if err != nil {
		log.Printf("[RECONCILE] REST fetch failed: %v", err)
		return
	}

	dbSnapshot, err := r.db.GetAllEntityStatuses()
	if err != nil {
		log.Printf("[RECONCILE] DB snapshot failed: %v", err)
		return
	}

	var checked, discrepancies, missingFromDB int
	seen := make(map[string]struct{}, len(restEntities))

	for _, restEntity := range restEntities {
		seen[restEntity.EntityID] = struct{}{}
		checked++

		dbEntity, exists := dbSnapshot[restEntity.EntityID]
		if !exists {
			missingFromDB++
			log.Printf("[RECONCILE] Entity in REST but not in DB: id=%s name=%q park=%q restStatus=%s",
				restEntity.EntityID, restEntity.Name, getParkName(restEntity.ParkID), restEntity.Status)
			continue
		}

		if dbEntity.Status != restEntity.Status {
			discrepancies++
			log.Printf("[RECONCILE] DISCREPANCY: id=%s name=%q park=%q dbStatus=%s restStatus=%s dbLastChange=%s",
				restEntity.EntityID, restEntity.Name, getParkName(restEntity.ParkID),
				dbEntity.Status, restEntity.Status, dbEntity.LastStatusChange.Format(time.RFC3339))
		}
	}

	// Note entities in DB that the REST snapshot did not return. These are
	// usually entities that the API has stopped reporting (e.g., seasonal
	// closures) rather than true drops, so log them only in summary.
	var missingFromREST int
	for entityID := range dbSnapshot {
		if _, ok := seen[entityID]; !ok {
			missingFromREST++
		}
	}

	log.Printf("[RECONCILE] Run complete in %s: checked=%d discrepancies=%d newInREST=%d missingFromREST=%d",
		time.Since(start).Round(time.Millisecond), checked, discrepancies, missingFromDB, missingFromREST)
}
