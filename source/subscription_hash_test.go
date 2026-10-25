package main

import "testing"

func sub(park, entity string) NotificationSubscription {
	return NotificationSubscription{ParkID: park, EntityID: entity}
}

// Shared vectors — the iOS RideSubscriptionManagerTests assert the same values.
const (
	emptySetHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	threeSubHash = "a06612bc91e4e9f58bb09b5c3ec7beed6d01a026dc09cf12578fd3836f24863b"
)

func TestSubscriptionsHashEmpty(t *testing.T) {
	if got := subscriptionsHash(nil); got != emptySetHash {
		t.Fatalf("empty hash = %s, want %s", got, emptySetHash)
	}
}

func TestSubscriptionsHashKnownVector(t *testing.T) {
	subs := []NotificationSubscription{sub("parkA", "ride1"), sub("parkA", "ride2"), sub("parkB", "ride3")}
	if got := subscriptionsHash(subs); got != threeSubHash {
		t.Fatalf("hash = %s, want %s", got, threeSubHash)
	}
}

func TestSubscriptionsHashOrderAndDuplicatesIgnored(t *testing.T) {
	subs := []NotificationSubscription{
		sub("parkB", "ride3"), sub("parkA", "ride2"), sub("parkA", "ride1"), sub("parkA", "ride2"),
	}
	if got := subscriptionsHash(subs); got != threeSubHash {
		t.Fatalf("hash = %s, want %s", got, threeSubHash)
	}
}

func TestSubscriptionsHashDiffersOnChange(t *testing.T) {
	subs := []NotificationSubscription{sub("parkA", "ride1"), sub("parkA", "ride2")}
	if subscriptionsHash(subs) == threeSubHash {
		t.Fatal("hash should change when a subscription is removed")
	}
}
