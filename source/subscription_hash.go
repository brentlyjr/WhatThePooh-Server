package main

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// subscriptionsHash returns a canonical fingerprint of a device's subscription set so the iOS
// client can detect drift (e.g. an update lost when the app was killed) without downloading
// the full list. Canonical form: unique "<parkId>:<entityId>" strings, sorted, joined by "\n",
// SHA-256, lowercase hex. An empty set hashes the empty string.
//
// MUST stay in lockstep with RideSubscriptionManager.subscriptionsHash in the iOS app — both
// sides share the test vectors in subscription_hash_test.go / RideSubscriptionManagerTests.
func subscriptionsHash(subscriptions []NotificationSubscription) string {
	seen := make(map[string]struct{}, len(subscriptions))
	keys := make([]string, 0, len(subscriptions))
	for _, sub := range subscriptions {
		key := sub.ParkID + ":" + sub.EntityID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	sort.Strings(keys)

	sum := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	return hex.EncodeToString(sum[:])
}
