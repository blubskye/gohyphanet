// GoHyphanet - JFK Protocol Implementation
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package protocol

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"net"

	"github.com/blubskye/gohyphanet/node/crypto"
)

const (
	// JFK protocol constants
	JFKPhase0 = 0 // Message 1
	JFKPhase1 = 1 // Message 2
	JFKPhase2 = 2 // Message 3
	JFKPhase3 = 3 // Message 4

	// Setup types
	SetupOpennetSeednode = 1
	SetupDarknet         = 0

	// Negotiation types
	NegType10 = 10 // Modern nodes
)

// JFKContext tracks the state of a JFK handshake
type JFKContext struct {
	// Our side
	NonceInitiator     []byte
	NonceInitiatorHash []byte
	ECDHContext        *crypto.ECDHContext

	// Their side
	NonceResponder    []byte
	ResponderExponent []byte
	ResponderSignature []byte
	Authenticator     []byte

	// Shared
	SharedSecret []byte

	// State
	Phase        int
	SetupType    int
	NegType      int
	RemoteAddr   *net.UDPAddr
}

// NewJFKContext creates a new JFK handshake context
func NewJFKContext(nodeIdentity *crypto.NodeIdentity, unknownInitiator bool, setupType int) (*JFKContext, error) {
	// Generate ECDH context
	ecdhCtx, err := crypto.NewECDHContext(nodeIdentity)
	if err != nil {
		return nil, fmt.Errorf("failed to create ECDH context: %w", err)
	}

	ctx := &JFKContext{
		ECDHContext: ecdhCtx,
		Phase:       JFKPhase0,
		SetupType:   setupType,
		NegType:     NegType10,
	}

	return ctx, nil
}

// BuildMessage1 constructs JFK Message 1
func (ctx *JFKContext) BuildMessage1(nodeIdentity *crypto.NodeIdentity, unknownInitiator bool) ([]byte, []byte, error) {
	msg, nonce, err := crypto.BuildJFKMessage1(ctx.ECDHContext, unknownInitiator, nodeIdentity.GetIdentityHash())
	if err != nil {
		return nil, nil, err
	}

	// Store our nonce
	ctx.NonceInitiator = nonce

	// Hash the nonce for storage
	nonceHash := sha256.Sum256(nonce)
	ctx.NonceInitiatorHash = nonceHash[:]

	return crypto.SerializeJFKMessage1(msg, unknownInitiator), nonce, nil
}

// ProcessMessage2 handles JFK Message 2
func (ctx *JFKContext) ProcessMessage2(data []byte, transientKey []byte, peerIP net.IP) error {
	msg, err := crypto.ParseJFKMessage2(data)
	if err != nil {
		return fmt.Errorf("failed to parse message 2: %w", err)
	}

	// Verify nonce matches
	if !hmac.Equal(msg.NonceInitiatorHash, ctx.NonceInitiatorHash) {
		return fmt.Errorf("nonce mismatch")
	}

	// Store responder's data
	ctx.NonceResponder = msg.NonceResponder
	ctx.ResponderExponent = msg.ResponderExponent
	ctx.ResponderSignature = msg.Signature
	ctx.Authenticator = msg.Authenticator

	// Compute shared secret
	sharedSecret, err := ctx.ECDHContext.ComputeSharedSecret(msg.ResponderExponent)
	if err != nil {
		return fmt.Errorf("failed to compute shared secret: %w", err)
	}
	ctx.SharedSecret = sharedSecret

	// Verify authenticator
	ourExponent := ctx.ECDHContext.GetPublicKeyBytes()
	expectedAuth := crypto.AssembleJFKAuthenticator(
		transientKey,
		msg.ResponderExponent,
		ourExponent,
		msg.NonceResponder,
		ctx.NonceInitiator,
		peerIP,
	)

	if !hmac.Equal(msg.Authenticator, expectedAuth) {
		return fmt.Errorf("authenticator verification failed")
	}

	ctx.Phase = JFKPhase1
	return nil
}

