package main

import (
	"log"
	"time"
)

func (p *Program) applyWSLocked(attr Attraction, receivedAt time.Time, createPending bool) {
	existing, seen := p.wsState[attr.ID]
	p.wsState[attr.ID] = attr
	if !seen || !createPending {
		return
	}

	if existing.Status != attr.Status {
		p.setPendingStatusLocked(attr, existing.Status, receivedAt)
	}
	if !waitEqual(existing.WaitTime, existing.WaitTimeReported, attr.WaitTime, attr.WaitTimeReported) {
		p.setPendingWaitLocked(attr, existing.WaitTime, existing.WaitTimeReported, receivedAt)
	}
}

func (p *Program) pendingFor(id string) *pendingFields {
	pend := p.pending[id]
	if pend == nil {
		pend = &pendingFields{}
		p.pending[id] = pend
	}
	return pend
}

func (p *Program) cleanupPending(id string) {
	pend := p.pending[id]
	if pend == nil {
		return
	}
	if pend.status == nil && pend.wait == nil {
		delete(p.pending, id)
	}
}

func (p *Program) setPendingStatusLocked(attr Attraction, oldStatus string, receivedAt time.Time) {
	pend := p.pendingFor(attr.ID)
	if pend.status != nil && pend.status.value != attr.Status {
		log.Printf("websocket superseded %s status %s -> %s (%s never seen on REST)",
			attr.Name, pend.status.value, attr.Status, pend.status.value)
		p.supersededTotal++
	}
	pend.status = &pendingStatus{
		value:      attr.Status,
		receivedAt: receivedAt,
		name:       attr.Name,
	}
	log.Printf("websocket update for %s status %s -> %s at %s (lastUpdated %s)",
		attr.Name, oldStatus, attr.Status, clock(receivedAt), lastUpdatedStamp(attr.LastUpdated))
}

func (p *Program) setPendingWaitLocked(attr Attraction, oldWait int, oldReported bool, receivedAt time.Time) {
	pend := p.pendingFor(attr.ID)
	if pend.wait != nil && !waitEqual(pend.wait.waitTime, pend.wait.reported, attr.WaitTime, attr.WaitTimeReported) {
		log.Printf("websocket superseded %s wait time %s -> %s (%s never seen on REST)",
			attr.Name,
			formatWait(pend.wait.waitTime, pend.wait.reported),
			formatWait(attr.WaitTime, attr.WaitTimeReported),
			formatWait(pend.wait.waitTime, pend.wait.reported))
		p.supersededTotal++
	}
	pend.wait = &pendingWait{
		waitTime:   attr.WaitTime,
		reported:   attr.WaitTimeReported,
		receivedAt: receivedAt,
		name:       attr.Name,
	}
	log.Printf("websocket update for %s wait time %s -> %s at %s (lastUpdated %s)",
		attr.Name,
		formatWait(oldWait, oldReported),
		formatWait(attr.WaitTime, attr.WaitTimeReported),
		clock(receivedAt),
		lastUpdatedStamp(attr.LastUpdated))
}

