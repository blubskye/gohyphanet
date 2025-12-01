// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package sone

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
)

// USKMonitor monitors USK updates for Sones
type USKMonitor struct {
	core       *Core
	uskManager *fcp.USKManager

	// Track subscribed Sones
	subscriptions map[string]string // soneID -> subscription identifier
	mu            sync.RWMutex

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewUSKMonitor creates a new USK monitor
func NewUSKMonitor(core *Core) *USKMonitor {
	ctx, cancel := context.WithCancel(context.Background())

	return &USKMonitor{
		core:          core,
		uskManager:    fcp.NewUSKManager(core.fcpClient),
		subscriptions: make(map[string]string),
		ctx:           ctx,
		cancel:        cancel,
	}
}

// Start begins monitoring USKs
func (m *USKMonitor) Start() error {
	// Subscribe to all followed Sones
	for _, localSone := range m.core.GetLocalSones() {
		for friendID := range localSone.Friends {
			if err := m.SubscribeSone(friendID); err != nil {
				log.Printf("Failed to subscribe to Sone %s: %v", friendID, err)
			}
		}
	}

	// Also subscribe to all known remote Sones
	for _, remoteSone := range m.core.GetAllSones() {
		if !remoteSone.IsLocal && remoteSone.RequestURI != "" {
			if err := m.SubscribeSone(remoteSone.ID); err != nil {
				log.Printf("Failed to subscribe to Sone %s: %v", remoteSone.ID, err)
			}
		}
	}

	// Start background refresh loop
	m.wg.Add(1)
	go m.refreshLoop()

	return nil
}

// Stop stops the USK monitor
func (m *USKMonitor) Stop() {
	m.cancel()
	m.wg.Wait()

	// Unsubscribe from all
	m.mu.Lock()
	for soneID, identifier := range m.subscriptions {
		if err := m.uskManager.Unsubscribe(identifier); err != nil {
			log.Printf("Failed to unsubscribe from Sone %s: %v", soneID, err)
		}
	}
	m.subscriptions = make(map[string]string)
	m.mu.Unlock()
}

// SubscribeSone subscribes to USK updates for a Sone
func (m *USKMonitor) SubscribeSone(soneID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already subscribed
	if _, exists := m.subscriptions[soneID]; exists {
		return nil // Already subscribed
	}

	// Get the Sone to find its request URI
	sone := m.core.GetSone(soneID)
	if sone == nil {
		return fmt.Errorf("sone not found: %s", soneID)
	}

	if sone.RequestURI == "" {
		return fmt.Errorf("sone has no request URI: %s", soneID)
	}

	// Build USK URI for the Sone
	uskURI := buildSoneUSK(sone.RequestURI, sone.LatestEdition)

	// Subscribe
	sub, err := m.uskManager.Subscribe(uskURI, func(uri string, edition int64, newURI string) {
		m.handleSoneUpdate(soneID, edition, newURI)
	})

	if err != nil {
		return err
	}

	m.subscriptions[soneID] = sub.ID
	log.Printf("Subscribed to USK updates for Sone: %s", sone.Name)

	return nil
}

// UnsubscribeSone unsubscribes from USK updates for a Sone
func (m *USKMonitor) UnsubscribeSone(soneID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	identifier, exists := m.subscriptions[soneID]
	if !exists {
		return nil // Not subscribed
	}

	if err := m.uskManager.Unsubscribe(identifier); err != nil {
		return err
	}

	delete(m.subscriptions, soneID)
	return nil
}

// handleSoneUpdate processes a USK update notification
func (m *USKMonitor) handleSoneUpdate(soneID string, edition int64, newURI string) {
	log.Printf("USK update for Sone %s: edition %d", soneID, edition)

	// Get the Sone
	sone := m.core.GetSone(soneID)
	if sone == nil {
		return
	}

	// Check if this is a newer edition
	if edition <= sone.LatestEdition {
		return // Not newer
	}

	// Update the edition
	sone.mu.Lock()
	sone.LatestEdition = edition
	sone.mu.Unlock()

	// Trigger a fetch of the new edition
	m.fetchSoneEdition(soneID, edition)

	// Publish event
	m.core.eventBus.Publish(Event{
		Type: EventSoneUpdating,
		Sone: sone,
	})
}

// fetchSoneEdition fetches a specific edition of a Sone
func (m *USKMonitor) fetchSoneEdition(soneID string, edition int64) {
	sone := m.core.GetSone(soneID)
	if sone == nil {
		return
	}

	// Build the SSK URI for this edition
	sskURI := buildSoneSSK(sone.RequestURI, edition)

	// Fetch the Sone XML
	ctx, cancel := context.WithTimeout(m.ctx, 5*time.Minute)
	defer cancel()

	ops := fcp.NewOperations(m.core.fcpClient)
	result, err := ops.Get(ctx, sskURI+"/sone.xml")

	if err != nil {
		log.Printf("Failed to fetch Sone %s edition %d: %v", sone.Name, edition, err)
		return
	}

	if !result.Success {
		log.Printf("Fetch failed for Sone %s: %s", sone.Name, result.Error)
		return
	}

	// Process the fetched XML
	if err := m.core.ProcessFetchedSone(soneID, result.Data); err != nil {
		log.Printf("Failed to process Sone %s: %v", sone.Name, err)
		return
	}

	log.Printf("Successfully fetched Sone %s edition %d", sone.Name, edition)
}

// refreshLoop periodically checks for new subscriptions
func (m *USKMonitor) refreshLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.refreshSubscriptions()
		}
	}
}

