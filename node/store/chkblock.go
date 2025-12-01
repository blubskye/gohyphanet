package store

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"

	"github.com/blubskye/gohyphanet/node/keys"
)

const (
	// CHKBlock constants
	CHKDataLength          = 32768 // Fixed data block size
	CHKTotalHeadersLength  = 36    // Fixed header size
	CHKMaxCompressedLength = 32764 // Maximum compressed payload (DATA_LENGTH - 4)
)

// CHKBlock represents a Content Hash Key block (immutable content)
type CHKBlock struct {
	data            []byte         // 32768 bytes of data
	headers         []byte         // 36 bytes of headers
	hashIdentifier  int16          // Must be keys.HashSHA256 (1)
	chk             *keys.NodeCHK
	clientKey       *keys.ClientCHK // Optional: full client key with decryption info
	cryptoAlgorithm byte
}

// NewCHKBlock creates a new CHKBlock with verification
func NewCHKBlock(data, headers []byte, key *keys.NodeCHK, verify bool) (*CHKBlock, error) {
	if len(data) != CHKDataLength {
		return nil, fmt.Errorf("data must be %d bytes, got %d", CHKDataLength, len(data))
	}
	if len(headers) != CHKTotalHeadersLength {
		return nil, fmt.Errorf("headers must be %d bytes, got %d", CHKTotalHeadersLength, len(headers))
	}
	if key == nil {
		return nil, fmt.Errorf("key cannot be nil")
	}

	// Extract hash identifier from headers
	hashIdentifier := int16(headers[0])<<8 | int16(headers[1])

	// Verify if requested
	if verify {
		if hashIdentifier != keys.HashSHA256 {
			return nil, fmt.Errorf("hash identifier %d is not SHA-256", hashIdentifier)
		}

		// Calculate hash: SHA-256(headers || data)
		hasher := sha256.New()
		hasher.Write(headers)
		hasher.Write(data)
		calculatedHash := hasher.Sum(nil)

		// Compare with routing key
		if !bytes.Equal(calculatedHash, key.GetRoutingKey()) {
			return nil, fmt.Errorf("hash verification failed")
		}
	}

	// Make copies to ensure immutability
	dataCopy := make([]byte, CHKDataLength)
	copy(dataCopy, data)
	headersCopy := make([]byte, CHKTotalHeadersLength)
	copy(headersCopy, headers)

	return &CHKBlock{
		data:            dataCopy,
		headers:         headersCopy,
		hashIdentifier:  hashIdentifier,
		chk:             key,
		cryptoAlgorithm: key.GetCryptoAlgorithm(),
	}, nil
}

// GetRoutingKey returns the routing key
func (b *CHKBlock) GetRoutingKey() []byte {
	return b.chk.GetRoutingKey()
}

// GetFullKey returns the full key (type + routing key)
func (b *CHKBlock) GetFullKey() []byte {
	return b.chk.GetFullKey()
}

// GetKey returns the underlying NodeCHK
func (b *CHKBlock) GetKey() keys.Key {
	return b.chk
}

// GetClientKey returns the client-level CHK key (if available)
func (b *CHKBlock) GetClientKey() *keys.ClientCHK {
	return b.clientKey
}

// SetClientKey sets the client-level CHK key
func (b *CHKBlock) SetClientKey(clientKey *keys.ClientCHK) {
	b.clientKey = clientKey
}

// GetRawData returns the raw data bytes
func (b *CHKBlock) GetRawData() []byte {
	return b.data
}

// GetRawHeaders returns the raw header bytes
func (b *CHKBlock) GetRawHeaders() []byte {
	return b.headers
}

// GetPubkeyBytes returns nil (CHK blocks don't have pubkeys)
func (b *CHKBlock) GetPubkeyBytes() []byte {
	return nil
}

// GetHashIdentifier returns the hash algorithm identifier
func (b *CHKBlock) GetHashIdentifier() int16 {
	return b.hashIdentifier
}

// GetCryptoAlgorithm returns the crypto algorithm
func (b *CHKBlock) GetCryptoAlgorithm() byte {
	return b.cryptoAlgorithm
}

// Equals checks if two CHKBlocks are equal
func (b *CHKBlock) Equals(other StorableBlock) bool {
	otherCHK, ok := other.(*CHKBlock)
	if !ok {
		return false
	}

	return bytes.Equal(b.GetRoutingKey(), otherCHK.GetRoutingKey()) &&
		bytes.Equal(b.data, otherCHK.data) &&
		bytes.Equal(b.headers, otherCHK.headers)
}

// Write serializes the block to a writer
func (b *CHKBlock) Write(w io.Writer) error {
	// Write headers
	if _, err := w.Write(b.headers); err != nil {
		return fmt.Errorf("failed to write headers: %w", err)
	}
	// Write data
	if _, err := w.Write(b.data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}
	return nil
}

// ReadCHKBlock deserializes a CHKBlock from a reader
func ReadCHKBlock(r io.Reader, key *keys.NodeCHK, verify bool) (*CHKBlock, error) {
	// Read headers
	headers := make([]byte, CHKTotalHeadersLength)
	if _, err := io.ReadFull(r, headers); err != nil {
		return nil, fmt.Errorf("failed to read headers: %w", err)
	}

	// Read data
	data := make([]byte, CHKDataLength)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}

	return NewCHKBlock(data, headers, key, verify)
}

// GetTotalLength returns the total block length (headers + data)
func (b *CHKBlock) GetTotalLength() int {
	return CHKTotalHeadersLength + CHKDataLength
}

// VerifyHash verifies the block's hash matches the key
func (b *CHKBlock) VerifyHash() error {
	if b.hashIdentifier != keys.HashSHA256 {
		return fmt.Errorf("hash identifier %d is not SHA-256", b.hashIdentifier)
	}

	hasher := sha256.New()
	hasher.Write(b.headers)
	hasher.Write(b.data)
	calculatedHash := hasher.Sum(nil)

	if !bytes.Equal(calculatedHash, b.chk.GetRoutingKey()) {
		return fmt.Errorf("hash verification failed")
	}

	return nil
}

// Clone creates a deep copy of the block
func (b *CHKBlock) Clone() *CHKBlock {
	dataCopy := make([]byte, len(b.data))
	copy(dataCopy, b.data)
	headersCopy := make([]byte, len(b.headers))
	copy(headersCopy, b.headers)

	return &CHKBlock{
		data:            dataCopy,
		headers:         headersCopy,
		hashIdentifier:  b.hashIdentifier,
		chk:             b.chk.Clone().(*keys.NodeCHK),
		cryptoAlgorithm: b.cryptoAlgorithm,
	}
}
