package store

import (
	"bytes"
	"crypto/dsa"
	"crypto/sha256"
	"fmt"
	"io"
	"math/big"

	"github.com/blubskye/gohyphanet/node/keys"
)

const (
	// SSKBlock constants
	SSKDataLength             = 1024 // Fixed data block size
	SSKMaxCompressedLength    = 1022 // Maximum compressed payload (DATA_LENGTH - 2)
	SSKTotalHeadersLength     = 135  // Total header size
	SSKEncryptedHeadersLength = 36   // Length of encrypted portion
	SSKSigRLength             = 32   // Signature R component length
	SSKSigSLength             = 32   // Signature S component length
	SSKEHDocnameLength        = 32   // Encrypted hashed docname length
	SSKHeadersOffset          = 36   // Start of encrypted fields
)

// SSKBlock represents a Signed Subspace Key block (mutable content)
type SSKBlock struct {
	data                []byte            // 1024 bytes of data
	headers             []byte            // 135 bytes of headers
	headersOffset       int               // Start of encrypted fields (36)
	nodeKey             *keys.NodeSSK
	pubKey              *dsa.PublicKey
	hashIdentifier      int16
	symCipherIdentifier int16
}

// NewSSKBlock creates a new SSKBlock with signature verification
func NewSSKBlock(data, headers []byte, nodeKey *keys.NodeSSK, dontVerify bool) (*SSKBlock, error) {
	if len(data) != SSKDataLength {
		return nil, fmt.Errorf("data must be %d bytes, got %d", SSKDataLength, len(data))
	}
	if len(headers) != SSKTotalHeadersLength {
		return nil, fmt.Errorf("headers must be %d bytes, got %d", SSKTotalHeadersLength, len(headers))
	}
	if nodeKey == nil {
		return nil, fmt.Errorf("nodeKey cannot be nil")
	}

	// Get public key from node key
	pubKey := nodeKey.GetPubKey()
	if pubKey == nil {
		return nil, fmt.Errorf("public key is required for SSK block")
	}

	// Parse headers
	hashIdentifier := int16(headers[0])<<8 | int16(headers[1])
	if hashIdentifier != keys.HashSHA256 {
		return nil, fmt.Errorf("hash identifier %d is not SHA-256", hashIdentifier)
	}

	symCipherIdentifier := int16(headers[2])<<8 | int16(headers[3])

	// Extract E(H(docname))
	ehDocname := headers[4:36]

	// Verify E(H(docname)) matches the key
	if !bytes.Equal(ehDocname, nodeKey.GetEncryptedHashedDocname()) {
		return nil, fmt.Errorf("E(H(docname)) mismatch - wrong key")
	}

	// Verify signature if requested
	if !dontVerify {
		// Extract signature components from headers
		// [72-103]: R (32 bytes, unsigned)
		// [104-135]: S (32 bytes, unsigned)
		if len(headers) < 136 {
			return nil, fmt.Errorf("headers too short for signature")
		}

		bufR := headers[72:104]
		bufS := headers[104:136]

		// Compute data hash
		dataHasher := sha256.New()
		dataHasher.Write(data)
		dataHash := dataHasher.Sum(nil)

		// Compute overall hash: SHA-256(headers[0:72] || dataHash)
		overallHasher := sha256.New()
		overallHasher.Write(headers[:72])
		overallHasher.Write(dataHash)
		overallHash := overallHasher.Sum(nil)

		// Verify DSA signature
		r := new(big.Int).SetBytes(bufR)
		s := new(big.Int).SetBytes(bufS)

		if !dsa.Verify(pubKey, overallHash, r, s) {
			return nil, fmt.Errorf("DSA signature verification failed")
		}
	}

	// Make copies to ensure immutability
	dataCopy := make([]byte, SSKDataLength)
	copy(dataCopy, data)
	headersCopy := make([]byte, SSKTotalHeadersLength)
	copy(headersCopy, headers)

	return &SSKBlock{
		data:                dataCopy,
		headers:             headersCopy,
		headersOffset:       SSKHeadersOffset,
		nodeKey:             nodeKey,
		pubKey:              pubKey,
		hashIdentifier:      hashIdentifier,
		symCipherIdentifier: symCipherIdentifier,
	}, nil
}

// GetRoutingKey returns the routing key
func (b *SSKBlock) GetRoutingKey() []byte {
	return b.nodeKey.GetRoutingKey()
}

