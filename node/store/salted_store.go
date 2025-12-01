package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
)

const (
	// Entry metadata size (128 bytes per slot)
	EntryMetadataSize = 128

	// Entry flags
	EntryFlagOccupied   = 0x01
	EntryFlagPlainKey   = 0x02
	EntryNewBlock       = 0x04
	EntryWrongStore     = 0x08

	// Slot filter flags
	SlotChecked    = 0x80000000
	SlotOccupied   = 0x40000000
	SlotNewBlock   = 0x20000000
	SlotWrongStore = 0x10000000

	// Probing
	OptionMaxProbe = 5
)

// SaltedHashFreenetStore is a persistent disk-based datastore
type SaltedHashFreenetStore struct {
	// Configuration
	basePath       string
	callback       StoreCallback
	storeSize      int64
	prevStoreSize  int64
	maxKeys        int64
	generation     int32
	salt           []byte // 16 bytes

	// Files
	configFile   *os.File
	metadataFile *os.File
	dataFile     *os.File

	// Slot filter
	slotFilter         []uint32
	slotFilterDisabled bool

	// Locks
	configLock sync.RWMutex
	keyLocks   map[string]*sync.RWMutex
	keyLocksMu sync.Mutex

	// Statistics
	hits                 int64
	misses               int64
	writes               int64
	keyCount             int64
	bloomFalsePositive   int64
	storeFileOffsetReady int64

	// State
	closed bool
}

// NewSaltedHashFreenetStore creates a new persistent datastore
func NewSaltedHashFreenetStore(basePath string, callback StoreCallback, maxKeys int64) (*SaltedHashFreenetStore, error) {
	if maxKeys <= 0 {
		maxKeys = 100000 // Default to 100k blocks
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %w", err)
	}

	// Generate or load salt
	salt, err := loadOrGenerateSalt(filepath.Join(basePath, "salt.dat"))
	if err != nil {
		return nil, err
	}

	store := &SaltedHashFreenetStore{
		basePath:           basePath,
		callback:           callback,
		storeSize:          maxKeys,
		maxKeys:            maxKeys,
		salt:               salt,
		keyLocks:           make(map[string]*sync.RWMutex),
		slotFilter:         make([]uint32, maxKeys),
		storeFileOffsetReady: maxKeys,
	}

	return store, nil
}

// Start initializes the store files
func (s *SaltedHashFreenetStore) Start() error {
	s.configLock.Lock()
	defer s.configLock.Unlock()

	var err error

	// Open or create metadata file
	metadataPath := filepath.Join(s.basePath, "metadata.dat")
	s.metadataFile, err = os.OpenFile(metadataPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("failed to open metadata file: %w", err)
	}

	// Open or create data file
	dataPath := filepath.Join(s.basePath, "data.dat")
	s.dataFile, err = os.OpenFile(dataPath, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		s.metadataFile.Close()
		return fmt.Errorf("failed to open data file: %w", err)
	}

	// Load or initialize slot filter
	slotFilterPath := filepath.Join(s.basePath, "slotfilter.dat")
	if err := s.loadSlotFilter(slotFilterPath); err != nil {
		// Initialize new slot filter
		s.slotFilter = make([]uint32, s.storeSize)
	}

	// Pre-allocate files if needed
	if err := s.ensureFileSize(); err != nil {
		return err
	}

	return nil
}

// Close shuts down the store
func (s *SaltedHashFreenetStore) Close() error {
	s.configLock.Lock()
	defer s.configLock.Unlock()

	if s.closed {
		return nil
	}

	s.closed = true

	// Save slot filter
	slotFilterPath := filepath.Join(s.basePath, "slotfilter.dat")
	if err := s.saveSlotFilter(slotFilterPath); err != nil {
		fmt.Printf("Warning: failed to save slot filter: %v\n", err)
	}

	// Close files
	if s.metadataFile != nil {
		s.metadataFile.Close()
	}
	if s.dataFile != nil {
		s.dataFile.Close()
	}
	if s.configFile != nil {
		s.configFile.Close()
	}

	return nil
}

