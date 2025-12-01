// GoHyphanet - Hyphanet Node Implementation
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
)

const (
	// IdentityLength is the length of a node identity
	IdentityLength = 32
)

// NodeIdentity represents a node's cryptographic identity
type NodeIdentity struct {
	// Private ECDSA key for signing
	ecdsaPrivateKey *ecdsa.PrivateKey
	// Public ECDSA key
	ecdsaPublicKey *ecdsa.PublicKey
	// Identity hash (SHA256 of identity)
	identityHash [IdentityLength]byte
	// Identity hash hash (SHA256 of identity hash)
	identityHashHash [IdentityLength]byte
	// Raw identity bytes
	identity []byte
}

// NewNodeIdentity creates a new node identity with generated keys
func NewNodeIdentity() (*NodeIdentity, error) {
	// Generate ECDSA P-256 keypair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ECDSA key: %w", err)
	}

	ni := &NodeIdentity{
		ecdsaPrivateKey: privateKey,
		ecdsaPublicKey:  &privateKey.PublicKey,
	}

	// Generate identity from public key
	ni.identity = elliptic.Marshal(elliptic.P256(), ni.ecdsaPublicKey.X, ni.ecdsaPublicKey.Y)

	// Compute identity hash (setup key)
	hash := sha256.Sum256(ni.identity)
	ni.identityHash = hash

	// Compute hash of hash
	hashHash := sha256.Sum256(hash[:])
	ni.identityHashHash = hashHash

	return ni, nil
}

// GetIdentity returns the raw identity bytes
func (ni *NodeIdentity) GetIdentity() []byte {
	return ni.identity
}

// GetIdentityHash returns the identity hash (setup key)
func (ni *NodeIdentity) GetIdentityHash() []byte {
	return ni.identityHash[:]
}

// GetIdentityHashHash returns the hash of the identity hash
func (ni *NodeIdentity) GetIdentityHashHash() []byte {
	return ni.identityHashHash[:]
}

// GetPublicKey returns the ECDSA public key
func (ni *NodeIdentity) GetPublicKey() *ecdsa.PublicKey {
	return ni.ecdsaPublicKey
}

// GetPrivateKey returns the ECDSA private key
func (ni *NodeIdentity) GetPrivateKey() *ecdsa.PrivateKey {
	return ni.ecdsaPrivateKey
}

// Sign signs data with the ECDSA private key
func (ni *NodeIdentity) Sign(data []byte) ([]byte, error) {
	hash := sha256.Sum256(data)
	r, s, err := ecdsa.Sign(rand.Reader, ni.ecdsaPrivateKey, hash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	// Concatenate r and s (32 bytes each for P-256)
	signature := make([]byte, 64)
	r.FillBytes(signature[0:32])
	s.FillBytes(signature[32:64])

	return signature, nil
}

// Verify verifies a signature
func (ni *NodeIdentity) Verify(data, signature []byte, pubKey *ecdsa.PublicKey) bool {
	if len(signature) != 64 {
		return false
	}

	hash := sha256.Sum256(data)

	r := new(big.Int).SetBytes(signature[0:32])
	s := new(big.Int).SetBytes(signature[32:64])

	return ecdsa.Verify(pubKey, hash[:], r, s)
}

// DecodeFreenetIdentity decodes a Freenet base64 identity string
// Returns the raw identity and its hash
func DecodeFreenetIdentity(identityBase64 string) ([]byte, []byte, error) {
	// Freenet uses ~ instead of = for base64
	normalizedBase64 := strings.ReplaceAll(identityBase64, "~", "=")

	// Add padding if needed
	padding := (4 - (len(normalizedBase64) % 4)) % 4
	normalizedBase64 = normalizedBase64 + strings.Repeat("=", padding)

	// Decode
	identity, err := base64.StdEncoding.DecodeString(normalizedBase64)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to decode identity: %w", err)
	}

	// Hash it
	hash := sha256.Sum256(identity)

	return identity, hash[:], nil
}
