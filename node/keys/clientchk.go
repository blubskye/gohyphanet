package keys

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	ClientCHKExtraLength     = 5
	ClientCHKCryptoKeyLength = 32
)

// Compression algorithms
const (
	CompressionNone   int16 = -1
	CompressionGZIP   int16 = 0
	CompressionBZIP2  int16 = 1
	CompressionLZMA   int16 = 2
	CompressionLZMA_NEW int16 = 3
)

// ClientCHK represents a client-level Content Hash Key
// Contains both routing information and decryption key
type ClientCHK struct {
	routingKey           []byte // 32 bytes - hash of encrypted content
	cryptoKey            []byte // 32 bytes - decryption key
	cryptoAlgorithm      byte   // encryption algorithm
	compressionAlgorithm int16  // -1 = uncompressed
	controlDocument      bool   // true = metadata, false = data
	hashCodeValue        int
	cachedNodeKey        *NodeCHK
}

// NewClientCHK creates a new ClientCHK
func NewClientCHK(routingKey, cryptoKey []byte, cryptoAlgorithm byte, compressionAlgorithm int16, controlDocument bool) (*ClientCHK, error) {
	if len(routingKey) != NodeCHKKeyLength {
		return nil, fmt.Errorf("routing key must be %d bytes", NodeCHKKeyLength)
	}
	if len(cryptoKey) != ClientCHKCryptoKeyLength {
		return nil, fmt.Errorf("crypto key must be %d bytes", ClientCHKCryptoKeyLength)
	}
	if cryptoAlgorithm != AlgoAESPCFB256SHA256 && cryptoAlgorithm != AlgoAESCTR256SHA256 {
		return nil, fmt.Errorf("invalid crypto algorithm: %d", cryptoAlgorithm)
	}

	// Make copies to ensure immutability
	routingKeyCopy := make([]byte, NodeCHKKeyLength)
	copy(routingKeyCopy, routingKey)
	cryptoKeyCopy := make([]byte, ClientCHKCryptoKeyLength)
	copy(cryptoKeyCopy, cryptoKey)

	return &ClientCHK{
		routingKey:           routingKeyCopy,
		cryptoKey:            cryptoKeyCopy,
		cryptoAlgorithm:      cryptoAlgorithm,
		compressionAlgorithm: compressionAlgorithm,
		controlDocument:      controlDocument,
		hashCodeValue:        calculateHash(routingKeyCopy, cryptoKeyCopy),
	}, nil
}

// NewClientCHKFromURI creates a ClientCHK from a FreenetURI
func NewClientCHKFromURI(uri *FreenetURI) (*ClientCHK, error) {
	if uri.KeyType != "CHK" {
		return nil, fmt.Errorf("not a CHK URI")
	}

	if len(uri.Extra) < ClientCHKExtraLength {
		return nil, fmt.Errorf("extra bytes too short: need %d, got %d", ClientCHKExtraLength, len(uri.Extra))
	}

	cryptoAlgorithm := uri.Extra[1]
	if cryptoAlgorithm != AlgoAESPCFB256SHA256 && cryptoAlgorithm != AlgoAESCTR256SHA256 {
		return nil, fmt.Errorf("invalid crypto algorithm: %d", cryptoAlgorithm)
	}

	controlDocument := (uri.Extra[2] & 0x02) != 0
	compressionAlgorithm := int16(uri.Extra[3])<<8 | int16(uri.Extra[4])

	return NewClientCHK(uri.RoutingKey, uri.CryptoKey, cryptoAlgorithm, compressionAlgorithm, controlDocument)
}

// NewClientCHKFromData creates a ClientCHK by encrypting data and computing its hash
// This is used for inserting new data into the network
func NewClientCHKFromData(data []byte) (*ClientCHK, error) {
	// Generate random encryption key
	cryptoKey := make([]byte, ClientCHKCryptoKeyLength)
	if _, err := rand.Read(cryptoKey); err != nil {
		return nil, fmt.Errorf("failed to generate crypto key: %w", err)
	}

	// Use AES-CTR-256 as default algorithm
	cryptoAlgorithm := AlgoAESCTR256SHA256

	// Encrypt the data using the crypto key
	encryptedData, err := EncryptDataCTR(data, cryptoKey, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt data: %w", err)
	}

	// Create headers (hash identifier)
	headers := make([]byte, 36) // CHK header size
	headers[0] = 0x00            // Hash identifier high byte
	headers[1] = 0x01            // Hash identifier low byte (SHA-256)

	// Compute routing key: SHA256(headers || encrypted data)
	hasher := sha256.New()
	hasher.Write(headers)
	hasher.Write(encryptedData)
	routingKey := hasher.Sum(nil)

	// Create the ClientCHK
	return NewClientCHK(routingKey, cryptoKey, cryptoAlgorithm, CompressionNone, false)
}

// GetExtra returns the extra bytes for this CHK
func (c *ClientCHK) GetExtra() []byte {
	extra := make([]byte, ClientCHKExtraLength)
	extra[0] = 0 // Reserved, currently 0
	extra[1] = c.cryptoAlgorithm
	if c.controlDocument {
		extra[2] = 2
	}
	extra[3] = byte(c.compressionAlgorithm >> 8)
	extra[4] = byte(c.compressionAlgorithm & 0xFF)
	return extra
}