// Fetch retrieves a block by routing key
func (s *SaltedHashFreenetStore) Fetch(routingKey, fullKey []byte, dontPromote, canReadClientCache,
	canReadSlashdotCache, ignoreOldBlocks bool, meta *BlockMetadata) (StorableBlock, error) {

	s.configLock.RLock()
	defer s.configLock.RUnlock()

	if s.closed {
		return nil, fmt.Errorf("store is closed")
	}

	// Get digested (salted) key
	digestedKey := s.getDigestedKey(routingKey)

	// Probe for entry
	entry, offset := s.probeEntry(digestedKey, routingKey, true)
	if entry == nil {
		atomic.AddInt64(&s.misses, 1)
		return nil, nil
	}

	// Check old block flag
	if ignoreOldBlocks && (entry.flag&EntryNewBlock) == 0 {
		atomic.AddInt64(&s.misses, 1)
		return nil, nil
	}

	// Read header and data
	header, data, err := s.readHeaderAndData(offset)
	if err != nil {
		atomic.AddInt64(&s.misses, 1)
		return nil, err
	}

	// Decrypt if needed
	if entry.encrypted {
		header, data, err = s.decryptEntry(header, data, entry.iv, routingKey)
		if err != nil {
			atomic.AddInt64(&s.misses, 1)
			return nil, err
		}
	}

	// Construct block
	block, err := s.callback.Construct(data, header, routingKey, fullKey,
		canReadClientCache, canReadSlashdotCache, meta, nil)

	if err != nil || block == nil {
		atomic.AddInt64(&s.misses, 1)
		return nil, err
	}

	atomic.AddInt64(&s.hits, 1)

	if meta != nil && (entry.flag&EntryNewBlock) == 0 {
		meta.SetOldBlock()
	}

	return block, nil
}

// Put stores a block
func (s *SaltedHashFreenetStore) Put(block StorableBlock, data, header []byte, overwrite, isOldBlock bool) error {
	s.configLock.RLock()
	defer s.configLock.RUnlock()

	if s.closed {
		return fmt.Errorf("store is closed")
	}

	routingKey := block.GetRoutingKey()
	fullKey := block.GetFullKey()
	digestedKey := s.getDigestedKey(routingKey)

	atomic.AddInt64(&s.writes, 1)

	// Check if already exists
	oldEntry, oldOffset := s.probeEntry(digestedKey, routingKey, false)
	if oldEntry != nil {
		if !s.callback.CollisionPossible() {
			// CHK: Update old block flag if needed
			if !isOldBlock && (oldEntry.flag&EntryNewBlock) == 0 {
				oldEntry.flag |= EntryNewBlock
				s.writeEntryMetadata(oldOffset, oldEntry)
			}
			return nil
		}

		// SSK: Check if identical or overwrite
		oldHeader, oldData, _ := s.readHeaderAndData(oldOffset)
		if oldEntry.encrypted {
			oldHeader, oldData, _ = s.decryptEntry(oldHeader, oldData, oldEntry.iv, routingKey)
		}

		identical := len(oldHeader) == len(header) && len(oldData) == len(data)
		if identical {
			for i := range header {
				if header[i] != oldHeader[i] {
					identical = false
					break
				}
			}
			for i := range data {
				if data[i] != oldData[i] {
					identical = false
					break
				}
			}
		}

		if identical {
			// Update old block flag if needed
			if !isOldBlock && (oldEntry.flag&EntryNewBlock) == 0 {
				oldEntry.flag |= EntryNewBlock
				s.writeEntryMetadata(oldOffset, oldEntry)
			}
			return nil
		}

		if !overwrite {
			return fmt.Errorf("key collision and overwrite not allowed")
		}

		// Overwrite at same offset
		return s.writeEntryAt(oldOffset, digestedKey, routingKey, fullKey, header, data, !isOldBlock)
	}

	// Find free slot
	offsets := getOffsetFromDigestedKey(digestedKey, s.storeSize)
	for _, offset := range offsets {
		if offset >= s.storeFileOffsetReady {
			continue
		}

		entry, _ := s.readEntryMetadata(offset)
		if entry == nil || (entry.flag&EntryFlagOccupied) == 0 {
			// Free slot found
			atomic.AddInt64(&s.keyCount, 1)
			return s.writeEntryAt(offset, digestedKey, routingKey, fullKey, header, data, !isOldBlock)
		}
	}

	// No free slot - overwrite first slot (collision)
	atomic.AddInt64(&s.keyCount, 1)
	return s.writeEntryAt(offsets[0], digestedKey, routingKey, fullKey, header, data, !isOldBlock)
}

