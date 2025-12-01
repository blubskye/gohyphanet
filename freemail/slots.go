// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package freemail

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Slot configuration constants
const (
	// SlotExpiration is the duration after which a slot expires (7 days)
	SlotExpiration = 7 * 24 * time.Hour

	// PollAheadSlots is the number of slots to check beyond the last used
	PollAheadSlots = 5

	// MaxSlotRetries is the maximum number of fetch attempts per slot
	MaxSlotRetries = 3

	// SlotPollInterval is the interval between slot polling attempts
	SlotPollInterval = 5 * time.Minute
)

// SlotState represents the state of a message slot
type SlotState int

const (
	SlotUnused SlotState = iota
	SlotUsed
	SlotExpired
	SlotFailed
)

// String returns the string representation of SlotState
func (s SlotState) String() string {
	switch s {
	case SlotUnused:
		return "unused"
	case SlotUsed:
		return "used"
	case SlotExpired:
		return "expired"
	case SlotFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Slot represents a message slot in a channel
type Slot struct {
	Number    int       `json:"number"`
	State     SlotState `json:"state"`
	Key       string    `json:"key,omitempty"`       // Freenet key for this slot
	MessageID string    `json:"message_id,omitempty"` // ID of message in this slot
	UsedAt    time.Time `json:"used_at,omitempty"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
	Retries   int       `json:"retries"`
	LastError string    `json:"last_error,omitempty"`
}

// IsExpired checks if the slot has expired
func (s *Slot) IsExpired() bool {
	if s.State == SlotExpired {
		return true
	}
	if s.State == SlotUsed && !s.ExpiresAt.IsZero() {
		return time.Now().After(s.ExpiresAt)
	}
	return false
}

// SlotRange represents a range of slots for a channel direction
type SlotRange struct {
	mu sync.RWMutex

	// Base key for generating slot keys
	BaseKey string `json:"base_key"`

	// Slot tracking
	Slots    map[int]*Slot `json:"slots"`
	NextSlot int           `json:"next_slot"` // Next slot to use for sending
	LastUsed int           `json:"last_used"` // Last slot that had a message

	// Direction: "send" or "receive"
	Direction string `json:"direction"`
}

// NewSlotRange creates a new slot range
func NewSlotRange(baseKey string, direction string) *SlotRange {
	return &SlotRange{
		BaseKey:   baseKey,
		Slots:     make(map[int]*Slot),
		NextSlot:  0,
		LastUsed:  -1,
		Direction: direction,
	}
}

// GetSlot returns a slot by number, creating it if necessary
func (sr *SlotRange) GetSlot(number int) *Slot {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if slot, exists := sr.Slots[number]; exists {
		return slot
	}

	slot := &Slot{
		Number: number,
		State:  SlotUnused,
		Key:    sr.generateSlotKey(number),
	}
	sr.Slots[number] = slot
	return slot
}

// generateSlotKey generates the Freenet key for a slot
func (sr *SlotRange) generateSlotKey(number int) string {
	// Key format: BaseKey-slot-N
	return fmt.Sprintf("%s-slot-%d", sr.BaseKey, number)
}

// AllocateSlot allocates the next available slot for sending
func (sr *SlotRange) AllocateSlot() *Slot {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	slot := &Slot{
		Number:    sr.NextSlot,
		State:     SlotUnused,
		Key:       sr.generateSlotKey(sr.NextSlot),
		ExpiresAt: time.Now().Add(SlotExpiration),
	}
	sr.Slots[sr.NextSlot] = slot
	sr.NextSlot++

	return slot
}

// MarkUsed marks a slot as used
func (sr *SlotRange) MarkUsed(number int, messageID string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	slot, exists := sr.Slots[number]
	if !exists {
		slot = &Slot{
			Number: number,
			Key:    sr.generateSlotKey(number),
		}
		sr.Slots[number] = slot
	}

	slot.State = SlotUsed
	slot.MessageID = messageID
	slot.UsedAt = time.Now()
	slot.ExpiresAt = time.Now().Add(SlotExpiration)

	if number > sr.LastUsed {
		sr.LastUsed = number
	}
}

// MarkExpired marks a slot as expired
func (sr *SlotRange) MarkExpired(number int) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if slot, exists := sr.Slots[number]; exists {
		slot.State = SlotExpired
	}
}

// MarkFailed marks a slot as failed after too many retries
func (sr *SlotRange) MarkFailed(number int, err string) {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	if slot, exists := sr.Slots[number]; exists {
		slot.Retries++
		slot.LastError = err
		if slot.Retries >= MaxSlotRetries {
			slot.State = SlotFailed
		}
	}
}

// GetSlotsToFetch returns slot numbers that should be fetched (poll ahead)
func (sr *SlotRange) GetSlotsToFetch() []int {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	var slots []int
	start := sr.LastUsed + 1
	if start < 0 {
		start = 0
	}

	for i := start; i < start+PollAheadSlots; i++ {
		slot, exists := sr.Slots[i]
		if !exists || slot.State == SlotUnused {
			slots = append(slots, i)
		}
	}

	return slots
}

// CleanExpired removes expired slots
func (sr *SlotRange) CleanExpired() int {
	sr.mu.Lock()
	defer sr.mu.Unlock()

	cleaned := 0
	for number, slot := range sr.Slots {
		if slot.IsExpired() {
			slot.State = SlotExpired
			cleaned++
			// Don't delete, just mark expired for tracking
			_ = number
		}
	}
	return cleaned
}

// SlotManager manages slots for all channels
type SlotManager struct {
	mu sync.RWMutex

	// Data directory for persistence
	dataDir string

	// Channel slots: channelID -> direction -> SlotRange
	channels map[string]map[string]*SlotRange

	// Callbacks
	onMessageReceived func(channelID string, slotNumber int, data []byte)
}

// NewSlotManager creates a new slot manager
func NewSlotManager(dataDir string) *SlotManager {
	return &SlotManager{
		dataDir:  dataDir,
		channels: make(map[string]map[string]*SlotRange),
	}
}

// SetMessageCallback sets the callback for received messages
func (sm *SlotManager) SetMessageCallback(callback func(channelID string, slotNumber int, data []byte)) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sm.onMessageReceived = callback
}

// GetSlotRange returns the slot range for a channel direction
func (sm *SlotManager) GetSlotRange(channelID, direction string) *SlotRange {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.channels[channelID]; !exists {
		sm.channels[channelID] = make(map[string]*SlotRange)
	}

	if sr, exists := sm.channels[channelID][direction]; exists {
		return sr
	}

	// Create new slot range
	baseKey := fmt.Sprintf("%s-%s", channelID, direction)
	sr := NewSlotRange(baseKey, direction)
	sm.channels[channelID][direction] = sr
	return sr
}

// InitializeChannel initializes slots for a new channel
func (sm *SlotManager) InitializeChannel(channelID string, sendBaseKey, receiveBaseKey string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if _, exists := sm.channels[channelID]; !exists {
		sm.channels[channelID] = make(map[string]*SlotRange)
	}

	sm.channels[channelID]["send"] = NewSlotRange(sendBaseKey, "send")
	sm.channels[channelID]["receive"] = NewSlotRange(receiveBaseKey, "receive")
}

// AllocateSendSlot allocates a slot for sending a message
func (sm *SlotManager) AllocateSendSlot(channelID string) *Slot {
	sr := sm.GetSlotRange(channelID, "send")
	return sr.AllocateSlot()
}

// MarkReceived marks a receive slot as having received a message
func (sm *SlotManager) MarkReceived(channelID string, slotNumber int, messageID string) {
	sr := sm.GetSlotRange(channelID, "receive")
	sr.MarkUsed(slotNumber, messageID)
}

// MarkSent marks a send slot as having sent a message
func (sm *SlotManager) MarkSent(channelID string, slotNumber int, messageID string) {
	sr := sm.GetSlotRange(channelID, "send")
	sr.MarkUsed(slotNumber, messageID)
}

// GetPendingFetches returns all slots that need to be fetched across all channels
func (sm *SlotManager) GetPendingFetches() map[string][]int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	pending := make(map[string][]int)
	for channelID, directions := range sm.channels {
		if sr, exists := directions["receive"]; exists {
			slots := sr.GetSlotsToFetch()
			if len(slots) > 0 {
				pending[channelID] = slots
			}
		}
	}
	return pending
}

// CleanAllExpired cleans expired slots in all channels
func (sm *SlotManager) CleanAllExpired() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	total := 0
	for _, directions := range sm.channels {
		for _, sr := range directions {
			total += sr.CleanExpired()
		}
	}
	return total
}

// Save persists slot state to disk
func (sm *SlotManager) Save() error {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	if sm.dataDir == "" {
		return nil
	}

	data, err := json.MarshalIndent(sm.channels, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal slots: %w", err)
	}

	filePath := filepath.Join(sm.dataDir, "slots.json")
	if err := os.WriteFile(filePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write slots file: %w", err)
	}

	return nil
}

// Load loads slot state from disk
func (sm *SlotManager) Load() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if sm.dataDir == "" {
		return nil
	}

	filePath := filepath.Join(sm.dataDir, "slots.json")
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read slots file: %w", err)
	}

	if err := json.Unmarshal(data, &sm.channels); err != nil {
		return fmt.Errorf("failed to unmarshal slots: %w", err)
	}

	return nil
}

// SlotPoller polls for messages in slots
type SlotPoller struct {
	mu sync.RWMutex

	slotManager *SlotManager
	fetcher     SlotFetcher // Interface for fetching slot data

	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup

	// Stats
	fetchCount int
	errorCount int
}

// SlotFetcher interface for fetching slot data from Freenet
type SlotFetcher interface {
	// FetchSlot fetches data from a slot key
	FetchSlot(key string) ([]byte, error)
}

// NewSlotPoller creates a new slot poller
func NewSlotPoller(slotManager *SlotManager, fetcher SlotFetcher) *SlotPoller {
	return &SlotPoller{
		slotManager: slotManager,
		fetcher:     fetcher,
		stopChan:    make(chan struct{}),
	}
}

// Start begins polling for messages
func (sp *SlotPoller) Start() {
	sp.mu.Lock()
	if sp.running {
		sp.mu.Unlock()
		return
	}
	sp.running = true
	sp.stopChan = make(chan struct{})
	sp.mu.Unlock()

	sp.wg.Add(1)
	go sp.pollLoop()
}

// Stop stops the poller
func (sp *SlotPoller) Stop() {
	sp.mu.Lock()
	if !sp.running {
		sp.mu.Unlock()
		return
	}
	sp.running = false
	close(sp.stopChan)
	sp.mu.Unlock()

	sp.wg.Wait()
}

// pollLoop is the main polling loop
func (sp *SlotPoller) pollLoop() {
	defer sp.wg.Done()

	ticker := time.NewTicker(SlotPollInterval)
	defer ticker.Stop()

	// Initial poll
	sp.pollOnce()

	for {
		select {
		case <-sp.stopChan:
			return
		case <-ticker.C:
			sp.pollOnce()
		}
	}
}

// pollOnce performs one round of polling
func (sp *SlotPoller) pollOnce() {
	pending := sp.slotManager.GetPendingFetches()

	for channelID, slots := range pending {
		for _, slotNum := range slots {
			sr := sp.slotManager.GetSlotRange(channelID, "receive")
			slot := sr.GetSlot(slotNum)

			data, err := sp.fetcher.FetchSlot(slot.Key)
			if err != nil {
				sp.mu.Lock()
				sp.errorCount++
				sp.mu.Unlock()
				sr.MarkFailed(slotNum, err.Error())
				continue
			}

			if data != nil {
				sp.mu.Lock()
				sp.fetchCount++
				sp.mu.Unlock()

				sr.MarkUsed(slotNum, "")

				// Notify via callback
				sp.slotManager.mu.RLock()
				callback := sp.slotManager.onMessageReceived
				sp.slotManager.mu.RUnlock()

				if callback != nil {
					callback(channelID, slotNum, data)
				}
			}
		}
	}

	// Clean expired slots periodically
	sp.slotManager.CleanAllExpired()
}

// GetStats returns polling statistics
func (sp *SlotPoller) GetStats() (fetched, errors int) {
	sp.mu.RLock()
	defer sp.mu.RUnlock()
	return sp.fetchCount, sp.errorCount
}
