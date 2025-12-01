// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package fcp

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
)

// USKSubscription represents an active USK subscription
type USKSubscription struct {
	ID          string
	URI         string
	Edition     int64
	Identifier  string
	DontPoll    bool
	Callbacks   []USKCallback
	callbacksMu sync.RWMutex
}

// USKCallback is called when a USK update is received
type USKCallback func(uri string, edition int64, newURI string)

// USKManager handles USK subscriptions
type USKManager struct {
	client        *Client
	subscriptions map[string]*USKSubscription
	mu            sync.RWMutex
	counter       uint64
}

// NewUSKManager creates a new USK manager
func NewUSKManager(client *Client) *USKManager {
	mgr := &USKManager{
		client:        client,
		subscriptions: make(map[string]*USKSubscription),
	}

	// Register handlers
	client.RegisterHandler("SubscribedUSKUpdate", mgr.handleUSKUpdate)
	client.RegisterHandler("SubscribedUSKRoundFinished", mgr.handleRoundFinished)
	client.RegisterHandler("SubscribedUSKSendingToNetwork", mgr.handleSendingToNetwork)

	return mgr
}

// Subscribe creates a new USK subscription
func (m *USKManager) Subscribe(uri string, callback USKCallback) (*USKSubscription, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already subscribed
	for _, sub := range m.subscriptions {
		if sub.URI == uri {
			// Add callback to existing subscription
			sub.callbacksMu.Lock()
			sub.Callbacks = append(sub.Callbacks, callback)
			sub.callbacksMu.Unlock()
			return sub, nil
		}
	}

	// Create new subscription
	m.counter++
	identifier := fmt.Sprintf("usk-%d", m.counter)

	sub := &USKSubscription{
		ID:         identifier,
		URI:        uri,
		Identifier: identifier,
		Callbacks:  []USKCallback{callback},
	}

	// Parse edition from URI if present
	sub.Edition = parseUSKEdition(uri)

	// Send SubscribeUSK message
	msg := &Message{
		Name: "SubscribeUSK",
		Fields: map[string]string{
			"URI":        uri,
			"Identifier": identifier,
			"DontPoll":   "false",
			"SparsePoll": "true",
			"Priority":   "4", // Semi-interactive
		},
	}

	if err := m.client.SendMessage(msg); err != nil {
		return nil, fmt.Errorf("failed to subscribe to USK: %w", err)
	}

	m.subscriptions[identifier] = sub
	return sub, nil
}

// Unsubscribe removes a USK subscription
func (m *USKManager) Unsubscribe(identifier string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	sub, exists := m.subscriptions[identifier]
	if !exists {
		return fmt.Errorf("subscription not found: %s", identifier)
	}

	// Send UnsubscribeUSK message
	msg := &Message{
		Name: "UnsubscribeUSK",
		Fields: map[string]string{
			"Identifier": identifier,
		},
	}

	if err := m.client.SendMessage(msg); err != nil {
		return fmt.Errorf("failed to unsubscribe from USK: %w", err)
	}

	delete(m.subscriptions, identifier)
	_ = sub // Subscription removed
	return nil
}

// UnsubscribeByURI removes a USK subscription by URI
func (m *USKManager) UnsubscribeByURI(uri string) error {
	m.mu.RLock()
	var identifier string
	for id, sub := range m.subscriptions {
		if sub.URI == uri {
			identifier = id
			break
		}
	}
	m.mu.RUnlock()

	if identifier == "" {
		return fmt.Errorf("subscription not found for URI: %s", uri)
	}

	return m.Unsubscribe(identifier)
}

// GetSubscription returns a subscription by identifier
func (m *USKManager) GetSubscription(identifier string) *USKSubscription {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.subscriptions[identifier]
}

// GetAllSubscriptions returns all active subscriptions
func (m *USKManager) GetAllSubscriptions() []*USKSubscription {
	m.mu.RLock()
	defer m.mu.RUnlock()

	subs := make([]*USKSubscription, 0, len(m.subscriptions))
	for _, sub := range m.subscriptions {
		subs = append(subs, sub)
	}
	return subs
}

// handleUSKUpdate processes SubscribedUSKUpdate messages
func (m *USKManager) handleUSKUpdate(msg *Message) error {
	identifier := msg.Fields["Identifier"]
	uri := msg.Fields["URI"]
	editionStr := msg.Fields["Edition"]

	edition, _ := strconv.ParseInt(editionStr, 10, 64)

	m.mu.RLock()
	sub, exists := m.subscriptions[identifier]
	m.mu.RUnlock()

	if !exists {
		return nil // Unknown subscription, ignore
	}

	// Update edition
	sub.Edition = edition

	// Notify callbacks
	sub.callbacksMu.RLock()
	callbacks := make([]USKCallback, len(sub.Callbacks))
	copy(callbacks, sub.Callbacks)
	sub.callbacksMu.RUnlock()

	for _, cb := range callbacks {
		cb(sub.URI, edition, uri)
	}

	return nil
}

// handleRoundFinished processes SubscribedUSKRoundFinished messages
func (m *USKManager) handleRoundFinished(msg *Message) error {
	// This indicates a polling round has finished
	// We can use this for status updates if needed
	return nil
}

// handleSendingToNetwork processes SubscribedUSKSendingToNetwork messages
func (m *USKManager) handleSendingToNetwork(msg *Message) error {
	// This indicates the node is actively looking for updates
	return nil
}

// parseUSKEdition extracts the edition number from a USK URI
func parseUSKEdition(uri string) int64 {
	// USK format: USK@key/path/edition
	if !strings.HasPrefix(uri, "USK@") {
		return 0
	}

	parts := strings.Split(uri, "/")
	if len(parts) < 3 {
		return 0
	}

	// Edition is the last numeric part
	lastPart := parts[len(parts)-1]
	edition, _ := strconv.ParseInt(lastPart, 10, 64)
	return edition
}

// UpdateUSKEdition updates the URI to use a specific edition
func UpdateUSKEdition(uri string, edition int64) string {
	if !strings.HasPrefix(uri, "USK@") {
		return uri
	}

	parts := strings.Split(uri, "/")
	if len(parts) < 3 {
		return uri
	}

	// Replace the last part with the new edition
	parts[len(parts)-1] = strconv.FormatInt(edition, 10)
	return strings.Join(parts, "/")
}

// ConvertUSKToSSK converts a USK to the equivalent SSK for a specific edition
func ConvertUSKToSSK(uri string, edition int64) string {
	if !strings.HasPrefix(uri, "USK@") {
		return uri
	}

	// USK@key/path/edition -> SSK@key/path-edition
	parts := strings.Split(uri, "/")
	if len(parts) < 3 {
		return uri
	}

	key := parts[0][4:] // Remove "USK@"
	path := parts[1]

	return fmt.Sprintf("SSK@%s/%s-%d", key, path, edition)
}