// GetURI converts this ClientCHK to a FreenetURI
func (c *ClientCHK) GetURI() *FreenetURI {
	return &FreenetURI{
		KeyType:    "CHK",
		RoutingKey: c.routingKey,
		CryptoKey:  c.cryptoKey,
		Extra:      c.GetExtra(),
	}
}

// GetNodeCHK returns the corresponding NodeCHK (for routing)
func (c *ClientCHK) GetNodeCHK() *NodeCHK {
	if c.cachedNodeKey == nil {
		c.cachedNodeKey, _ = NewNodeCHK(c.routingKey, c.cryptoAlgorithm)
	}
	return c.cachedNodeKey
}

// GetRoutingKey returns the routing key
func (c *ClientCHK) GetRoutingKey() []byte {
	return c.routingKey
}

// GetCryptoKey returns the decryption key
func (c *ClientCHK) GetCryptoKey() []byte {
	return c.cryptoKey
}

// GetCryptoAlgorithm returns the crypto algorithm
func (c *ClientCHK) GetCryptoAlgorithm() byte {
	return c.cryptoAlgorithm
}

// GetCompressionAlgorithm returns the compression algorithm
func (c *ClientCHK) GetCompressionAlgorithm() int16 {
	return c.compressionAlgorithm
}

// IsControlDocument returns whether this is a control document (metadata)
func (c *ClientCHK) IsControlDocument() bool {
	return c.controlDocument
}

// WriteRawBinaryKey serializes the key in compact binary format
func (c *ClientCHK) WriteRawBinaryKey(w io.Writer) error {
	// Write extra bytes
	if _, err := w.Write(c.GetExtra()); err != nil {
		return err
	}
	// Write routing key
	if _, err := w.Write(c.routingKey); err != nil {
		return err
	}
	// Write crypto key
	_, err := w.Write(c.cryptoKey)
	return err
}

// ReadClientCHK deserializes a ClientCHK from a reader
func ReadClientCHK(r io.Reader) (*ClientCHK, error) {
	// Read extra bytes
	extra := make([]byte, ClientCHKExtraLength)
	if _, err := io.ReadFull(r, extra); err != nil {
		return nil, fmt.Errorf("failed to read extra bytes: %w", err)
	}

	cryptoAlgorithm := extra[1]
	controlDocument := (extra[2] & 0x02) != 0
	compressionAlgorithm := int16(extra[3])<<8 | int16(extra[4])

	// Read routing key
	routingKey := make([]byte, NodeCHKKeyLength)
	if _, err := io.ReadFull(r, routingKey); err != nil {
		return nil, fmt.Errorf("failed to read routing key: %w", err)
	}

	// Read crypto key
	cryptoKey := make([]byte, ClientCHKCryptoKeyLength)
	if _, err := io.ReadFull(r, cryptoKey); err != nil {
		return nil, fmt.Errorf("failed to read crypto key: %w", err)
	}

	return NewClientCHK(routingKey, cryptoKey, cryptoAlgorithm, compressionAlgorithm, controlDocument)
}

// Equals checks if two ClientCHKs are equal
func (c *ClientCHK) Equals(other *ClientCHK) bool {
	if other == nil {
		return false
	}
	return c.cryptoAlgorithm == other.cryptoAlgorithm &&
		c.compressionAlgorithm == other.compressionAlgorithm &&
		c.controlDocument == other.controlDocument &&
		bytes.Equal(c.routingKey, other.routingKey) &&
		bytes.Equal(c.cryptoKey, other.cryptoKey)
}

// HashCode returns a hash code for this key
func (c *ClientCHK) HashCode() int {
	return c.hashCodeValue
}

// Clone creates a deep copy of this ClientCHK
func (c *ClientCHK) Clone() *ClientCHK {
	routingKeyCopy := make([]byte, len(c.routingKey))
	copy(routingKeyCopy, c.routingKey)
	cryptoKeyCopy := make([]byte, len(c.cryptoKey))
	copy(cryptoKeyCopy, c.cryptoKey)

	clone := &ClientCHK{
		routingKey:           routingKeyCopy,
		cryptoKey:            cryptoKeyCopy,
		cryptoAlgorithm:      c.cryptoAlgorithm,
		compressionAlgorithm: c.compressionAlgorithm,
		controlDocument:      c.controlDocument,
		hashCodeValue:        c.hashCodeValue,
	}

	// Don't copy cached node key - let it be recreated if needed
	return clone
}

// ToNormalizedDouble converts key to 0.0-1.0 range for routing
func (c *ClientCHK) ToNormalizedDouble() float64 {
	return c.GetNodeCHK().ToNormalizedDouble()
}

// WriteBinary writes the key in binary format with type prefix
func (c *ClientCHK) WriteBinary(w io.Writer) error {
	// Write key type byte (1 = CHK)
	if err := binary.Write(w, binary.BigEndian, byte(1)); err != nil {
		return err
	}
	return c.WriteRawBinaryKey(w)
}