// Helper functions

func (s *SaltedHashFreenetStore) getDigestedKey(routingKey []byte) []byte {
	h := sha256.New()
	h.Write(s.salt)
	h.Write(routingKey)
	return h.Sum(nil)
}

func getOffsetFromDigestedKey(digestedKey []byte, storeSize int64) []int64 {
	keyValue := bytesToInt64(digestedKey)
	offsets := make([]int64, OptionMaxProbe)

	for i := 0; i < OptionMaxProbe; i++ {
		// h + 141*i^2 + 13*i
		offset := ((keyValue + 141*int64(i*i) + 13*int64(i)) & 0x7FFFFFFFFFFFFFFF) % storeSize
		offsets[i] = offset
	}

	// Ensure all offsets are unique
	for i := 0; i < OptionMaxProbe; i++ {
		for j := 0; j < i; j++ {
			if offsets[i] == offsets[j] {
				offsets[i] = (offsets[i] + 1) % storeSize
			}
		}
	}

	return offsets
}

func bytesToInt64(b []byte) int64 {
	if len(b) < 8 {
		return 0
	}
	return int64(binary.BigEndian.Uint64(b[0:8]))
}

func (s *SaltedHashFreenetStore) probeEntry(digestedKey, routingKey []byte, withData bool) (*Entry, int64) {
	offsets := getOffsetFromDigestedKey(digestedKey, s.storeSize)

	for _, offset := range offsets {
		// Check slot filter first
		if !s.slotFilterDisabled && offset < int64(len(s.slotFilter)) {
			slotValue := atomic.LoadUint32(&s.slotFilter[offset])
			if !s.slotLikelyMatch(slotValue, digestedKey) {
				continue
			}
		}

		// Read entry
		entry, err := s.readEntry(offset, digestedKey, routingKey, withData)
		if err != nil {
			continue
		}

		if entry != nil && (entry.flag&EntryFlagOccupied) != 0 {
			return entry, offset
		}
	}

	return nil, -1
}

func (s *SaltedHashFreenetStore) slotLikelyMatch(slotValue uint32, digestedKey []byte) bool {
	if (slotValue & SlotChecked) == 0 {
		return false // Not checked yet
	}
	if (slotValue & SlotOccupied) == 0 {
		return false // Definitely empty
	}

	// Compare first 3 bytes
	wanted := uint32(digestedKey[2]) | (uint32(digestedKey[1]) << 8) | (uint32(digestedKey[0]) << 16)
	got := slotValue & 0xFFFFFF

	return wanted == got
}

// Entry represents metadata for a stored block
type Entry struct {
	digestedKey []byte // 32 bytes
	iv          []byte // 16 bytes for encryption
	flag        byte
	storeSize   int64
	plainKey    []byte // 32 bytes (optional)
	generation  int32
	encrypted   bool
}

