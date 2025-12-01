// GoHyphanet - Session Tracker
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package session

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/node/peer"
)

const (
	// Session timeouts
	SessionTimeout        = 30 * time.Minute
	SessionCleanupInterval = 5 * time.Minute
)

// Tracker manages all active sessions
type Tracker struct {
	sessions    map[string]*Session // key: peer address string
	byPeer      map[*peer.Peer]*Session
	mu          sync.RWMutex
	debugMode   bool
	stopCleanup chan struct{}
}

// NewTracker creates a new session tracker
func NewTracker(debugMode bool) *Tracker {
	t := &Tracker{
		sessions:    make(map[string]*Session),
		byPeer:      make(map[*peer.Peer]*Session),
		debugMode:   debugMode,
		stopCleanup: make(chan struct{}),
	}

	// Start cleanup goroutine
	go t.cleanupLoop()

	return t
}

// CreateSession creates a new session for a peer
func (t *Tracker) CreateSession(p *peer.Peer, outgoingKey, incomingKey []byte) (*Session, error) {
	if p == nil || p.Address == nil {
		return nil, fmt.Errorf("invalid peer")
	}

	session, err := NewSession(p, outgoingKey, incomingKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	key := p.Address.String()

	// Remove old session if exists
	if oldSession, exists := t.sessions[key]; exists {
		if t.debugMode {
			log.Printf("[SESSION] Replacing existing session for %s", key)
		}
		delete(t.byPeer, oldSession.Peer)
	}

	t.sessions[key] = session
	t.byPeer[p] = session

	if t.debugMode {
		log.Printf("[SESSION] Created session for %s", key)
	}

	return session, nil
}

// GetSession retrieves a session by peer address
func (t *Tracker) GetSession(addr *net.UDPAddr) *Session {
	if addr == nil {
		return nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	key := addr.String()
	return t.sessions[key]
}

// GetSessionByPeer retrieves a session by peer object
func (t *Tracker) GetSessionByPeer(p *peer.Peer) *Session {
	if p == nil {
		return nil
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	return t.byPeer[p]
}

// HasSession returns true if a session exists for the address
func (t *Tracker) HasSession(addr *net.UDPAddr) bool {
	return t.GetSession(addr) != nil
}

// RemoveSession removes a session
func (t *Tracker) RemoveSession(addr *net.UDPAddr) {
	if addr == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	key := addr.String()
	if session, exists := t.sessions[key]; exists {
		delete(t.sessions, key)
		delete(t.byPeer, session.Peer)

		if t.debugMode {
			log.Printf("[SESSION] Removed session for %s", key)
		}
	}
}

// RemoveSessionByPeer removes a session by peer object
func (t *Tracker) RemoveSessionByPeer(p *peer.Peer) {
	if p == nil || p.Address == nil {
		return
	}
	t.RemoveSession(p.Address)
}

// GetAllSessions returns all active sessions
func (t *Tracker) GetAllSessions() []*Session {
	t.mu.RLock()
	defer t.mu.RUnlock()

	sessions := make([]*Session, 0, len(t.sessions))
	for _, session := range t.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

// Count returns the number of active sessions
func (t *Tracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.sessions)
}

// CleanupStale removes inactive sessions
func (t *Tracker) CleanupStale() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	removed := 0
	for key, session := range t.sessions {
		if !session.IsActive(SessionTimeout) {
			delete(t.sessions, key)
			delete(t.byPeer, session.Peer)
			removed++

			if t.debugMode {
				log.Printf("[SESSION] Cleaned up stale session for %s (last active: %v ago)",
					key, time.Since(session.LastActivity))
			}
		}
	}

	return removed
}

// CleanupSessionsNeedingRekey identifies sessions that need rekeying
func (t *Tracker) CleanupSessionsNeedingRekey() []*Session {
	t.mu.RLock()
	defer t.mu.RUnlock()

	needRekey := make([]*Session, 0)
	for _, session := range t.sessions {
		if session.ShouldRekey() {
			needRekey = append(needRekey, session)
		}
	}

	return needRekey
}

// cleanupLoop periodically cleans up stale sessions
func (t *Tracker) cleanupLoop() {
	ticker := time.NewTicker(SessionCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			removed := t.CleanupStale()
			if t.debugMode && removed > 0 {
				log.Printf("[SESSION] Cleanup removed %d stale sessions", removed)
			}

			// Check for sessions needing rekey
			needRekey := t.CleanupSessionsNeedingRekey()
			if t.debugMode && len(needRekey) > 0 {
				log.Printf("[SESSION] %d sessions need rekeying", len(needRekey))
			}

		case <-t.stopCleanup:
			return
		}
	}
}

// Stop stops the session tracker
func (t *Tracker) Stop() {
	close(t.stopCleanup)
}

// GetStats returns session tracker statistics
func (t *Tracker) GetStats() map[string]interface{} {
	t.mu.RLock()
	defer t.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["total_sessions"] = len(t.sessions)

	// Count sessions needing rekey
	needRekey := 0
	totalPacketsSent := uint64(0)
	totalPacketsRecvd := uint64(0)

	for _, session := range t.sessions {
		if session.ShouldRekey() {
			needRekey++
		}
		sessionStats := session.GetStats()
		totalPacketsSent += sessionStats["packets_sent"].(uint64)
		totalPacketsRecvd += sessionStats["packets_recvd"].(uint64)
	}

	stats["need_rekey"] = needRekey
	stats["total_packets_sent"] = totalPacketsSent
	stats["total_packets_recvd"] = totalPacketsRecvd

	return stats
}

// EncryptForPeer encrypts a packet for a specific peer
func (t *Tracker) EncryptForPeer(addr *net.UDPAddr, plaintext []byte) ([]byte, error) {
	session := t.GetSession(addr)
	if session == nil {
		return nil, fmt.Errorf("no session for peer %s", addr)
	}

	return session.EncryptPacket(plaintext)
}

// DecryptFromPeer decrypts a packet from a specific peer
func (t *Tracker) DecryptFromPeer(addr *net.UDPAddr, packet []byte) ([]byte, error) {
	session := t.GetSession(addr)
	if session == nil {
		return nil, fmt.Errorf("no session for peer %s", addr)
	}

	return session.DecryptPacket(packet)
}
