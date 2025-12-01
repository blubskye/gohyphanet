// GoHyphanet - Peer Node Management
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package peer

import (
	"net"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/node/crypto"
	"github.com/blubskye/gohyphanet/node/protocol"
)

// PeerState represents the connection state of a peer
type PeerState int

const (
	StateDisconnected PeerState = iota
	StateHandshaking
	StateConnected
	StateFailed
)

func (s PeerState) String() string {
	switch s {
	case StateDisconnected:
		return "Disconnected"
	case StateHandshaking:
		return "Handshaking"
	case StateConnected:
		return "Connected"
	case StateFailed:
		return "Failed"
	default:
		return "Unknown"
	}
}

// Peer represents a connected or connectable Hyphanet node
type Peer struct {
	// Identity
	Identity     []byte  // Node identity (ECDSA public key)
	IdentityHash []byte  // SHA256(identity) - used as cipher key
	Location     float64 // Routing location (0.0-1.0)

	// Network
	Address     *net.UDPAddr
	LastContact time.Time
	LastSent    time.Time

	// State
	State          PeerState
	ConnectedSince time.Time

	// Handshake
	JFKContext *protocol.JFKContext

	// Session
	OutgoingKey []byte // Session key for outgoing packets
	IncomingKey []byte // Session key for incoming packets

	// Capabilities
	Version     int
	NegTypes    []int
	IsSeedNode  bool
	IsOpennet   bool
	IsDarknet   bool

	// Statistics
	BytesSent     uint64
	BytesReceived uint64
	MessagesIn    uint64
	MessagesOut   uint64
	SuccessRate   float64

	// Internal
	mu sync.RWMutex
}

// NewPeer creates a new peer
func NewPeer(addr *net.UDPAddr, isSeedNode bool) *Peer {
	return &Peer{
		Address:    addr,
		State:      StateDisconnected,
		IsSeedNode: isSeedNode,
		NegTypes:   []int{protocol.NegType10},
	}
}

// NewSeedPeer creates a peer from a seed node identity
func NewSeedPeer(addr *net.UDPAddr, identityBase64 string) (*Peer, error) {
	peer := NewPeer(addr, true)
	peer.IsOpennet = true

	// Decode and set identity
	if identityBase64 != "" {
		identity, identityHash, err := crypto.DecodeFreenetIdentity(identityBase64)
		if err != nil {
			return nil, err
		}
		peer.Identity = identity
		peer.IdentityHash = identityHash
	}

	return peer, nil
}

// GetState returns the current state
func (p *Peer) GetState() PeerState {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.State
}

// SetState updates the peer state
func (p *Peer) SetState(state PeerState) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.State = state

	if state == StateConnected && p.ConnectedSince.IsZero() {
		p.ConnectedSince = time.Now()
	}
}

// IsConnected returns true if the peer is connected
func (p *Peer) IsConnected() bool {
	return p.GetState() == StateConnected
}

// IsHandshaking returns true if handshake is in progress
func (p *Peer) IsHandshaking() bool {
	return p.GetState() == StateHandshaking
}

// UpdateLastContact updates the last contact time
func (p *Peer) UpdateLastContact() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.LastContact = time.Now()
}

// UpdateLastSent updates the last sent time
func (p *Peer) UpdateLastSent() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.LastSent = time.Now()
}

// RecordBytesSent records bytes sent to this peer
func (p *Peer) RecordBytesSent(bytes int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.BytesSent += uint64(bytes)
}

// RecordBytesReceived records bytes received from this peer
func (p *Peer) RecordBytesReceived(bytes int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.BytesReceived += uint64(bytes)
}

// RecordMessageSent records a message sent
func (p *Peer) RecordMessageSent() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.MessagesOut++
}

// RecordMessageReceived records a message received
func (p *Peer) RecordMessageReceived() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.MessagesIn++
}

// GetSessionKeys returns the session keys
func (p *Peer) GetSessionKeys() (outgoing, incoming []byte) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.OutgoingKey, p.IncomingKey
}

// SetSessionKeys sets the session keys
func (p *Peer) SetSessionKeys(outgoing, incoming []byte) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.OutgoingKey = outgoing
	p.IncomingKey = incoming
}

// String returns a string representation of the peer
func (p *Peer) String() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.Address != nil {
		return p.Address.String()
	}
	return "unknown"
}
