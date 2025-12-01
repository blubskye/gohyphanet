// GoHyphanet - Hyphanet Node Implementation
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package crypto

import (
	"crypto/ecdh"
	"crypto/rand"
	"fmt"
)

// ECDHContext holds an ECDH key exchange context
type ECDHContext struct {
	privateKey *ecdh.PrivateKey
	publicKey  *ecdh.PublicKey
	signature  []byte // ECDSA signature of the public key
	lifetime   int64  // timestamp when created
}

// NewECDHContext creates a new ECDH context using P-256 curve
func NewECDHContext(nodeIdentity *NodeIdentity) (*ECDHContext, error) {
	// Generate ECDH P-256 key pair
	privateKey, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ECDH key: %w", err)
	}

	ctx := &ECDHContext{
		privateKey: privateKey,
		publicKey:  privateKey.PublicKey(),
	}

	// Sign the public key with node's ECDSA key
	pubKeyBytes := ctx.publicKey.Bytes()
	signature, err := nodeIdentity.Sign(pubKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to sign public key: %w", err)
	}

	ctx.signature = signature

	return ctx, nil
}

// GetPublicKeyBytes returns the public key in network format
func (ctx *ECDHContext) GetPublicKeyBytes() []byte {
	return ctx.publicKey.Bytes()
}

// GetSignature returns the ECDSA signature of the public key
func (ctx *ECDHContext) GetSignature() []byte {
	return ctx.signature
}

// ComputeSharedSecret computes the shared secret with a peer's public key
func (ctx *ECDHContext) ComputeSharedSecret(peerPubKey []byte) ([]byte, error) {
	// Parse peer's public key
	pubKey, err := ecdh.P256().NewPublicKey(peerPubKey)
	if err != nil {
		return nil, fmt.Errorf("invalid peer public key: %w", err)
	}

	// Compute shared secret
	secret, err := ctx.privateKey.ECDH(pubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to compute shared secret: %w", err)
	}

	return secret, nil
}