func (p *Program) applyREST(attractions []Attraction, receivedAt time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	newByID := make(map[string]Attraction, len(attractions))
	for _, attr := range attractions {
		newByID[attr.ID] = attr
	}

	if !p.restBaselineDone {
		for id, attr := range newByID {
			p.restState[id] = attr
		}
		p.restBaselineDone = true
		log.Printf("REST baseline complete (%d attractions)", len(p.restState))
	}

	matched, diverged, unmatched := 0, 0, 0

	for id, pend := range p.pending {
		newEnt, inPoll := newByID[id]
		prevEnt, hadPrev := p.restState[id]

		if pend.status != nil {
			name := pend.status.name
			if inPoll && newEnt.Status == pend.status.value {
				log.Printf("polling update for %s status %s at %s (%s, lastUpdated %s)",
					name, newEnt.Status, clock(receivedAt),
					formatLag(receivedAt.Sub(pend.status.receivedAt)),
					lastUpdatedStamp(newEnt.LastUpdated))
				pend.status = nil
				matched++
				p.matchedTotal++
			} else {
				if inPoll && hadPrev && prevEnt.Status != newEnt.Status {
					log.Printf("REST DIVERGED for %s status: websocket wants %s, poll now shows %s (still waiting)",
						name, pend.status.value, newEnt.Status)
					diverged++
					p.divergedTotal++
				}
				pend.status.pollsWaited++
				if pend.status.pollsWaited >= unmatchedPolls {
					lastREST := "missing"
					if inPoll {
						lastREST = newEnt.Status
					} else if hadPrev {
						lastREST = prevEnt.Status
					}
					log.Printf("UNMATCHED for %s status %s (websocket at %s, last REST was %s after %d polls / %s)",
						name, pend.status.value, clock(pend.status.receivedAt), lastREST,
						pend.status.pollsWaited, time.Duration(pend.status.pollsWaited)*pollInterval)
					pend.status = nil
					unmatched++
					p.unmatchedTotal++
				}
			}
		}

		if pend.wait != nil {
			name := pend.wait.name
			if inPoll && waitEqual(pend.wait.waitTime, pend.wait.reported, newEnt.WaitTime, newEnt.WaitTimeReported) {
				log.Printf("polling update for %s wait time %s at %s (%s, lastUpdated %s)",
					name, formatWait(newEnt.WaitTime, newEnt.WaitTimeReported), clock(receivedAt),
					formatLag(receivedAt.Sub(pend.wait.receivedAt)),
					lastUpdatedStamp(newEnt.LastUpdated))
				pend.wait = nil
				matched++
				p.matchedTotal++
			} else {
				if inPoll && hadPrev && !waitEqual(prevEnt.WaitTime, prevEnt.WaitTimeReported, newEnt.WaitTime, newEnt.WaitTimeReported) {
					log.Printf("REST DIVERGED for %s wait time: websocket wants %s, poll now shows %s (still waiting)",
						name,
						formatWait(pend.wait.waitTime, pend.wait.reported),
						formatWait(newEnt.WaitTime, newEnt.WaitTimeReported))
					diverged++
					p.divergedTotal++
				}
				pend.wait.pollsWaited++
				if pend.wait.pollsWaited >= unmatchedPolls {
					lastREST := "missing"
					if inPoll {
						lastREST = formatWait(newEnt.WaitTime, newEnt.WaitTimeReported)
					} else if hadPrev {
						lastREST = formatWait(prevEnt.WaitTime, prevEnt.WaitTimeReported)
					}
					log.Printf("UNMATCHED for %s wait time %s (websocket at %s, last REST was %s after %d polls / %s)",
						name, formatWait(pend.wait.waitTime, pend.wait.reported),
						clock(pend.wait.receivedAt), lastREST,
						pend.wait.pollsWaited, time.Duration(pend.wait.pollsWaited)*pollInterval)
					pend.wait = nil
					unmatched++
					p.unmatchedTotal++
				}
			}
		}

		p.cleanupPending(id)
	}

	for id, attr := range newByID {
		p.restState[id] = attr
	}

	pendingCount := 0
	for _, pend := range p.pending {
		if pend.status != nil {
			pendingCount++
		}
		if pend.wait != nil {
			pendingCount++
		}
	}

	log.Printf("poll complete: matched %d, pending %d, diverged %d, unmatched %d",
		matched, pendingCount, diverged, unmatched)
}

func (p *Program) dumpShutdown() {
	p.mu.Lock()
	defer p.mu.Unlock()

	log.Printf("shutting down: matched=%d unmatched=%d diverged=%d superseded=%d",
		p.matchedTotal, p.unmatchedTotal, p.divergedTotal, p.supersededTotal)

	if len(p.pending) == 0 {
		log.Printf("no pending websocket updates")
		return
	}

	for _, pend := range p.pending {
		if pend.status != nil {
			log.Printf("still pending: %s status %s (websocket at %s, waited %d polls)",
				pend.status.name, pend.status.value, clock(pend.status.receivedAt), pend.status.pollsWaited)
		}
		if pend.wait != nil {
			log.Printf("still pending: %s wait time %s (websocket at %s, waited %d polls)",
				pend.wait.name, formatWait(pend.wait.waitTime, pend.wait.reported),
				clock(pend.wait.receivedAt), pend.wait.pollsWaited)
		}
	}
}