// refreshSubscriptions ensures all followed Sones are subscribed
func (m *USKMonitor) refreshSubscriptions() {
	// Get all Sones that should be subscribed
	shouldSubscribe := make(map[string]bool)

	// Add all friends of local Sones
	for _, localSone := range m.core.GetLocalSones() {
		for friendID := range localSone.Friends {
			shouldSubscribe[friendID] = true
		}
	}

	// Subscribe to any missing
	for soneID := range shouldSubscribe {
		m.mu.RLock()
		_, exists := m.subscriptions[soneID]
		m.mu.RUnlock()

		if !exists {
			if err := m.SubscribeSone(soneID); err != nil {
				log.Printf("Failed to subscribe to Sone %s: %v", soneID, err)
			}
		}
	}

	// Unsubscribe from any no longer needed
	m.mu.Lock()
	for soneID := range m.subscriptions {
		if !shouldSubscribe[soneID] {
			if identifier, ok := m.subscriptions[soneID]; ok {
				m.uskManager.Unsubscribe(identifier)
				delete(m.subscriptions, soneID)
			}
		}
	}
	m.mu.Unlock()
}

// Helper functions

// buildSoneUSK builds a USK URI for a Sone
func buildSoneUSK(requestURI string, edition int64) string {
	// Request URI is typically SSK@key/name
	// Convert to USK@key/name/edition

	if strings.HasPrefix(requestURI, "USK@") {
		return fcp.UpdateUSKEdition(requestURI, edition)
	}

	if strings.HasPrefix(requestURI, "SSK@") {
		// Convert SSK to USK
		// SSK@key/name -> USK@key/name/edition
		key := requestURI[4:] // Remove "SSK@"
		return fmt.Sprintf("USK@%s/%d", key, edition)
	}

	return requestURI
}

// buildSoneSSK builds an SSK URI for a specific Sone edition
func buildSoneSSK(requestURI string, edition int64) string {
	if strings.HasPrefix(requestURI, "USK@") {
		return fcp.ConvertUSKToSSK(requestURI, edition)
	}

	if strings.HasPrefix(requestURI, "SSK@") {
		// Already SSK, just append edition if needed
		// SSK@key/name -> SSK@key/name-edition
		parts := strings.Split(requestURI, "/")
		if len(parts) >= 2 {
			return fmt.Sprintf("SSK@%s/%s-%d", parts[0][4:], parts[1], edition)
		}
	}

	return requestURI
}

// EventSoneUpdating is published when a Sone update is detected
const EventSoneUpdating = EventType(100)