func (s *SaltedHashFreenetStore) readEntry(offset int64, digestedKey, routingKey []byte, withData bool) (*Entry, error) {
	entry, err := s.readEntryMetadata(offset)
	if err != nil || entry == nil {
		return nil, err
	}

	// Verify digested key matches
	if len(digestedKey) == 32 && len(entry.digestedKey) == 32 {
		match := true
		for i := 0; i < 32; i++ {
			if digestedKey[i] != entry.digestedKey[i] {
				match = false
				break
			}
		}
		if !match {
			return nil, nil
		}
	}

	return entry, nil
}

func (s *SaltedHashFreenetStore) readEntryMetadata(offset int64) (*Entry, error) {
	buf := make([]byte, EntryMetadataSize)
	_, err := s.metadataFile.ReadAt(buf, offset*EntryMetadataSize)
	if err != nil {
		return nil, err
	}

	entry := &Entry{
		digestedKey: buf[0:32],
		iv:          buf[32:48],
		flag:        buf[48],
		generation:  int32(binary.BigEndian.Uint32(buf[96:100])),
		encrypted:   true,
	}

	// Check if plain key is stored
	if (entry.flag & EntryFlagPlainKey) != 0 {
		entry.plainKey = buf[64:96]
	}

	return entry, nil
}

func (s *SaltedHashFreenetStore) writeEntryMetadata(offset int64, entry *Entry) error {
	buf := make([]byte, EntryMetadataSize)

	copy(buf[0:32], entry.digestedKey)
	copy(buf[32:48], entry.iv)
	buf[48] = entry.flag

	if entry.plainKey != nil {
		copy(buf[64:96], entry.plainKey)
	}

	binary.BigEndian.PutUint32(buf[96:100], uint32(entry.generation))

	_, err := s.metadataFile.WriteAt(buf, offset*EntryMetadataSize)
	return err
}

func (s *SaltedHashFreenetStore) writeEntryAt(offset int64, digestedKey, routingKey, fullKey, header, data []byte, newBlock bool) error {
	// Generate random IV
	iv := make([]byte, 16)
	rand.Read(iv)

	// Encrypt data
	encHeader, encData, err := s.encryptEntry(header, data, iv, routingKey)
	if err != nil {
		return err
	}

	// Write header and data
	if err := s.writeHeaderAndData(offset, encHeader, encData); err != nil {
		return err
	}

	// Create entry metadata
	entry := &Entry{
		digestedKey: digestedKey,
		iv:          iv,
		flag:        EntryFlagOccupied | EntryFlagPlainKey,
		plainKey:    routingKey,
		generation:  atomic.LoadInt32(&s.generation),
		encrypted:   true,
	}

	if newBlock {
		entry.flag |= EntryNewBlock
	}

	// Write metadata
	if err := s.writeEntryMetadata(offset, entry); err != nil {
		return err
	}

	// Update slot filter
	if offset < int64(len(s.slotFilter)) {
		slotValue := uint32(SlotChecked | SlotOccupied)
		if newBlock {
			slotValue |= uint32(SlotNewBlock)
		}
		// Add first 3 bytes of digested key
		slotValue |= uint32(digestedKey[2]) | (uint32(digestedKey[1]) << 8) | (uint32(digestedKey[0]) << 16)
		atomic.StoreUint32(&s.slotFilter[offset], slotValue)
	}

	return nil
}

func (s *SaltedHashFreenetStore) readHeaderAndData(offset int64) ([]byte, []byte, error) {
	headerLen := s.callback.HeaderLength()
	dataLen := s.callback.DataLength()

	totalLen := int64(headerLen + dataLen)
	buf := make([]byte, totalLen)

	_, err := s.dataFile.ReadAt(buf, offset*totalLen)
	if err != nil {
		return nil, nil, err
	}

	return buf[:headerLen], buf[headerLen:], nil
}

func (s *SaltedHashFreenetStore) writeHeaderAndData(offset int64, header, data []byte) error {
	headerLen := s.callback.HeaderLength()
	dataLen := s.callback.DataLength()
	totalLen := int64(headerLen + dataLen)

	buf := make([]byte, totalLen)
	copy(buf, header)
	copy(buf[headerLen:], data)

	_, err := s.dataFile.WriteAt(buf, offset*totalLen)
	return err
}

