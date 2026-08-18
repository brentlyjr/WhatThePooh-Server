package main

import (
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
)

// ---------------------------------------------------------------------------
// DEBUG ONLY — temporary poll logging, remove once wait-time polling is proven.
//
// Off unless LOG_WAIT_TIME_POLLS=true, so it is inert in production even if
// this ships. run-local.sh turns it on. To rip it out entirely, delete this
// block, the `if logWaitTimePolls { … }` block at the end of
// getWaitTimesHandler, and the export in scripts/run-local.sh.
// ---------------------------------------------------------------------------
var logWaitTimePolls = os.Getenv("LOG_WAIT_TIME_POLLS") == "true"

// shortToken trims a device token for debug logs — enough to tell devices
// apart, not enough to be a credential sitting in a log sink.
func shortToken(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8] + "…"
}

// ---------------------------------------------------------------------------

// WaitTimeEntry is a single ride's current wait time, served from the
// EntityManager's in-memory state (no database access). parkId/parkName are
// hoisted to the response envelope since every entry shares them.
type WaitTimeEntry struct {
	EntityID string       `json:"entityId"`
	Name     string       `json:"name"`
	Status   EntityStatus `json:"status"`
	// WaitTime is null when the attraction reports no STANDBY queue — render
	// that as "—", not "0 min". See Entity.WaitTimeReported.
	WaitTime  *int   `json:"waitTime"`
	RideEmoji string `json:"rideEmoji,omitempty"`
	// LastWaitTimeChange is the freshness field ("45 min · updated 2 min ago").
	// LastUpdated is the entity's last API frame time, which advances even when
	// the wait did not.
	LastWaitTimeChange time.Time `json:"lastWaitTimeChange"`
	LastUpdated        time.Time `json:"lastUpdated"`
}

// waitTimeRequest is the POST body. The device token travels in the body
// rather than the URL so it stays out of access logs.
type waitTimeRequest struct {
	DeviceToken string `json:"deviceToken"`
	ParkID      string `json:"parkId"`
}

func toWaitTimeEntry(e Entity) WaitTimeEntry {
	entry := WaitTimeEntry{
		EntityID:           e.EntityID,
		Name:               e.Name,
		Status:             e.Status,
		RideEmoji:          getRideEmoji(e.EntityID),
		LastWaitTimeChange: e.LastWaitTimeChange,
		LastUpdated:        e.LastUpdated,
	}
	if e.WaitTimeReported {
		wait := e.WaitTime
		entry.WaitTime = &wait
	}
	return entry
}

// getWaitTimesHandler returns current wait times for every attraction in a
// park, served entirely from the EntityManager's sync.Map. The device token is
// a "registered app" gate, not park authorization — any registered device can
// read any park, and park data is public.
//
// For a device already in the CachedDB device cache this touches no database
// at all. An unregistered token misses the cache and hits Postgres, since
// CachedDB does not cache nil lookups.
func getWaitTimesHandler(entityManager *EntityManager) fiber.Handler {
	return func(c *fiber.Ctx) error {
		start := time.Now() // DEBUG ONLY (see logWaitTimePolls)

		var req waitTimeRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		if req.DeviceToken == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "deviceToken is required",
			})
		}
		if req.ParkID == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "parkId is required",
			})
		}
		if !isKnownPark(req.ParkID) {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error":  "Unknown parkId",
				"parkId": req.ParkID,
			})
		}

		device, err := db.GetDeviceToken(req.DeviceToken)
		if err != nil {
			log.Printf("Failed to verify device %s for wait times: %v", shortToken(req.DeviceToken), err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to verify device",
			})
		}
		if device == nil {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"error": "Device token not found. Please register device first.",
			})
		}

		attractions := entityManager.GetAttractionsByPark(req.ParkID)

		// make(...) rather than a nil slice so waitTimes serializes as [] and
		// never null.
		waitTimes := make([]WaitTimeEntry, 0, len(attractions))
		for _, entity := range attractions {
			waitTimes = append(waitTimes, toWaitTimeEntry(entity))
		}

		// DEBUG ONLY — delete with the logWaitTimePolls block above.
		if logWaitTimePolls {
			log.Printf("📊 WAIT TIME POLL: device=%s park=%q returned=%d took=%s",
				shortToken(req.DeviceToken), getParkName(req.ParkID), len(waitTimes),
				time.Since(start).Round(time.Millisecond))
		}

		return c.JSON(fiber.Map{
			"parkId":     req.ParkID,
			"parkName":   getParkName(req.ParkID),
			"serverTime": time.Now().UTC(),
			"count":      len(waitTimes),
			"waitTimes":  waitTimes,
		})
	}
}
