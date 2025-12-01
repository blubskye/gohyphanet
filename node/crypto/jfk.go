// GoHyphanet - Hyphanet Node Implementation
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package crypto

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

const (
	// JFK protocol constants
	HashLength          = 32 // SHA256
	NonceSize           = 32
	ModulusLengthP256   = 65 // Uncompressed ECDH P-256 public key
	SignatureLengthP256 = 64 // ECDSA P-256 signature (r+s)

	// Setup types
	SetupOpennetSeednode = 1
)

var (
	JFKPrefixInitiator = []byte("I")
	JFKPrefixResponder = []byte("R")
)

// JFKMessage1 represents the first JFK handshake message
type JFKMessage1 struct {
	NonceHash          []byte // SHA256(Nonce)
	InitiatorExponent  []byte // g^i (ECDH public key)
	InitiatorIdentHash []byte // Hash of initiator identity (for unknown initiator)
}

// BuildJFKMessage1 constructs a JFK Message 1
func BuildJFKMessage1(ecdhCtx *ECDHContext, unknownInitiator bool, identityHash []byte) (*JFKMessage1, []byte, error) {
	// Generate random nonce
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Hash the nonce
	nonceHashArr := sha256.Sum256(nonce)
	nonceHash := nonceHashArr[:]

	// Get exponential (public key)
	exponential := ecdhCtx.GetPublicKeyBytes()

	msg := &JFKMessage1{
		NonceHash:         nonceHash,
		InitiatorExponent: exponential,
	}

	// Calculate message size
	messageSize := HashLength + len(exponential)
	if unknownInitiator {
		messageSize += IdentityLength
		msg.InitiatorIdentHash = identityHash
	}

	// Construct message
	message := make([]byte, messageSize)
	offset := 0

	// Nonce hash
	copy(message[offset:], nonceHash)
	offset += HashLength

	// Exponential
	copy(message[offset:], exponential)
	offset += len(exponential)

	// Identity hash (if unknown initiator)
	if unknownInitiator {
		copy(message[offset:], identityHash)
	}

	return msg, nonce, nil
}

// SerializeJFKMessage1 serializes a JFK Message 1 to bytes
func SerializeJFKMessage1(msg *JFKMessage1, unknownInitiator bool) []byte {
	messageSize := HashLength + len(msg.InitiatorExponent)
	if unknownInitiator {
		messageSize += IdentityLength
	}

	message := make([]byte, messageSize)
	offset := 0

	copy(message[offset:], msg.NonceHash)
	offset += HashLength

	copy(message[offset:], msg.InitiatorExponent)
	offset += len(msg.InitiatorExponent)

	if unknownInitiator {
		copy(message[offset:], msg.InitiatorIdentHash)
	}

	return message
}

// JFKMessage2 represents the second JFK handshake message
type JFKMessage2 struct {
	NonceInitiatorHash []byte
	NonceResponder     []byte
	ResponderExponent  []byte
	Signature          []byte
	Authenticator      []byte
}

// ParseJFKMessage2 parses a JFK Message 2 from bytes
func ParseJFKMessage2(data []byte) (*JFKMessage2, error) {
	minSize := HashLength + NonceSize + ModulusLengthP256 + SignatureLengthP256 + HashLength
	if len(data) < minSize {
		return nil, fmt.Errorf("message too short: got %d, need at least %d", len(data), minSize)
	}

	msg := &JFKMessage2{}
	offset := 0

	// Nonce initiator hash
	msg.NonceInitiatorHash = make([]byte, HashLength)
	copy(msg.NonceInitiatorHash, data[offset:offset+HashLength])
	offset += HashLength

	// Nonce responder
	msg.NonceResponder = make([]byte, NonceSize)
	copy(msg.NonceResponder, data[offset:offset+NonceSize])
	offset += NonceSize

	// Responder exponential
	msg.ResponderExponent = make([]byte, ModulusLengthP256)
	copy(msg.ResponderExponent, data[offset:offset+ModulusLengthP256])
	offset += ModulusLengthP256

	// Signature
	msg.Signature = make([]byte, SignatureLengthP256)
	copy(msg.Signature, data[offset:offset+SignatureLengthP256])
	offset += SignatureLengthP256

	// Authenticator
	msg.Authenticator = make([]byte, HashLength)
	copy(msg.Authenticator, data[offset:offset+HashLength])

	return msg, nil
}

// AssembleJFKAuthenticator creates the JFK authenticator HMAC
func AssembleJFKAuthenticator(transientKey, exponentialA, exponentialB, nonceA, nonceB []byte, ipAddress []byte) []byte {
	// Concatenate: exponentialA + exponentialB + nonceA + nonceB + ipAddress
	data := make([]byte, 0, len(exponentialA)+len(exponentialB)+len(nonceA)+len(nonceB)+len(ipAddress))
	data = append(data, exponentialA...)
	data = append(data, exponentialB...)
	data = append(data, nonceA...)
	data = append(data, nonceB...)
	data = append(data, ipAddress...)

	// Compute HMAC-SHA256
	mac := hmac.New(sha256.New, transientKey)
	mac.Write(data)
	return mac.Sum(nil)
}

// VerifyJFKAuthenticator verifies the JFK authenticator HMAC
func VerifyJFKAuthenticator(transientKey, exponentialA, exponentialB, nonceA, nonceB, ipAddress, authenticator []byte) bool {
	expected := AssembleJFKAuthenticator(transientKey, exponentialA, exponentialB, nonceA, nonceB, ipAddress)
	return hmac.Equal(expected, authenticator)
}
