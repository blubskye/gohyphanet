// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package freemail

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// Mailsite constants
const (
	// MailsiteVersion is the current mailsite format version
	MailsiteVersion = 1

	// MailsiteInsertInterval is the interval between mailsite updates
	MailsiteInsertInterval = 6 * time.Hour

	// MailsiteSlotCount is the number of mailsite slots to maintain
	MailsiteSlotCount = 10
)

// MailsiteData represents the published mailsite information
type MailsiteData struct {
	Version     int    `json:"version"`
	Identity    string `json:"identity"`
	Nickname    string `json:"nickname"`
	RTSKey      string `json:"rts_key"`       // Key for receiving RTS messages
	PublicKeyPEM string `json:"public_key"`   // PEM-encoded RSA public key
	SlotInfo    *MailsiteSlotInfo `json:"slot_info"`
	UpdatedAt   int64  `json:"updated_at"`
}

// MailsiteSlotInfo contains slot configuration for messaging
type MailsiteSlotInfo struct {
	BaseKey    string `json:"base_key"`    // Base key for slot generation
	NextSlot   int    `json:"next_slot"`   // Next slot for incoming messages
	SlotCount  int    `json:"slot_count"`  // Number of active slots
}

// Mailsite manages mailsite publishing and retrieval
type Mailsite struct {
	mu sync.RWMutex

	// Account information
	identity    string
	nickname    string
	insertURI   string // USK insert URI for mailsite
	requestURI  string // USK request URI for mailsite
	rtsKey      string // Key for receiving RTS messages
	publicKeyPEM string

	// Slot configuration
	slotBaseKey string
	nextSlot    int

	// Current edition
	edition int64

	// Last update time
	lastUpdate time.Time

	// Inserter interface
	inserter MailsiteInserter
}

// MailsiteInserter interface for inserting mailsite data
type MailsiteInserter interface {
	// InsertMailsite inserts mailsite data to a USK
	InsertMailsite(uri string, data []byte) error
}

// NewMailsite creates a new mailsite manager
func NewMailsite(identity, nickname, insertURI, requestURI, rtsKey, publicKeyPEM string) *Mailsite {
	return &Mailsite{
		identity:    identity,
		nickname:    nickname,
		insertURI:   insertURI,
		requestURI:  requestURI,
		rtsKey:      rtsKey,
		publicKeyPEM: publicKeyPEM,
		slotBaseKey: fmt.Sprintf("%s-mail", identity),
		nextSlot:    0,
		edition:     0,
	}
}

// SetInserter sets the mailsite inserter
func (m *Mailsite) SetInserter(inserter MailsiteInserter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.inserter = inserter
}

// GetRequestURI returns the mailsite request URI
func (m *Mailsite) GetRequestURI() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.requestURI
}

// GetRTSKey returns the RTS key for receiving RTS messages
func (m *Mailsite) GetRTSKey() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.rtsKey
}

// GetSlotBaseKey returns the base key for slot generation
func (m *Mailsite) GetSlotBaseKey() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.slotBaseKey
}

// GetNextSlot returns and increments the next slot number
func (m *Mailsite) GetNextSlot() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	slot := m.nextSlot
	m.nextSlot++
	return slot
}

// BuildMailsiteData creates the mailsite data structure
func (m *Mailsite) BuildMailsiteData() *MailsiteData {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return &MailsiteData{
		Version:     MailsiteVersion,
		Identity:    m.identity,
		Nickname:    m.nickname,
		RTSKey:      m.rtsKey,
		PublicKeyPEM: m.publicKeyPEM,
		SlotInfo: &MailsiteSlotInfo{
			BaseKey:   m.slotBaseKey,
			NextSlot:  m.nextSlot,
			SlotCount: MailsiteSlotCount,
		},
		UpdatedAt: time.Now().Unix(),
	}
}

// Publish publishes the mailsite to Freenet
func (m *Mailsite) Publish() error {
	m.mu.Lock()
	inserter := m.inserter
	m.mu.Unlock()

	if inserter == nil {
		return fmt.Errorf("no inserter configured")
	}

	data := m.BuildMailsiteData()
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal mailsite: %w", err)
	}

	if err := inserter.InsertMailsite(m.insertURI, jsonData); err != nil {
		return fmt.Errorf("failed to insert mailsite: %w", err)
	}

	m.mu.Lock()
	m.lastUpdate = time.Now()
	m.edition++
	m.mu.Unlock()

	return nil
}

// NeedsUpdate checks if the mailsite needs to be updated
func (m *Mailsite) NeedsUpdate() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.lastUpdate.IsZero() {
		return true
	}
	return time.Since(m.lastUpdate) > MailsiteInsertInterval
}