func (s *SaltedHashFreenetStore) encryptEntry(header, data, iv, routingKey []byte) ([]byte, []byte, error) {
	// Create cipher with routing key + salt
	key := make([]byte, 32)
	h := sha256.New()
	h.Write(routingKey)
	h.Write(s.salt)
	copy(key, h.Sum(nil))

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}

	stream := cipher.NewCTR(block, iv)

	encHeader := make([]byte, len(header))
	encData := make([]byte, len(data))

	stream.XORKeyStream(encHeader, header)
	stream.XORKeyStream(encData, data)

	return encHeader, encData, nil
}

func (s *SaltedHashFreenetStore) decryptEntry(header, data, iv, routingKey []byte) ([]byte, []byte, error) {
	// CTR mode: encryption and decryption are the same
	return s.encryptEntry(header, data, iv, routingKey)
}

func (s *SaltedHashFreenetStore) ensureFileSize() error {
	headerLen := int64(s.callback.HeaderLength())
	dataLen := int64(s.callback.DataLength())

	// Metadata file size
	metadataSize := s.storeSize * EntryMetadataSize
	if err := s.metadataFile.Truncate(metadataSize); err != nil {
		return err
	}

	// Data file size
	dataSize := s.storeSize * (headerLen + dataLen)
	if err := s.dataFile.Truncate(dataSize); err != nil {
		return err
	}

	return nil
}

func (s *SaltedHashFreenetStore) loadSlotFilter(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return binary.Read(f, binary.BigEndian, &s.slotFilter)
}

func (s *SaltedHashFreenetStore) saveSlotFilter(path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	return binary.Write(f, binary.BigEndian, s.slotFilter)
}

// Standard interface methods

func (s *SaltedHashFreenetStore) SetMaxKeys(maxStoreKeys int64, shrinkNow bool) error {
	// Resizing would be complex - for now just update the limit
	atomic.StoreInt64(&s.maxKeys, maxStoreKeys)
	return nil
}

func (s *SaltedHashFreenetStore) GetMaxKeys() int64 {
	return atomic.LoadInt64(&s.maxKeys)
}

func (s *SaltedHashFreenetStore) Hits() int64 {
	return atomic.LoadInt64(&s.hits)
}

func (s *SaltedHashFreenetStore) Misses() int64 {
	return atomic.LoadInt64(&s.misses)
}

func (s *SaltedHashFreenetStore) Writes() int64 {
	return atomic.LoadInt64(&s.writes)
}

func (s *SaltedHashFreenetStore) KeyCount() int64 {
	return atomic.LoadInt64(&s.keyCount)
}

func (s *SaltedHashFreenetStore) GetBloomFalsePositive() int64 {
	return atomic.LoadInt64(&s.bloomFalsePositive)
}

func (s *SaltedHashFreenetStore) ProbablyInStore(routingKey []byte) bool {
	digestedKey := s.getDigestedKey(routingKey)
	offsets := getOffsetFromDigestedKey(digestedKey, s.storeSize)

	for _, offset := range offsets {
		if offset < int64(len(s.slotFilter)) {
			slotValue := atomic.LoadUint32(&s.slotFilter[offset])
			if s.slotLikelyMatch(slotValue, digestedKey) {
				return true
			}
		}
	}

	return false
}

// Helper function to load or generate salt
func loadOrGenerateSalt(path string) ([]byte, error) {
	// Try to load existing salt
	data, err := os.ReadFile(path)
	if err == nil && len(data) == 16 {
		return data, nil
	}

	// Generate new salt
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return nil, err
	}

	// Save salt
	if err := os.WriteFile(path, salt, 0600); err != nil {
		return nil, err
	}

	return salt, nil
}
