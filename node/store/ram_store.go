package store

import (
	"bytes"
	"crypto/dsa"
	"fmt"
	"sync"
	"sync/atomic"
)

// StoredBlock represents a block stored in RAM
type StoredBlock struct {
	header   []byte
	data     []byte
	fullKey  []byte
	oldBlock bool
}

// RAMFreenetStore is a simple in-memory LRU datastore
type RAMFreenetStore struct {
	mu                 sync.RWMutex
	blocksByRoutingKey map[string]*StoredBlock
	callback           StoreCallback
	maxKeys            int64
	hits               int64
	misses             int64
	writes             int64
	keyCount           int64
	closed             bool

	// LRU tracking
	accessOrder []string // Most recently accessed at the end
	orderMu     sync.Mutex
}

// defaultStoreCallback is a simple callback that works for both CHK and SSK
type defaultStoreCallback struct{}

func (d *defaultStoreCallback) DataLength() int          { return CHKDataLength }
func (d *defaultStoreCallback) HeaderLength() int        { return CHKTotalHeadersLength }
func (d *defaultStoreCallback) RoutingKeyLength() int    { return 32 }
func (d *defaultStoreCallback) FullKeyLength() int       { return 34 }
func (d *defaultStoreCallback) StoreFullKeys() bool      { return true }
func (d *defaultStoreCallback) CollisionPossible() bool  { return true }
func (d *defaultStoreCallback) ConstructNeedsKey() bool  { return true }

func (d *defaultStoreCallback) Construct(data, headers, routingKey, fullKey []byte,
	canReadClientCache, canReadSlashdotCache bool,
	meta *BlockMetadata, knownPubKey *dsa.PublicKey) (KeyBlock, error) {
	// This is a simplified implementation - just return nil
	// In practice, the Put method doesn't use this
	return nil, fmt.Errorf("construct not implemented for default callback")
}

func (d *defaultStoreCallback) RoutingKeyFromFullKey(keyBuf []byte) []byte {
	if len(keyBuf) < 2 {
		return nil
	}
	return keyBuf[2:] // Skip type bytes
}

// NewRAMFreenetStore creates a new RAM-based datastore
func NewRAMFreenetStore(callback StoreCallback, maxKeys int64) *RAMFreenetStore {
	if maxKeys <= 0 {
		maxKeys = 10000 // Default to 10k blocks
	}

	// Use default callback if none provided
	if callback == nil {
		callback = &defaultStoreCallback{}
	}

	return &RAMFreenetStore{
		blocksByRoutingKey: make(map[string]*StoredBlock),
		callback:           callback,
		maxKeys:            maxKeys,
		accessOrder:        make([]string, 0, maxKeys),
	}
}

// Start initializes the store (no-op for RAM store)
func (s *RAMFreenetStore) Start() error {
	return nil
}

// Close shuts down the store
func (s *RAMFreenetStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.closed = true
	s.blocksByRoutingKey = nil
	s.accessOrder = nil

	return nil
}

// Fetch retrieves a block by routing key
func (s *RAMFreenetStore) Fetch(routingKey, fullKey []byte, dontPromote, canReadClientCache,
	canReadSlashdotCache, ignoreOldBlocks bool, meta *BlockMetadata) (StorableBlock, error) {

	s.mu.RLock()
	if s.closed {
		s.mu.RUnlock()
		return nil, fmt.Errorf("store is closed")
	}

	key := string(routingKey)
	block, exists := s.blocksByRoutingKey[key]

	if !exists {
		s.mu.RUnlock()
		atomic.AddInt64(&s.misses, 1)
		return nil, nil
	}

	// Check old block flag
	if ignoreOldBlocks && block.oldBlock {
		s.mu.RUnlock()
		atomic.AddInt64(&s.misses, 1)
		return nil, nil
	}

	// Copy block data for reconstruction (to avoid holding lock)
	headerCopy := make([]byte, len(block.header))
	copy(headerCopy, block.header)
	dataCopy := make([]byte, len(block.data))
	copy(dataCopy, block.data)
	var fullKeyCopy []byte
	if block.fullKey != nil {
		fullKeyCopy = make([]byte, len(block.fullKey))
		copy(fullKeyCopy, block.fullKey)
	}
	isOldBlock := block.oldBlock

	s.mu.RUnlock()

	// Promote in LRU if needed (outside main lock)
	if !dontPromote {
		s.promoteKey(key)
	}

	// Reconstruct block outside of lock
	constructed, err := s.callback.Construct(
		dataCopy, headerCopy, routingKey, fullKeyCopy,
		canReadClientCache, canReadSlashdotCache, meta, nil)

	if err != nil {
		// Block is corrupted, remove it
		s.mu.Lock()
		delete(s.blocksByRoutingKey, key)
		s.removeFromAccessOrder(key)
		s.mu.Unlock()

		atomic.AddInt64(&s.misses, 1)
		return nil, err
	}

	if constructed == nil {
		atomic.AddInt64(&s.misses, 1)
		return nil, nil
	}

	atomic.AddInt64(&s.hits, 1)

	if meta != nil && isOldBlock {
		meta.SetOldBlock()
	}

	return constructed, nil
}

