package keys

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

const (
	NodeCHKKeyLength     = 32
	NodeCHKFullKeyLength = 34 // 2 (type) + 32 (routing key)
)

// NodeCHK represents a node-level Content Hash Key (immutable content)
// This is used for routing and does not contain decryption information
type NodeCHK struct {
	BaseKey
	cryptoAlgorithm byte
}

// NewNodeCHK creates a new NodeCHK from a routing key and crypto algorithm
func NewNodeCHK(routingKey []byte, cryptoAlgorithm byte) (*NodeCHK, error) {
	if len(routingKey) != NodeCHKKeyLength {
		return nil, fmt.Errorf("routing key must be %d bytes, got %d", NodeCHKKeyLength, len(routingKey))
	}

	if cryptoAlgorithm != AlgoAESPCFB256SHA256 && cryptoAlgorithm != AlgoAESCTR256SHA256 {
		return nil, fmt.Errorf("invalid crypto algorithm: %d", cryptoAlgorithm)
	}

	// Make a copy to ensure immutability
	keyCopy := make([]byte, NodeCHKKeyLength)
	copy(keyCopy, routingKey)

	return &NodeCHK{
		BaseKey: BaseKey{
			routingKey: keyCopy,
			hashCode:   calculateHash(keyCopy),
		},
		cryptoAlgorithm: cryptoAlgorithm,
	}, nil
}

// GetType returns the key type (high byte = CHK, low byte = algorithm)
func (nchk *NodeCHK) GetType() uint16 {
	return (uint16(CHKBaseType) << 8) | uint16(nchk.cryptoAlgorithm)
}

// GetKeyBytes returns the routing key for CHK
func (nchk *NodeCHK) GetKeyBytes() []byte {
	return nchk.routingKey
}

// GetFullKey returns the complete serialized key
func (nchk *NodeCHK) GetFullKey() []byte {
	buf := make([]byte, NodeCHKFullKeyLength)
	keyType := nchk.GetType()
	buf[0] = byte(keyType >> 8)
	buf[1] = byte(keyType & 0xFF)
	copy(buf[2:], nchk.routingKey)
	return buf
}

// ToNormalizedDouble converts key to 0.0-1.0 range for routing
func (nchk *NodeCHK) ToNormalizedDouble() float64 {
	return nchk.BaseKey.ToNormalizedDouble(nchk.GetType())
}

// Write serializes the key to a writer
func (nchk *NodeCHK) Write(w io.Writer) error {
	keyType := nchk.GetType()
	if err := binary.Write(w, binary.BigEndian, keyType); err != nil {
		return err
	}
	_, err := w.Write(nchk.routingKey)
	return err
}

// Clone creates a deep copy of the key
func (nchk *NodeCHK) Clone() Key {
	keyCopy := make([]byte, len(nchk.routingKey))
	copy(keyCopy, nchk.routingKey)

	return &NodeCHK{
		BaseKey: BaseKey{
			routingKey:             keyCopy,
			hashCode:               nchk.hashCode,
			cachedNormalizedDouble: nchk.cachedNormalizedDouble,
		},
		cryptoAlgorithm: nchk.cryptoAlgorithm,
	}
}

// Equals checks if two keys are equal
func (nchk *NodeCHK) Equals(other Key) bool {
	otherCHK, ok := other.(*NodeCHK)
	if !ok {
		return false
	}
	return nchk.cryptoAlgorithm == otherCHK.cryptoAlgorithm &&
		bytes.Equal(nchk.routingKey, otherCHK.routingKey)
}

// GetCryptoAlgorithm returns the crypto algorithm
func (nchk *NodeCHK) GetCryptoAlgorithm() byte {
	return nchk.cryptoAlgorithm
}

// ReadNodeCHK deserializes a NodeCHK from a reader
func ReadNodeCHK(r io.Reader, cryptoAlgorithm byte) (*NodeCHK, error) {
	routingKey := make([]byte, NodeCHKKeyLength)
	if _, err := io.ReadFull(r, routingKey); err != nil {
		return nil, fmt.Errorf("failed to read routing key: %w", err)
	}
	return NewNodeCHK(routingKey, cryptoAlgorithm)
}

// ReadNodeCHKWithType deserializes a NodeCHK including its type prefix
func ReadNodeCHKWithType(r io.Reader) (*NodeCHK, error) {
	var keyType uint16
	if err := binary.Read(r, binary.BigEndian, &keyType); err != nil {
		return nil, fmt.Errorf("failed to read key type: %w", err)
	}

	baseType := byte(keyType >> 8)
	if baseType != CHKBaseType {
		return nil, fmt.Errorf("not a CHK key, got base type %d", baseType)
	}

	cryptoAlgorithm := byte(keyType & 0xFF)
	return ReadNodeCHK(r, cryptoAlgorithm)
}