// BuildMessage3 constructs JFK Message 3
func (ctx *JFKContext) BuildMessage3(nodeRef []byte, transientKey []byte, peerIP net.IP) ([]byte, error) {
	if ctx.Phase != JFKPhase1 {
		return nil, fmt.Errorf("invalid phase for message 3: %d", ctx.Phase)
	}

	// Build authenticator
	ourExponent := ctx.ECDHContext.GetPublicKeyBytes()
	authenticator := crypto.AssembleJFKAuthenticator(
		transientKey,
		ourExponent,
		ctx.ResponderExponent,
		ctx.NonceInitiator,
		ctx.NonceResponder,
		peerIP,
	)

	// Build message: Ni + Nr + g^i + authenticator + noderef
	message := make([]byte, 0, len(ctx.NonceInitiator)+len(ctx.NonceResponder)+
		len(ourExponent)+len(authenticator)+len(nodeRef))

	message = append(message, ctx.NonceInitiator...)
	message = append(message, ctx.NonceResponder...)
	message = append(message, ourExponent...)
	message = append(message, authenticator...)
	message = append(message, nodeRef...)

	ctx.Phase = JFKPhase2
	return message, nil
}

// ProcessMessage4 handles JFK Message 4
func (ctx *JFKContext) ProcessMessage4(data []byte, transientKey []byte, peerIP net.IP) ([]byte, error) {
	if ctx.Phase != JFKPhase2 {
		return nil, fmt.Errorf("invalid phase for message 4: %d", ctx.Phase)
	}

	// Parse message 4
	// Format: Ni + Nr + g^r + authenticator + noderef
	minSize := len(ctx.NonceInitiator) + len(ctx.NonceResponder) +
		len(ctx.ResponderExponent) + crypto.HashLength

	if len(data) < minSize {
		return nil, fmt.Errorf("message 4 too short")
	}

	offset := 0

	// Skip Ni
	offset += len(ctx.NonceInitiator)

	// Skip Nr
	offset += len(ctx.NonceResponder)

	// Skip g^r
	offset += len(ctx.ResponderExponent)

	// Extract authenticator
	authenticator := data[offset : offset+crypto.HashLength]
	offset += crypto.HashLength

	// Verify authenticator
	expectedAuth := crypto.AssembleJFKAuthenticator(
		transientKey,
		ctx.ResponderExponent,
		ctx.ECDHContext.GetPublicKeyBytes(),
		ctx.NonceResponder,
		ctx.NonceInitiator,
		peerIP,
	)

	if !hmac.Equal(authenticator, expectedAuth) {
		return nil, fmt.Errorf("message 4 authenticator verification failed")
	}

	// Extract noderef (rest of message)
	nodeRef := data[offset:]

	ctx.Phase = JFKPhase3
	return nodeRef, nil
}

// IsComplete returns true if the handshake is complete
func (ctx *JFKContext) IsComplete() bool {
	return ctx.Phase == JFKPhase3
}

// DeriveSessionKeys derives session encryption keys from the shared secret
func (ctx *JFKContext) DeriveSessionKeys() ([]byte, []byte, error) {
	if !ctx.IsComplete() {
		return nil, nil, fmt.Errorf("handshake not complete")
	}

	// Derive keys from shared secret
	// Outgoing key: HMAC(sharedSecret, "O" + Ni + Nr)
	// Incoming key: HMAC(sharedSecret, "I" + Ni + Nr)

	hashO := hmac.New(sha256.New, ctx.SharedSecret)
	hashO.Write([]byte("O"))
	hashO.Write(ctx.NonceInitiator)
	hashO.Write(ctx.NonceResponder)
	outgoingKey := hashO.Sum(nil)

	hashI := hmac.New(sha256.New, ctx.SharedSecret)
	hashI.Write([]byte("I"))
	hashI.Write(ctx.NonceInitiator)
	hashI.Write(ctx.NonceResponder)
	incomingKey := hashI.Sum(nil)

	return outgoingKey, incomingKey, nil
}