// Put stores a block
func (s *RAMFreenetStore) Put(block StorableBlock, data, header []byte, overwrite, isOldBlock bool) error {
	if block == nil {
		return fmt.Errorf("block cannot be nil")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	routingKey := block.GetRoutingKey()
	fullKey := block.GetFullKey()
	key := string(routingKey)

	atomic.AddInt64(&s.writes, 1)

	existingBlock, exists := s.blocksByRoutingKey[key]
	storeFullKeys := s.callback.StoreFullKeys()
	collisionPossible := s.callback.CollisionPossible()

	if exists {
		if collisionPossible {
			// SSK: Check if identical
			identical := bytes.Equal(existingBlock.data, data) &&
				bytes.Equal(existingBlock.header, header)

			if storeFullKeys && existingBlock.fullKey != nil {
				identical = identical && bytes.Equal(existingBlock.fullKey, fullKey)
			}

			if identical {
				// Update old block flag if needed
				if !isOldBlock && existingBlock.oldBlock {
					existingBlock.oldBlock = false
				}
				return nil
			}

			// Different data - check if we should overwrite
			if !overwrite {
				return fmt.Errorf("key collision and overwrite not allowed")
			}

			// Overwrite existing block
			existingBlock.data = make([]byte, len(data))
			copy(existingBlock.data, data)
			existingBlock.header = make([]byte, len(header))
			copy(existingBlock.header, header)
			existingBlock.oldBlock = isOldBlock

			if storeFullKeys {
				existingBlock.fullKey = make([]byte, len(fullKey))
				copy(existingBlock.fullKey, fullKey)
			}

			// Promote to most recent
			s.removeFromAccessOrder(key)
			s.accessOrder = append(s.accessOrder, key)

			return nil
		} else {
			// CHK: No collision possible, just update old block flag
			if !isOldBlock && existingBlock.oldBlock {
				existingBlock.oldBlock = false
			}
			return nil
		}
	}

	// New block - make copies
	newBlock := &StoredBlock{
		data:     make([]byte, len(data)),
		header:   make([]byte, len(header)),
		oldBlock: isOldBlock,
	}
	copy(newBlock.data, data)
	copy(newBlock.header, header)

	if storeFullKeys {
		newBlock.fullKey = make([]byte, len(fullKey))
		copy(newBlock.fullKey, fullKey)
	}

	// Add to store
	s.blocksByRoutingKey[key] = newBlock
	s.accessOrder = append(s.accessOrder, key)
	atomic.AddInt64(&s.keyCount, 1)

	// LRU eviction if needed
	if int64(len(s.blocksByRoutingKey)) > s.maxKeys {
		s.evictOldest()
	}

	return nil
}

// promoteKey moves a key to the most recent position in LRU
func (s *RAMFreenetStore) promoteKey(key string) {
	s.orderMu.Lock()
	defer s.orderMu.Unlock()

	s.removeFromAccessOrderUnlocked(key)
	s.accessOrder = append(s.accessOrder, key)
}

// removeFromAccessOrder removes a key from the access order list
func (s *RAMFreenetStore) removeFromAccessOrder(key string) {
	s.orderMu.Lock()
	defer s.orderMu.Unlock()
	s.removeFromAccessOrderUnlocked(key)
}

// removeFromAccessOrderUnlocked removes a key from access order (must hold orderMu)
func (s *RAMFreenetStore) removeFromAccessOrderUnlocked(key string) {
	for i, k := range s.accessOrder {
		if k == key {
			// Remove by swapping with last and truncating
			s.accessOrder[i] = s.accessOrder[len(s.accessOrder)-1]
			s.accessOrder = s.accessOrder[:len(s.accessOrder)-1]
			return
		}
	}
}

// evictOldest removes the least recently used block (must hold mu)
func (s *RAMFreenetStore) evictOldest() {
	s.orderMu.Lock()
	defer s.orderMu.Unlock()

	if len(s.accessOrder) == 0 {
		return
	}

	// Remove oldest (first in list)
	oldestKey := s.accessOrder[0]
	s.accessOrder = s.accessOrder[1:]

	delete(s.blocksByRoutingKey, oldestKey)
	atomic.AddInt64(&s.keyCount, -1)
}

// SetMaxKeys changes the maximum number of keys
func (s *RAMFreenetStore) SetMaxKeys(maxStoreKeys int64, shrinkNow bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.maxKeys = maxStoreKeys

	if shrinkNow {
		// Evict blocks until we're under the limit
		for int64(len(s.blocksByRoutingKey)) > s.maxKeys {
			s.evictOldest()
		}
	}

	return nil
}

// GetMaxKeys returns the maximum number of keys
func (s *RAMFreenetStore) GetMaxKeys() int64 {
	return s.maxKeys
}

// Hits returns the number of cache hits
func (s *RAMFreenetStore) Hits() int64 {
	return atomic.LoadInt64(&s.hits)
}

// Misses returns the number of cache misses
func (s *RAMFreenetStore) Misses() int64 {
	return atomic.LoadInt64(&s.misses)
}

// Writes returns the number of writes
func (s *RAMFreenetStore) Writes() int64 {
	return atomic.LoadInt64(&s.writes)
}

// KeyCount returns the current number of keys
func (s *RAMFreenetStore) KeyCount() int64 {
	return atomic.LoadInt64(&s.keyCount)
}

// GetBloomFalsePositive returns 0 (no bloom filter in RAM store)
func (s *RAMFreenetStore) GetBloomFalsePositive() int64 {
	return 0
}

// ProbablyInStore checks if a key is in the store
func (s *RAMFreenetStore) ProbablyInStore(routingKey []byte) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	key := string(routingKey)
	_, exists := s.blocksByRoutingKey[key]
	return exists
}

