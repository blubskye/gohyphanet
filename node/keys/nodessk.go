package keys

import (
	"bytes"
	"crypto/dsa"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	NodeSSKPubKeyHashSize   = 32
	NodeSSKEHDocnameSize    = 32
	NodeSSKRoutingKeyLength = 32
	NodeSSKFullKeyLength    = 66 // 2 (type) + 32 (ehDocname) + 32 (pubKeyHash)
	NodeSSKVersion          = 1
)

// NodeSSK represents a node-level Signed Subspace Key (mutable content)
// This is used for routing and does not contain decryption information
type NodeSSK struct {
	BaseKey
	cryptoAlgorithm        byte
	pubKeyHash             []byte // 32 bytes - SHA256(public key)
	encryptedHashedDocname []byte // 32 bytes - E(H(docname))
	pubKey                 *dsa.PublicKey
	hashCodeValue          int
}

// NewNodeSSK creates a new NodeSSK
func NewNodeSSK(pubKeyHash, ehDocname []byte, pubKey *dsa.PublicKey, cryptoAlgorithm byte) (*NodeSSK, error) {
	if len(pubKeyHash) != NodeSSKPubKeyHashSize {
		return nil, fmt.Errorf("pubKeyHash must be %d bytes, got %d", NodeSSKPubKeyHashSize, len(pubKeyHash))
	}
	if len(ehDocname) != NodeSSKEHDocnameSize {
		return nil, fmt.Errorf("ehDocname must be %d bytes, got %d", NodeSSKEHDocnameSize, len(ehDocname))
	}
	if cryptoAlgorithm != AlgoAESPCFB256SHA256 && cryptoAlgorithm != AlgoAESCTR256SHA256 {
		return nil, fmt.Errorf("invalid crypto algorithm: %d", cryptoAlgorithm)
	}

	// Verify pubKey matches pubKeyHash if provided
	if pubKey != nil {
		computedHash := hashPublicKey(pubKey)
		if !bytes.Equal(computedHash, pubKeyHash) {
			return nil, fmt.Errorf("pubKey hash mismatch")
		}
	}

	// Make copies to ensure immutability
	pubKeyHashCopy := make([]byte, NodeSSKPubKeyHashSize)
	copy(pubKeyHashCopy, pubKeyHash)
	ehDocnameCopy := make([]byte, NodeSSKEHDocnameSize)
	copy(ehDocnameCopy, ehDocname)

	// Calculate routing key: H(E(H(docname)) + H(pubkey))
	routingKey := makeSSKRoutingKey(pubKeyHashCopy, ehDocnameCopy)

	return &NodeSSK{
		BaseKey: BaseKey{
			routingKey: routingKey,
			hashCode:   calculateHash(routingKey),
		},
		cryptoAlgorithm:        cryptoAlgorithm,
		pubKeyHash:             pubKeyHashCopy,
		encryptedHashedDocname: ehDocnameCopy,
		pubKey:                 pubKey,
		hashCodeValue:          calculateHash(pubKeyHashCopy, ehDocnameCopy),
	}, nil
}

// makeSSKRoutingKey calculates the SSK routing key: H(ehDocname + pubKeyHash)
func makeSSKRoutingKey(pubKeyHash, ehDocname []byte) []byte {
	h := sha256.New()
	h.Write(ehDocname)
	h.Write(pubKeyHash)
	sum := h.Sum(nil)
	return sum
}

// hashPublicKey computes SHA256(public key bytes)
func hashPublicKey(pubKey *dsa.PublicKey) []byte {
	// Serialize public key to bytes (implementation depends on DSA key format)
	// For now, use a simple serialization
	keyBytes := pubKey.Y.Bytes()
	hash := sha256.Sum256(keyBytes)
	return hash[:]
}

// GetType returns the key type (high byte = SSK, low byte = algorithm)
func (nssk *NodeSSK) GetType() uint16 {
	return (uint16(SSKBaseType) << 8) | uint16(nssk.cryptoAlgorithm)
}

// GetKeyBytes returns the encrypted hashed docname for SSK
func (nssk *NodeSSK) GetKeyBytes() []byte {
	return nssk.encryptedHashedDocname
}

// GetFullKey returns the complete serialized key
func (nssk *NodeSSK) GetFullKey() []byte {
	buf := make([]byte, NodeSSKFullKeyLength)
	keyType := nssk.GetType()
	buf[0] = byte(keyType >> 8)
	buf[1] = byte(keyType & 0xFF)
	copy(buf[2:], nssk.encryptedHashedDocname)
	copy(buf[2+NodeSSKEHDocnameSize:], nssk.pubKeyHash)
	return buf
}

// ToNormalizedDouble converts key to 0.0-1.0 range for routing
func (nssk *NodeSSK) ToNormalizedDouble() float64 {
	return nssk.BaseKey.ToNormalizedDouble(nssk.GetType())
}

