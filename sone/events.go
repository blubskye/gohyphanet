// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package sone

import (
	"sync"
)

// EventType represents the type of Sone event
type EventType int

const (
	// Sone events
	EventSoneDiscovered EventType = iota
	EventSoneUpdated
	EventSoneRemoved
	EventLocalSoneAdded
	EventLocalSoneRemoved
	EventSoneInserting
	EventSoneInserted
	EventSoneInsertAborted

	// Post events
	EventNewPostFound
	EventPostRemoved
	EventPostMarkedKnown

	// Reply events
	EventNewReplyFound
	EventReplyRemoved
	EventReplyMarkedKnown

	// Image events
	EventImageInsertStarted
	EventImageInsertFinished
	EventImageInsertFailed

	// Notification events
	EventMentionDetected
)

// Event represents a Sone event
type Event struct {
	Type    EventType
	Sone    *Sone
	Post    *Post
	Reply   *PostReply
	Image   *Image
	Message string
	Data    interface{}
}

// EventHandler is a function that handles events
type EventHandler func(Event)

// EventBus provides pub/sub functionality for Sone events
type EventBus struct {
	mu          sync.RWMutex
	handlers    map[EventType][]EventHandler
	allHandlers []EventHandler
	eventChan   chan Event
	running     bool
	wg          sync.WaitGroup
}

// NewEventBus creates a new event bus
func NewEventBus() *EventBus {
	return &EventBus{
		handlers:    make(map[EventType][]EventHandler),
		allHandlers: make([]EventHandler, 0),
		eventChan:   make(chan Event, 100),
	}
}

// Subscribe registers a handler for a specific event type
func (eb *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.handlers[eventType] = append(eb.handlers[eventType], handler)
}

// SubscribeAll registers a handler for all event types
func (eb *EventBus) SubscribeAll(handler EventHandler) {
	eb.mu.Lock()
	defer eb.mu.Unlock()

	eb.allHandlers = append(eb.allHandlers, handler)
}

// Publish sends an event to all subscribed handlers
func (eb *EventBus) Publish(event Event) {
	if eb.running {
		eb.eventChan <- event
	} else {
		eb.dispatchEvent(event)
	}
}

// Start begins async event processing
func (eb *EventBus) Start() {
	eb.mu.Lock()
	if eb.running {
		eb.mu.Unlock()
		return
	}
	eb.running = true
	eb.mu.Unlock()

	eb.wg.Add(1)
	go func() {
		defer eb.wg.Done()
		for event := range eb.eventChan {
			eb.dispatchEvent(event)
		}
	}()
}

// Stop stops async event processing
func (eb *EventBus) Stop() {
	eb.mu.Lock()
	if !eb.running {
		eb.mu.Unlock()
		return
	}
	eb.running = false
	eb.mu.Unlock()

	close(eb.eventChan)
	eb.wg.Wait()
}

func (eb *EventBus) dispatchEvent(event Event) {
	eb.mu.RLock()
	defer eb.mu.RUnlock()

	// Call type-specific handlers
	if handlers, ok := eb.handlers[event.Type]; ok {
		for _, handler := range handlers {
			handler(event)
		}
	}

	// Call all-event handlers
	for _, handler := range eb.allHandlers {
		handler(event)
	}
}

// Helper methods for publishing specific events

// PublishSoneDiscovered publishes a new Sone discovered event
func (eb *EventBus) PublishSoneDiscovered(sone *Sone) {
	eb.Publish(Event{
		Type: EventSoneDiscovered,
		Sone: sone,
	})
}

// PublishSoneUpdated publishes a Sone updated event
func (eb *EventBus) PublishSoneUpdated(sone *Sone) {
	eb.Publish(Event{
		Type: EventSoneUpdated,
		Sone: sone,
	})
}

// PublishSoneInserting publishes a Sone inserting event
func (eb *EventBus) PublishSoneInserting(sone *Sone) {
	eb.Publish(Event{
		Type: EventSoneInserting,
		Sone: sone,
	})
}

// PublishSoneInserted publishes a Sone inserted event
func (eb *EventBus) PublishSoneInserted(sone *Sone, edition int64) {
	eb.Publish(Event{
		Type: EventSoneInserted,
		Sone: sone,
		Data: edition,
	})
}

// PublishSoneInsertAborted publishes a Sone insert aborted event
func (eb *EventBus) PublishSoneInsertAborted(sone *Sone, reason string) {
	eb.Publish(Event{
		Type:    EventSoneInsertAborted,
		Sone:    sone,
		Message: reason,
	})
}