// GetFullKey returns the full key (type + ehDocname + pubKeyHash)
func (b *SSKBlock) GetFullKey() []byte {
	return b.nodeKey.GetFullKey()
}

// GetKey returns the underlying NodeSSK
func (b *SSKBlock) GetKey() keys.Key {
	return b.nodeKey
}

// GetRawData returns the raw data bytes
func (b *SSKBlock) GetRawData() []byte {
	return b.data
}

// GetRawHeaders returns the raw header bytes
func (b *SSKBlock) GetRawHeaders() []byte {
	return b.headers
}

// GetPubkeyBytes returns the serialized public key
func (b *SSKBlock) GetPubkeyBytes() []byte {
	if b.pubKey == nil {
		return nil
	}
	// Serialize DSA public key (Y value)
	return b.pubKey.Y.Bytes()
}

// GetPubKey returns the DSA public key
func (b *SSKBlock) GetPubKey() *dsa.PublicKey {
	return b.pubKey
}

// GetHashIdentifier returns the hash algorithm identifier
func (b *SSKBlock) GetHashIdentifier() int16 {
	return b.hashIdentifier
}

// GetSymCipherIdentifier returns the symmetric cipher identifier
func (b *SSKBlock) GetSymCipherIdentifier() int16 {
	return b.symCipherIdentifier
}

// GetEncryptedHeaders returns the encrypted portion of headers
func (b *SSKBlock) GetEncryptedHeaders() []byte {
	return b.headers[b.headersOffset : b.headersOffset+SSKEncryptedHeadersLength]
}

// Equals checks if two SSKBlocks are equal
func (b *SSKBlock) Equals(other StorableBlock) bool {
	otherSSK, ok := other.(*SSKBlock)
	if !ok {
		return false
	}

	return bytes.Equal(b.GetRoutingKey(), otherSSK.GetRoutingKey()) &&
		bytes.Equal(b.data, otherSSK.data) &&
		bytes.Equal(b.headers, otherSSK.headers)
}

// Write serializes the block to a writer
func (b *SSKBlock) Write(w io.Writer) error {
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

// ReadSSKBlock deserializes an SSKBlock from a reader
func ReadSSKBlock(r io.Reader, nodeKey *keys.NodeSSK, dontVerify bool) (*SSKBlock, error) {
	// Read headers
	headers := make([]byte, SSKTotalHeadersLength)
	if _, err := io.ReadFull(r, headers); err != nil {
		return nil, fmt.Errorf("failed to read headers: %w", err)
	}

	// Read data
	data := make([]byte, SSKDataLength)
	if _, err := io.ReadFull(r, data); err != nil {
		return nil, fmt.Errorf("failed to read data: %w", err)
	}

	return NewSSKBlock(data, headers, nodeKey, dontVerify)
}

// GetTotalLength returns the total block length (headers + data)
func (b *SSKBlock) GetTotalLength() int {
	return SSKTotalHeadersLength + SSKDataLength
}

// VerifySignature verifies the block's DSA signature
func (b *SSKBlock) VerifySignature() error {
	if b.pubKey == nil {
		return fmt.Errorf("no public key available")
	}

	// Extract signature components
	bufR := b.headers[72:104]
	bufS := b.headers[104:136]

	// Compute data hash
	dataHasher := sha256.New()
	dataHasher.Write(b.data)
	dataHash := dataHasher.Sum(nil)

	// Compute overall hash
	overallHasher := sha256.New()
	overallHasher.Write(b.headers[:72])
	overallHasher.Write(dataHash)
	overallHash := overallHasher.Sum(nil)

	// Verify signature
	r := new(big.Int).SetBytes(bufR)
	s := new(big.Int).SetBytes(bufS)

	if !dsa.Verify(b.pubKey, overallHash, r, s) {
		return fmt.Errorf("DSA signature verification failed")
	}

	return nil
}

// Clone creates a deep copy of the block
func (b *SSKBlock) Clone() *SSKBlock {
	dataCopy := make([]byte, len(b.data))
	copy(dataCopy, b.data)
	headersCopy := make([]byte, len(b.headers))
	copy(headersCopy, b.headers)

	return &SSKBlock{
		data:                dataCopy,
		headers:             headersCopy,
		headersOffset:       b.headersOffset,
		nodeKey:             b.nodeKey.Clone().(*keys.NodeSSK),
		pubKey:              b.pubKey, // DSA keys are immutable
		hashIdentifier:      b.hashIdentifier,
		symCipherIdentifier: b.symCipherIdentifier,
	}
}
