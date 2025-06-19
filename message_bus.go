package main

import (
    "log"
    "sync"
    "time"
)

// Message types
type StatusChangeMessage struct {
    EntityID      string
    ParkID        string
    OldStatus     EntityStatus
    NewStatus     EntityStatus
    OldWaitTime   int
    NewWaitTime   int
    Timestamp     time.Time
}

type WaitTimeMessage struct {
    EntityID      string
    OldWaitTime   int
    NewWaitTime   int
    Timestamp     time.Time
}

// MessageBus handles pub/sub for both status and wait time messages
type MessageBus struct {
    statusSubscribers    []chan StatusChangeMessage
    waitTimeSubscribers  []chan WaitTimeMessage
    mu                  sync.RWMutex
}

var (
    // Global MessageBus instance
    messageBus = NewMessageBus()
)

func NewMessageBus() *MessageBus {
    return &MessageBus{
        statusSubscribers:   make([]chan StatusChangeMessage, 0),
        waitTimeSubscribers: make([]chan WaitTimeMessage, 0),
    }
}

// Subscribe to status changes
func (mb *MessageBus) SubscribeStatus() chan StatusChangeMessage {
    mb.mu.Lock()
    defer mb.mu.Unlock()
    
    ch := make(chan StatusChangeMessage, 100)
    mb.statusSubscribers = append(mb.statusSubscribers, ch)
    return ch
}

// Subscribe to wait time changes
func (mb *MessageBus) SubscribeWaitTime() chan WaitTimeMessage {
    mb.mu.Lock()
    defer mb.mu.Unlock()
    
    ch := make(chan WaitTimeMessage, 100)
    mb.waitTimeSubscribers = append(mb.waitTimeSubscribers, ch)
    return ch
}

// Publish status change
func (mb *MessageBus) PublishStatus(msg StatusChangeMessage) {
    mb.mu.RLock()
    defer mb.mu.RUnlock()
    
    for _, ch := range mb.statusSubscribers {
        select {
        case ch <- msg:
            // Message sent successfully
        default:
            log.Printf("Status subscriber channel full, dropping message for entity %s", msg.EntityID)
        }
    }
}

// Publish wait time change
func (mb *MessageBus) PublishWaitTime(msg WaitTimeMessage) {
    mb.mu.RLock()
    defer mb.mu.RUnlock()
    
    for _, ch := range mb.waitTimeSubscribers {
        select {
        case ch <- msg:
            // Message sent successfully
        default:
            log.Printf("Wait time subscriber channel full, dropping message for entity %s", msg.EntityID)
        }
    }
} 