package main

import (
	"sync"
	"testing"
	"time"
)

// resetStatusAlertWindow clears the package-level window. Tests that touch it
// must not use t.Parallel().
func resetStatusAlertWindow() {
	statusAlertWindow.Lock()
	defer statusAlertWindow.Unlock()
	for i := range statusAlertWindow.slots {
		statusAlertWindow.slots[i] = statusAlertSlot{}
	}
}

func utc(hour, min int) time.Time {
	return time.Date(2026, 10, 24, hour, min, 0, 0, time.UTC)
}

func TestStatusAlertsHourlyShape(t *testing.T) {
	resetStatusAlertWindow()
	now := utc(16, 30)
	recordStatusAlertSentAt(now)
	recordStatusAlertSentAt(now)
	recordStatusAlertSentAt(utc(14, 5))

	got := statusAlertsHourlyAt(now)
	if len(got) != 24 {
		t.Fatalf("len = %d, want 24", len(got))
	}
	if got[0].Hour != "2026-10-23T17:00:00Z" {
		t.Errorf("first hour = %s, want 2026-10-23T17:00:00Z", got[0].Hour)
	}
	last := got[23]
	if last.Hour != "2026-10-24T16:00:00Z" || last.Sent != 2 {
		t.Errorf("last = %+v, want 16:00 with 2", last)
	}
	if got[21].Hour != "2026-10-24T14:00:00Z" || got[21].Sent != 1 {
		t.Errorf("got[21] = %+v, want 14:00 with 1", got[21])
	}
	var total uint64
	for _, h := range got {
		total += h.Sent
	}
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
}

func TestStatusAlertsHourRollover(t *testing.T) {
	resetStatusAlertWindow()
	recordStatusAlertSentAt(utc(10, 59))
	recordStatusAlertSentAt(utc(11, 0))

	got := statusAlertsHourlyAt(utc(11, 0))
	if got[22].Hour != "2026-10-24T10:00:00Z" || got[22].Sent != 1 {
		t.Errorf("10:00 bucket = %+v, want 1", got[22])
	}
	if got[23].Hour != "2026-10-24T11:00:00Z" || got[23].Sent != 1 {
		t.Errorf("11:00 bucket = %+v, want 1", got[23])
	}
}

func TestStatusAlertsNonUTCInput(t *testing.T) {
	resetStatusAlertWindow()
	pacific := time.FixedZone("PDT", -7*3600)
	// 03:59 PDT == 10:59 UTC
	recordStatusAlertSentAt(time.Date(2026, 10, 24, 3, 59, 0, 0, pacific))

	got := statusAlertsHourlyAt(utc(11, 0))
	if got[22].Hour != "2026-10-24T10:00:00Z" || got[22].Sent != 1 {
		t.Errorf("10:00 UTC bucket = %+v, want 1", got[22])
	}
}

func TestStatusAlertsSlotReuse(t *testing.T) {
	resetStatusAlertWindow()
	h := utc(9, 15)
	h24 := h.Add(24 * time.Hour) // same slot index
	recordStatusAlertSentAt(h)
	recordStatusAlertSentAt(h24)

	got := statusAlertsHourlyAt(h24)
	if got[23].Sent != 1 {
		t.Errorf("H+24 bucket = %d, want 1 (old slot should reset)", got[23].Sent)
	}
	for _, b := range got {
		if b.Hour == "2026-10-24T09:00:00Z" {
			t.Errorf("hour H should be outside the window, found %+v", b)
		}
	}
}

func TestStatusAlertsLongGap(t *testing.T) {
	resetStatusAlertWindow()
	for i := 0; i < 5; i++ {
		recordStatusAlertSentAt(utc(8, i))
	}
	got := statusAlertsHourlyAt(utc(8, 0).Add(30 * time.Hour))
	for _, b := range got {
		if b.Sent != 0 {
			t.Errorf("bucket %s = %d, want 0 after 30h gap", b.Hour, b.Sent)
		}
	}
}

func TestStatusAlertsConcurrent(t *testing.T) {
	resetStatusAlertWindow()
	const workers, perWorker = 8, 500
	now := utc(12, 0)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				recordStatusAlertSentAt(now)
			}
		}()
	}
	// Concurrent reads alongside writes, for -race.
	for i := 0; i < 50; i++ {
		statusAlertsHourlyAt(now)
	}
	wg.Wait()

	if got := statusAlertsHourlyAt(now)[23].Sent; got != workers*perWorker {
		t.Errorf("sent = %d, want %d", got, workers*perWorker)
	}
}