// GetStats returns statistics about the store
func (s *RAMFreenetStore) GetStats() StoreStats {
	return StoreStats{
		Hits:       s.Hits(),
		Misses:     s.Misses(),
		Writes:     s.Writes(),
		KeyCount:   s.KeyCount(),
		MaxKeys:    s.GetMaxKeys(),
		HitRate:    s.getHitRate(),
		Capacity:   float64(s.KeyCount()) / float64(s.GetMaxKeys()),
	}
}

func (s *RAMFreenetStore) getHitRate() float64 {
	hits := float64(s.Hits())
	total := hits + float64(s.Misses())
	if total == 0 {
		return 0.0
	}
	return hits / total
}

// StoreStats contains statistics about store performance
type StoreStats struct {
	Hits     int64
	Misses   int64
	Writes   int64
	KeyCount int64
	MaxKeys  int64
	HitRate  float64
	Capacity float64
}

// String returns a formatted string of store statistics
func (ss StoreStats) String() string {
	return fmt.Sprintf("Store Stats: Keys=%d/%d (%.1f%% full), Hits=%d, Misses=%d, Writes=%d, Hit Rate=%.2f%%",
		ss.KeyCount, ss.MaxKeys, ss.Capacity*100, ss.Hits, ss.Misses, ss.Writes, ss.HitRate*100)
}