// MailsiteFetcher fetches mailsite data from Freenet
type MailsiteFetcher struct {
	mu sync.RWMutex

	// Cache of fetched mailsites
	cache map[string]*CachedMailsite

	// Fetcher interface
	fetcher MailsiteDataFetcher
}

// CachedMailsite represents a cached mailsite
type CachedMailsite struct {
	Data      *MailsiteData
	FetchedAt time.Time
	Edition   int64
}

// MailsiteDataFetcher interface for fetching mailsite data
type MailsiteDataFetcher interface {
	// FetchMailsite fetches mailsite data from a USK
	FetchMailsite(uri string) ([]byte, int64, error)
}

// NewMailsiteFetcher creates a new mailsite fetcher
func NewMailsiteFetcher(fetcher MailsiteDataFetcher) *MailsiteFetcher {
	return &MailsiteFetcher{
		cache:   make(map[string]*CachedMailsite),
		fetcher: fetcher,
	}
}

// Fetch fetches a mailsite, using cache if available
func (mf *MailsiteFetcher) Fetch(uri string) (*MailsiteData, error) {
	// Check cache
	mf.mu.RLock()
	cached, exists := mf.cache[uri]
	mf.mu.RUnlock()

	if exists && time.Since(cached.FetchedAt) < MailsiteInsertInterval {
		return cached.Data, nil
	}

	// Fetch from Freenet
	data, edition, err := mf.fetcher.FetchMailsite(uri)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch mailsite: %w", err)
	}

	// Parse mailsite data
	var mailsite MailsiteData
	if err := json.Unmarshal(data, &mailsite); err != nil {
		return nil, fmt.Errorf("failed to parse mailsite: %w", err)
	}

	// Update cache
	mf.mu.Lock()
	mf.cache[uri] = &CachedMailsite{
		Data:      &mailsite,
		FetchedAt: time.Now(),
		Edition:   edition,
	}
	mf.mu.Unlock()

	return &mailsite, nil
}

// GetCached returns a cached mailsite without fetching
func (mf *MailsiteFetcher) GetCached(uri string) *MailsiteData {
	mf.mu.RLock()
	defer mf.mu.RUnlock()

	if cached, exists := mf.cache[uri]; exists {
		return cached.Data
	}
	return nil
}

// InvalidateCache removes a mailsite from the cache
func (mf *MailsiteFetcher) InvalidateCache(uri string) {
	mf.mu.Lock()
	defer mf.mu.Unlock()
	delete(mf.cache, uri)
}

// ClearCache clears all cached mailsites
func (mf *MailsiteFetcher) ClearCache() {
	mf.mu.Lock()
	defer mf.mu.Unlock()
	mf.cache = make(map[string]*CachedMailsite)
}

// MailsitePublisher periodically publishes mailsite updates
type MailsitePublisher struct {
	mu sync.RWMutex

	mailsite *Mailsite

	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewMailsitePublisher creates a new mailsite publisher
func NewMailsitePublisher(mailsite *Mailsite) *MailsitePublisher {
	return &MailsitePublisher{
		mailsite: mailsite,
		stopChan: make(chan struct{}),
	}
}

// Start begins periodic mailsite publishing
func (mp *MailsitePublisher) Start() {
	mp.mu.Lock()
	if mp.running {
		mp.mu.Unlock()
		return
	}
	mp.running = true
	mp.stopChan = make(chan struct{})
	mp.mu.Unlock()

	mp.wg.Add(1)
	go mp.publishLoop()
}

// Stop stops the publisher
func (mp *MailsitePublisher) Stop() {
	mp.mu.Lock()
	if !mp.running {
		mp.mu.Unlock()
		return
	}
	mp.running = false
	close(mp.stopChan)
	mp.mu.Unlock()

	mp.wg.Wait()
}

// publishLoop is the main publishing loop
func (mp *MailsitePublisher) publishLoop() {
	defer mp.wg.Done()

	ticker := time.NewTicker(MailsiteInsertInterval)
	defer ticker.Stop()

	// Initial publish if needed
	if mp.mailsite.NeedsUpdate() {
		mp.mailsite.Publish()
	}

	for {
		select {
		case <-mp.stopChan:
			return
		case <-ticker.C:
			if mp.mailsite.NeedsUpdate() {
				mp.mailsite.Publish()
			}
		}
	}
}

// PublishNow immediately publishes the mailsite
func (mp *MailsitePublisher) PublishNow() error {
	return mp.mailsite.Publish()
}