// Write serializes the key to a writer
func (nssk *NodeSSK) Write(w io.Writer) error {
	keyType := nssk.GetType()
	if err := binary.Write(w, binary.BigEndian, keyType); err != nil {
		return err
	}
	if _, err := w.Write(nssk.encryptedHashedDocname); err != nil {
		return err
	}
	_, err := w.Write(nssk.pubKeyHash)
	return err
}

// Clone creates a deep copy of the key
func (nssk *NodeSSK) Clone() Key {
	pubKeyHashCopy := make([]byte, len(nssk.pubKeyHash))
	copy(pubKeyHashCopy, nssk.pubKeyHash)
	ehDocnameCopy := make([]byte, len(nssk.encryptedHashedDocname))
	copy(ehDocnameCopy, nssk.encryptedHashedDocname)
	routingKeyCopy := make([]byte, len(nssk.routingKey))
	copy(routingKeyCopy, nssk.routingKey)

	return &NodeSSK{
		BaseKey: BaseKey{
			routingKey:             routingKeyCopy,
			hashCode:               nssk.hashCode,
			cachedNormalizedDouble: nssk.cachedNormalizedDouble,
		},
		cryptoAlgorithm:        nssk.cryptoAlgorithm,
		pubKeyHash:             pubKeyHashCopy,
		encryptedHashedDocname: ehDocnameCopy,
		pubKey:                 nssk.pubKey, // DSA keys are immutable
		hashCodeValue:          nssk.hashCodeValue,
	}
}

// Equals checks if two keys are equal
func (nssk *NodeSSK) Equals(other Key) bool {
	otherSSK, ok := other.(*NodeSSK)
	if !ok {
		return false
	}
	return nssk.cryptoAlgorithm == otherSSK.cryptoAlgorithm &&
		bytes.Equal(nssk.pubKeyHash, otherSSK.pubKeyHash) &&
		bytes.Equal(nssk.encryptedHashedDocname, otherSSK.encryptedHashedDocname)
}

// GetPubKeyHash returns the public key hash
func (nssk *NodeSSK) GetPubKeyHash() []byte {
	return nssk.pubKeyHash
}

// GetEncryptedHashedDocname returns the encrypted hashed docname
func (nssk *NodeSSK) GetEncryptedHashedDocname() []byte {
	return nssk.encryptedHashedDocname
}

// GetCryptoAlgorithm returns the crypto algorithm
func (nssk *NodeSSK) GetCryptoAlgorithm() byte {
	return nssk.cryptoAlgorithm
}

// GetPubKey returns the public key if available
func (nssk *NodeSSK) GetPubKey() *dsa.PublicKey {
	return nssk.pubKey
}

// SetPubKey sets the public key if it matches the hash
func (nssk *NodeSSK) SetPubKey(pubKey *dsa.PublicKey) error {
	if nssk.pubKey != nil {
		// Already set, verify it's the same
		if nssk.pubKey.Y.Cmp(pubKey.Y) == 0 {
			return nil
		}
		return fmt.Errorf("pubKey already set to different value")
	}

	// Verify hash matches
	computedHash := hashPublicKey(pubKey)
	if !bytes.Equal(computedHash, nssk.pubKeyHash) {
		return fmt.Errorf("pubkey hash mismatch")
	}

	nssk.pubKey = pubKey
	return nil
}

// ReadNodeSSK deserializes a NodeSSK from a reader
func ReadNodeSSK(r io.Reader, cryptoAlgorithm byte) (*NodeSSK, error) {
	ehDocname := make([]byte, NodeSSKEHDocnameSize)
	if _, err := io.ReadFull(r, ehDocname); err != nil {
		return nil, fmt.Errorf("failed to read ehDocname: %w", err)
	}

	pubKeyHash := make([]byte, NodeSSKPubKeyHashSize)
	if _, err := io.ReadFull(r, pubKeyHash); err != nil {
		return nil, fmt.Errorf("failed to read pubKeyHash: %w", err)
	}

	return NewNodeSSK(pubKeyHash, ehDocname, nil, cryptoAlgorithm)
}

// ReadNodeSSKWithType deserializes a NodeSSK including its type prefix
func ReadNodeSSKWithType(r io.Reader) (*NodeSSK, error) {
	var keyType uint16
	if err := binary.Read(r, binary.BigEndian, &keyType); err != nil {
		return nil, fmt.Errorf("failed to read key type: %w", err)
	}

	baseType := byte(keyType >> 8)
	if baseType != SSKBaseType {
		return nil, fmt.Errorf("not an SSK key, got base type %d", baseType)
	}

	cryptoAlgorithm := byte(keyType & 0xFF)
	return ReadNodeSSK(r, cryptoAlgorithm)
}