// PublishNewPostFound publishes a new post found event
func (eb *EventBus) PublishNewPostFound(post *Post) {
	eb.Publish(Event{
		Type: EventNewPostFound,
		Post: post,
	})
}

// PublishPostRemoved publishes a post removed event
func (eb *EventBus) PublishPostRemoved(post *Post) {
	eb.Publish(Event{
		Type: EventPostRemoved,
		Post: post,
	})
}

// PublishNewReplyFound publishes a new reply found event
func (eb *EventBus) PublishNewReplyFound(reply *PostReply) {
	eb.Publish(Event{
		Type:  EventNewReplyFound,
		Reply: reply,
	})
}

// PublishReplyRemoved publishes a reply removed event
func (eb *EventBus) PublishReplyRemoved(reply *PostReply) {
	eb.Publish(Event{
		Type:  EventReplyRemoved,
		Reply: reply,
	})
}

// PublishMentionDetected publishes a mention detected event
func (eb *EventBus) PublishMentionDetected(post *Post, mentionedSoneID string) {
	eb.Publish(Event{
		Type:    EventMentionDetected,
		Post:    post,
		Message: mentionedSoneID,
	})
}

// Notification represents a user notification
type Notification struct {
	ID          string
	Type        string
	Text        string
	CreatedTime int64
	Dismissable bool
	Elements    []interface{}
}

// NotificationManager manages user notifications
type NotificationManager struct {
	mu            sync.RWMutex
	notifications map[string]*Notification
	eventBus      *EventBus
}

// NewNotificationManager creates a new notification manager
func NewNotificationManager(eventBus *EventBus) *NotificationManager {
	nm := &NotificationManager{
		notifications: make(map[string]*Notification),
		eventBus:      eventBus,
	}

	// Subscribe to events that generate notifications
	eventBus.Subscribe(EventNewPostFound, nm.handleNewPost)
	eventBus.Subscribe(EventNewReplyFound, nm.handleNewReply)
	eventBus.Subscribe(EventSoneDiscovered, nm.handleNewSone)
	eventBus.Subscribe(EventMentionDetected, nm.handleMention)

	return nm
}

func (nm *NotificationManager) handleNewPost(event Event) {
	// Add to new posts notification
	nm.mu.Lock()
	defer nm.mu.Unlock()

	notif, ok := nm.notifications["new-posts"]
	if !ok {
		notif = &Notification{
			ID:          "new-posts",
			Type:        "new-posts",
			Dismissable: true,
			Elements:    make([]interface{}, 0),
		}
		nm.notifications["new-posts"] = notif
	}

	notif.Elements = append(notif.Elements, event.Post)
}

func (nm *NotificationManager) handleNewReply(event Event) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	notif, ok := nm.notifications["new-replies"]
	if !ok {
		notif = &Notification{
			ID:          "new-replies",
			Type:        "new-replies",
			Dismissable: true,
			Elements:    make([]interface{}, 0),
		}
		nm.notifications["new-replies"] = notif
	}

	notif.Elements = append(notif.Elements, event.Reply)
}

func (nm *NotificationManager) handleNewSone(event Event) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	notif, ok := nm.notifications["new-sones"]
	if !ok {
		notif = &Notification{
			ID:          "new-sones",
			Type:        "new-sones",
			Dismissable: true,
			Elements:    make([]interface{}, 0),
		}
		nm.notifications["new-sones"] = notif
	}

	notif.Elements = append(notif.Elements, event.Sone)
}

func (nm *NotificationManager) handleMention(event Event) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	notif, ok := nm.notifications["mentions"]
	if !ok {
		notif = &Notification{
			ID:          "mentions",
			Type:        "mentions",
			Dismissable: true,
			Elements:    make([]interface{}, 0),
		}
		nm.notifications["mentions"] = notif
	}

	notif.Elements = append(notif.Elements, event.Post)
}

// GetNotifications returns all current notifications
func (nm *NotificationManager) GetNotifications() []*Notification {
	nm.mu.RLock()
	defer nm.mu.RUnlock()

	notifs := make([]*Notification, 0, len(nm.notifications))
	for _, n := range nm.notifications {
		if len(n.Elements) > 0 {
			notifs = append(notifs, n)
		}
	}

	return notifs
}

// DismissNotification removes a notification
func (nm *NotificationManager) DismissNotification(id string) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	delete(nm.notifications, id)
}

// ClearNotification clears elements from a notification
func (nm *NotificationManager) ClearNotification(id string) {
	nm.mu.Lock()
	defer nm.mu.Unlock()

	if notif, ok := nm.notifications[id]; ok {
		notif.Elements = make([]interface{}, 0)
	}
}
