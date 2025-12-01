package store

import (
	"crypto/dsa"
	"crypto/sha256"
	"fmt"

	"github.com/blubskye/gohyphanet/node/keys"
)

// StorableBlock is the minimal interface for blocks that can be stored
type StorableBlock interface {
	// GetRoutingKey returns the routing key for network routing
	GetRoutingKey() []byte

	// GetFullKey returns the complete key with type information
	GetFullKey() []byte

	// Equals checks if two blocks are equal
	Equals(other StorableBlock) bool
}

// KeyBlock extends StorableBlock with key-specific methods
type KeyBlock interface {
	StorableBlock

	// GetKey returns the underlying key (NodeCHK or NodeSSK)
	GetKey() keys.Key

	// GetRawHeaders returns the raw header bytes
	GetRawHeaders() []byte

	// GetRawData returns the raw data bytes
	GetRawData() []byte

	// GetPubkeyBytes returns the serialized public key (SSK only, nil for CHK)
	GetPubkeyBytes() []byte
}

// BlockMetadata contains metadata about a block fetch/store operation
type BlockMetadata struct {
	oldBlock bool
}

// IsOldBlock returns whether the block is marked as old
func (m *BlockMetadata) IsOldBlock() bool {
	return m.oldBlock
}

// SetOldBlock marks the block as old
func (m *BlockMetadata) SetOldBlock() {
	m.oldBlock = true
}

// NewBlockMetadata creates a new BlockMetadata
func NewBlockMetadata() *BlockMetadata {
	return &BlockMetadata{oldBlock: false}
}

// StoreCallback defines the interface for type-specific store operations
type StoreCallback interface {
	// Fixed-size parameters
	DataLength() int
	HeaderLength() int
	RoutingKeyLength() int
	FullKeyLength() int

	// Storage configuration
	StoreFullKeys() bool
	CollisionPossible() bool
	ConstructNeedsKey() bool

	// Block construction from raw data
	Construct(data, headers, routingKey, fullKey []byte,
		canReadClientCache, canReadSlashdotCache bool,
		meta *BlockMetadata, knownPubKey *dsa.PublicKey) (KeyBlock, error)

	// Extract routing key from full key
	RoutingKeyFromFullKey(keyBuf []byte) []byte
}

// FreenetStore is the main interface for all datastores
type FreenetStore interface {
	// Fetch retrieves a block by routing key
	Fetch(routingKey, fullKey []byte, dontPromote, canReadClientCache,
		canReadSlashdotCache, ignoreOldBlocks bool, meta *BlockMetadata) (StorableBlock, error)

	// Put stores a block
	Put(block StorableBlock, data, header []byte, overwrite, oldBlock bool) error

	// SetMaxKeys changes the store size
	SetMaxKeys(maxStoreKeys int64, shrinkNow bool) error

	// GetMaxKeys returns the maximum number of keys
	GetMaxKeys() int64

	// Statistics
	Hits() int64
	Misses() int64
	Writes() int64
	KeyCount() int64
	GetBloomFalsePositive() int64

	// ProbablyInStore checks if a key might be in the store (bloom filter)
	ProbablyInStore(routingKey []byte) bool

	// Lifecycle
	Start() error
	Close() error
}

// CHKStoreCallback implements StoreCallback for CHK blocks
type CHKStoreCallback struct {
	store FreenetStore
}

// NewCHKStoreCallback creates a new CHK store callback
func NewCHKStoreCallback(store FreenetStore) *CHKStoreCallback {
	return &CHKStoreCallback{store: store}
}

// DataLength returns the fixed CHK data length
func (c *CHKStoreCallback) DataLength() int {
	return CHKDataLength
}

// HeaderLength returns the fixed CHK header length
func (c *CHKStoreCallback) HeaderLength() int {
	return CHKTotalHeadersLength
}

// FullKeyLength returns the full CHK key length (34 bytes)
func (c *CHKStoreCallback) FullKeyLength() int {
	return 34 // 2 bytes type + 32 bytes routing key
}

// RoutingKeyLength returns the CHK routing key length (32 bytes)
func (c *CHKStoreCallback) RoutingKeyLength() int {
	return 32
}

// StoreFullKeys returns true - CHK blocks store full keys
func (c *CHKStoreCallback) StoreFullKeys() bool {
	return true
}

// CollisionPossible returns false - CHK blocks are content-addressed
func (c *CHKStoreCallback) CollisionPossible() bool {
	return false
}

// ConstructNeedsKey returns false - CHK construction doesn't need the key
func (c *CHKStoreCallback) ConstructNeedsKey() bool {
	return false
}

