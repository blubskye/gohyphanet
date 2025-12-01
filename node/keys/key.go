package keys

import (
	"crypto/sha256"
	"io"
	"math/big"
)

// Key constants
const (
	// Crypto algorithms
	AlgoAESPCFB256SHA256 byte = 2
	AlgoAESCTR256SHA256  byte = 3

	// Hash algorithms
	HashSHA256 = 1

	// Key lengths
	RoutingKeyLength = 32
	CryptoKeyLength  = 32

	// Base types
	CHKBaseType uint8 = 1
	SSKBaseType uint8 = 2
)

// Key is the base interface for all Freenet keys
type Key interface {
	// GetRoutingKey returns the routing key bytes used for network routing
	GetRoutingKey() []byte

	// GetKeyBytes returns the key-specific bytes (implementation-dependent)
	GetKeyBytes() []byte

	// GetFullKey returns the complete serialized key including type
	GetFullKey() []byte

	// GetType returns the key type (high byte = base type, low byte = crypto algorithm)
	GetType() uint16

	// ToNormalizedDouble converts the key to a 0.0-1.0 value for location-based routing
	ToNormalizedDouble() float64

	// Write serializes the key to a writer
	Write(w io.Writer) error

	// Clone creates a deep copy of the key
	Clone() Key

	// Equals checks if two keys are equal
	Equals(other Key) bool
}

// BaseKey provides common functionality for all key types
type BaseKey struct {
	routingKey             []byte
	cachedNormalizedDouble float64
	hashCode               int
}

// GetRoutingKey returns the routing key
func (k *BaseKey) GetRoutingKey() []byte {
	return k.routingKey
}

// ToNormalizedDouble converts key to 0.0-1.0 range for routing
// This matches the Java implementation's keyDigestAsNormalizedDouble
func (k *BaseKey) ToNormalizedDouble(keyType uint16) float64 {
	if k.cachedNormalizedDouble > 0 {
		return k.cachedNormalizedDouble
	}

	// Hash: SHA256(routingKey || keyType)
	h := sha256.New()
	h.Write(k.routingKey)
	h.Write([]byte{byte(keyType >> 8), byte(keyType & 0xFF)})
	digest := h.Sum(nil)

	k.cachedNormalizedDouble = keyDigestAsNormalizedDouble(digest)
	return k.cachedNormalizedDouble
}

// keyDigestAsNormalizedDouble converts a hash digest to a 0.0-1.0 double
// Matches Java's MessageDigest conversion logic
func keyDigestAsNormalizedDouble(digest []byte) float64 {
	// Convert first 8 bytes to a long, then normalize to 0.0-1.0
	// Java uses: digest[0] to digest[7] as a signed long
	var value int64
	for i := 0; i < 8 && i < len(digest); i++ {
		value = (value << 8) | int64(digest[i])
	}

	// Convert signed long to unsigned by adding 2^63 if negative
	// Then divide by 2^64 to get 0.0-1.0 range
	bigValue := new(big.Int).SetInt64(value)
	if value < 0 {
		// Add 2^63
		shift := new(big.Int).Lsh(big.NewInt(1), 63)
		bigValue.Add(bigValue, shift)
	}

	// Convert to float in 0.0-1.0 range by dividing by 2^63
	divisor := new(big.Float).SetInt(new(big.Int).Lsh(big.NewInt(1), 63))
	result := new(big.Float).SetInt(bigValue)
	result.Quo(result, divisor)

	normalized, _ := result.Float64()
	return normalized
}

// calculateHash computes a hash code for the key (matches Java hashCode)
func calculateHash(data ...[]byte) int {
	h := sha256.New()
	for _, d := range data {
		h.Write(d)
	}
	digest := h.Sum(nil)

	// Convert first 4 bytes to int32
	hashCode := int(digest[0])<<24 | int(digest[1])<<16 | int(digest[2])<<8 | int(digest[3])
	return hashCode
}
