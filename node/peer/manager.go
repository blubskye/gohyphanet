// GoHyphanet - Peer Manager
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package peer

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// Manager handles all peer connections
type Manager struct {
	peers      map[string]*Peer // key: address string
	seedPeers  []*Peer
	darknetPeers []*Peer
	opennetPeers []*Peer

	mu         sync.RWMutex
	debugMode  bool
}

// NewManager creates a new peer manager
func NewManager(debugMode bool) *Manager {
	return &Manager{
		peers:     make(map[string]*Peer),
		seedPeers: make([]*Peer, 0),
		debugMode: debugMode,
	}
}

// AddSeedNode adds a seed node
func (m *Manager) AddSeedNode(addr *net.UDPAddr, identityBase64 string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	peer, err := NewSeedPeer(addr, identityBase64)
	if err != nil {
		return fmt.Errorf("failed to create seed peer: %w", err)
	}

	key := addr.String()
	m.peers[key] = peer
	m.seedPeers = append(m.seedPeers, peer)

	if m.debugMode {
		log.Printf("[PEERS] Added seed node: %s", addr)
	}

	return nil
}

// AddPeer adds a peer
func (m *Manager) AddPeer(peer *Peer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := peer.Address.String()
	m.peers[key] = peer

	if peer.IsSeedNode {
		m.seedPeers = append(m.seedPeers, peer)
	} else if peer.IsDarknet {
		m.darknetPeers = append(m.darknetPeers, peer)
	} else if peer.IsOpennet {
		m.opennetPeers = append(m.opennetPeers, peer)
	}

	if m.debugMode {
		log.Printf("[PEERS] Added peer: %s", peer)
	}
}

// GetPeer retrieves a peer by address
func (m *Manager) GetPeer(addr *net.UDPAddr) *Peer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := addr.String()
	return m.peers[key]
}

// GetPeerByIdentityHash retrieves a peer by identity hash
func (m *Manager) GetPeerByIdentityHash(identityHash []byte) *Peer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, peer := range m.peers {
		if peer.IdentityHash != nil && string(peer.IdentityHash) == string(identityHash) {
			return peer
		}
	}
	return nil
}

// GetSeedPeers returns all seed peers
func (m *Manager) GetSeedPeers() []*Peer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return append([]*Peer{}, m.seedPeers...)
}

// GetConnectedPeers returns all connected peers
func (m *Manager) GetConnectedPeers() []*Peer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	connected := make([]*Peer, 0)
	for _, peer := range m.peers {
		if peer.IsConnected() {
			connected = append(connected, peer)
		}
	}
	return connected
}

// GetAllPeers returns all peers
func (m *Manager) GetAllPeers() []*Peer {
	m.mu.RLock()
	defer m.mu.RUnlock()

	all := make([]*Peer, 0, len(m.peers))
	for _, peer := range m.peers {
		all = append(all, peer)
	}
	return all
}

// RemovePeer removes a peer
func (m *Manager) RemovePeer(addr *net.UDPAddr) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := addr.String()
	delete(m.peers, key)

	if m.debugMode {
		log.Printf("[PEERS] Removed peer: %s", addr)
	}
}

// Count returns the number of peers
func (m *Manager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.peers)
}

// CountConnected returns the number of connected peers
func (m *Manager) CountConnected() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, peer := range m.peers {
		if peer.IsConnected() {
			count++
		}
	}
	return count
}

// CleanupStalePeers removes peers that haven't been contacted recently
func (m *Manager) CleanupStalePeers(maxAge time.Duration) int {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	removed := 0

	for key, peer := range m.peers {
		// Don't remove seed nodes or darknet peers
		if peer.IsSeedNode || peer.IsDarknet {
			continue
		}

		// Remove if too old and not connected
		if !peer.IsConnected() && !peer.LastContact.IsZero() &&
			now.Sub(peer.LastContact) > maxAge {
			delete(m.peers, key)
			removed++
		}
	}

	if m.debugMode && removed > 0 {
		log.Printf("[PEERS] Cleaned up %d stale peers", removed)
	}

	return removed
}

// GetStats returns peer statistics
func (m *Manager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total"] = len(m.peers)
	stats["seed"] = len(m.seedPeers)
	stats["darknet"] = len(m.darknetPeers)
	stats["opennet"] = len(m.opennetPeers)
	stats["connected"] = m.CountConnected()

	return stats
}