// Construct creates a CHK block from raw data
func (c *CHKStoreCallback) Construct(data, headers, routingKey, fullKey []byte,
	canReadClientCache, canReadSlashdotCache bool,
	meta *BlockMetadata, knownPubKey *dsa.PublicKey) (KeyBlock, error) {

	if data == nil || headers == nil {
		return nil, fmt.Errorf("need data and headers")
	}

	// Extract crypto algorithm from full key
	var cryptoAlgorithm byte
	if fullKey != nil && len(fullKey) >= 2 {
		cryptoAlgorithm = fullKey[1]
	} else {
		cryptoAlgorithm = keys.AlgoAESCTR256SHA256 // Default
	}

	// Create NodeCHK
	nodeKey, err := keys.NewNodeCHK(routingKey, cryptoAlgorithm)
	if err != nil {
		return nil, err
	}

	// Create CHK block with verification
	return NewCHKBlock(data, headers, nodeKey, true)
}

// RoutingKeyFromFullKey extracts the routing key from a full key
func (c *CHKStoreCallback) RoutingKeyFromFullKey(keyBuf []byte) []byte {
	if len(keyBuf) == 32 {
		return keyBuf
	}
	if len(keyBuf) != 34 {
		return nil
	}
	// Skip 2-byte type prefix
	return keyBuf[2:34]
}

// SSKStoreCallback implements StoreCallback for SSK blocks
type SSKStoreCallback struct {
	store FreenetStore
	// pubkeyCache would go here in a full implementation
}

// NewSSKStoreCallback creates a new SSK store callback
func NewSSKStoreCallback(store FreenetStore) *SSKStoreCallback {
	return &SSKStoreCallback{store: store}
}

// DataLength returns the fixed SSK data length
func (s *SSKStoreCallback) DataLength() int {
	return SSKDataLength
}

// HeaderLength returns the fixed SSK header length
func (s *SSKStoreCallback) HeaderLength() int {
	return SSKTotalHeadersLength
}

// FullKeyLength returns the full SSK key length (66 bytes)
func (s *SSKStoreCallback) FullKeyLength() int {
	return 66 // 2 bytes type + 32 bytes ehDocname + 32 bytes pubKeyHash
}

// RoutingKeyLength returns the SSK routing key length (32 bytes)
func (s *SSKStoreCallback) RoutingKeyLength() int {
	return 32
}

// StoreFullKeys returns true - SSK blocks store full keys
func (s *SSKStoreCallback) StoreFullKeys() bool {
	return true
}

// CollisionPossible returns true - SSK blocks can be updated
func (s *SSKStoreCallback) CollisionPossible() bool {
	return true
}

// ConstructNeedsKey returns true - SSK construction needs the public key
func (s *SSKStoreCallback) ConstructNeedsKey() bool {
	return true
}

// Construct creates an SSK block from raw data
func (s *SSKStoreCallback) Construct(data, headers, routingKey, fullKey []byte,
	canReadClientCache, canReadSlashdotCache bool,
	meta *BlockMetadata, knownPubKey *dsa.PublicKey) (KeyBlock, error) {

	if data == nil || headers == nil {
		return nil, fmt.Errorf("need data and headers")
	}
	if fullKey == nil {
		return nil, fmt.Errorf("need full key to reconstruct SSK")
	}

	// Extract crypto algorithm from full key
	var cryptoAlgorithm byte
	if len(fullKey) >= 2 {
		cryptoAlgorithm = fullKey[1]
	} else {
		cryptoAlgorithm = keys.AlgoAESPCFB256SHA256 // Default for SSK
	}

	// Extract ehDocname and pubKeyHash from full key
	if len(fullKey) != 66 {
		return nil, fmt.Errorf("invalid full key length: %d", len(fullKey))
	}

	ehDocname := fullKey[2:34]
	pubKeyHash := fullKey[34:66]

	// Create NodeSSK
	nodeKey, err := keys.NewNodeSSK(pubKeyHash, ehDocname, knownPubKey, cryptoAlgorithm)
	if err != nil {
		return nil, err
	}

	// Create SSK block with verification
	return NewSSKBlock(data, headers, nodeKey, knownPubKey == nil)
}

// RoutingKeyFromFullKey extracts the routing key from a full key
func (s *SSKStoreCallback) RoutingKeyFromFullKey(keyBuf []byte) []byte {
	if len(keyBuf) == 32 {
		return keyBuf
	}
	if len(keyBuf) != 66 {
		return nil
	}

	// For SSK, need to compute routing key from ehDocname and pubKeyHash
	ehDocname := keyBuf[2:34]
	pubKeyHash := keyBuf[34:66]

	// Compute H(ehDocname + pubKeyHash)
	hasher := sha256.New()
	hasher.Write(ehDocname)
	hasher.Write(pubKeyHash)
	return hasher.Sum(nil)
}
