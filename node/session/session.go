// GoHyphanet - Session Management
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package session

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/node/peer"
)

const (
	// Session constants
	MaxSequenceNumber = 0x7FFFFFFF // 2^31 - 1
	RekeyThreshold    = 0x40000000 // Rekey after 1 billion packets
	RekeyInterval     = 1 * time.Hour
	HMACLength        = 32 // SHA256 HMAC length
	IVLength          = 16 // AES block size
)

// Session represents an active encrypted session with a peer
type Session struct {
	// Peer reference
	Peer *peer.Peer

	// Session keys
	OutgoingKey []byte // Key for encrypting outgoing packets
	IncomingKey []byte // Key for decrypting incoming packets

	// Sequence numbers for replay protection
	OutgoingSeqNum uint32
	IncomingSeqNum uint32
	seenSeqNums    map[uint32]bool // Track recent sequence numbers

	// Rekeying
	LastRekey     time.Time
	PacketsSent   uint64
	PacketsRecvd  uint64

	// Ciphers (cached for performance)
	outgoingCipher cipher.Block
	incomingCipher cipher.Block

	// Lifecycle
	EstablishedAt time.Time
	LastActivity  time.Time

	mu sync.RWMutex
}

// NewSession creates a new session with the given keys
func NewSession(p *peer.Peer, outgoingKey, incomingKey []byte) (*Session, error) {
	if len(outgoingKey) != 32 || len(incomingKey) != 32 {
		return nil, fmt.Errorf("invalid key length")
	}

	// Create AES ciphers
	outCipher, err := aes.NewCipher(outgoingKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create outgoing cipher: %w", err)
	}

	inCipher, err := aes.NewCipher(incomingKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create incoming cipher: %w", err)
	}

	now := time.Now()
	return &Session{
		Peer:           p,
		OutgoingKey:    outgoingKey,
		IncomingKey:    incomingKey,
		OutgoingSeqNum: 1,
		IncomingSeqNum: 0,
		seenSeqNums:    make(map[uint32]bool),
		LastRekey:      now,
		outgoingCipher: outCipher,
		incomingCipher: inCipher,
		EstablishedAt:  now,
		LastActivity:   now,
	}, nil
}

// EncryptPacket encrypts a packet with authentication
func (s *Session) EncryptPacket(plaintext []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if we need to rekey
	if s.ShouldRekeyLocked() {
		return nil, fmt.Errorf("session needs rekeying")
	}

	// Generate IV
	iv := make([]byte, IVLength)
	if _, err := rand.Read(iv); err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	// Encrypt using CTR mode
	ctr := cipher.NewCTR(s.outgoingCipher, iv)
	ciphertext := make([]byte, len(plaintext))
	ctr.XORKeyStream(ciphertext, plaintext)

	// Build packet: IV + ciphertext + HMAC
	// HMAC covers: seqNum + IV + ciphertext
	h := hmac.New(sha256.New, s.OutgoingKey)
	h.Write(uint32ToBytes(s.OutgoingSeqNum))
	h.Write(iv)
	h.Write(ciphertext)
	mac := h.Sum(nil)

	// Assemble: seqNum(4) + IV(16) + ciphertext + HMAC(32)
	packet := make([]byte, 4+IVLength+len(ciphertext)+HMACLength)
	copy(packet[0:4], uint32ToBytes(s.OutgoingSeqNum))
	copy(packet[4:4+IVLength], iv)
	copy(packet[4+IVLength:4+IVLength+len(ciphertext)], ciphertext)
	copy(packet[4+IVLength+len(ciphertext):], mac)

	// Update state
	s.OutgoingSeqNum++
	s.PacketsSent++
	s.LastActivity = time.Now()

	return packet, nil
}

// DecryptPacket decrypts and authenticates a packet
func (s *Session) DecryptPacket(packet []byte) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Minimum size: seqNum(4) + IV(16) + HMAC(32) = 52 bytes
	if len(packet) < 52 {
		return nil, fmt.Errorf("packet too short")
	}

	// Extract components
	seqNum := bytesToUint32(packet[0:4])
	iv := packet[4 : 4+IVLength]
	macStart := len(packet) - HMACLength
	ciphertext := packet[4+IVLength : macStart]
	receivedMAC := packet[macStart:]

	// Verify HMAC
	h := hmac.New(sha256.New, s.IncomingKey)
	h.Write(packet[0:4]) // seqNum
	h.Write(iv)
	h.Write(ciphertext)
	expectedMAC := h.Sum(nil)

	if !hmac.Equal(receivedMAC, expectedMAC) {
		return nil, fmt.Errorf("HMAC verification failed")
	}

	// Check sequence number for replay attacks
	if !s.isValidSequenceNumber(seqNum) {
		return nil, fmt.Errorf("invalid sequence number: %d", seqNum)
	}

	// Decrypt using CTR mode
	ctr := cipher.NewCTR(s.incomingCipher, iv)
	plaintext := make([]byte, len(ciphertext))
	ctr.XORKeyStream(plaintext, ciphertext)

	// Update state
	s.IncomingSeqNum = seqNum
	s.seenSeqNums[seqNum] = true
	s.PacketsRecvd++
	s.LastActivity = time.Now()

	// Cleanup old sequence numbers (keep last 1000)
	if len(s.seenSeqNums) > 1000 {
		s.cleanupSeqNums()
	}

	return plaintext, nil
}

// isValidSequenceNumber checks if a sequence number is valid (not replayed)
func (s *Session) isValidSequenceNumber(seqNum uint32) bool {
	// Must be greater than last seen
	if seqNum <= s.IncomingSeqNum {
		// Check if we've seen this number recently (could be out of order)
		if s.seenSeqNums[seqNum] {
			return false // Replay attack
		}
		// Allow some out-of-order packets (within 100 of current)
		if s.IncomingSeqNum-seqNum > 100 {
			return false // Too old
		}
	}

	// Check for sequence number wraparound
	if seqNum > MaxSequenceNumber {
		return false
	}

	return true
}

// cleanupSeqNums removes old sequence numbers from the tracking map
func (s *Session) cleanupSeqNums() {
	// Keep only sequence numbers within 1000 of current
	minSeqNum := s.IncomingSeqNum - 1000
	for seqNum := range s.seenSeqNums {
		if seqNum < minSeqNum {
			delete(s.seenSeqNums, seqNum)
		}
	}
}

// ShouldRekey returns true if the session should be rekeyed
func (s *Session) ShouldRekey() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ShouldRekeyLocked()
}

// ShouldRekeyLocked checks if rekeying is needed (must hold lock)
func (s *Session) ShouldRekeyLocked() bool {
	// Rekey if we've sent too many packets
	if s.OutgoingSeqNum > RekeyThreshold {
		return true
	}

	// Rekey if it's been too long
	if time.Since(s.LastRekey) > RekeyInterval {
		return true
	}

	return false
}

// UpdateKeys updates the session keys and resets counters
func (s *Session) UpdateKeys(outgoingKey, incomingKey []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(outgoingKey) != 32 || len(incomingKey) != 32 {
		return fmt.Errorf("invalid key length")
	}

	// Create new ciphers
	outCipher, err := aes.NewCipher(outgoingKey)
	if err != nil {
		return fmt.Errorf("failed to create outgoing cipher: %w", err)
	}

	inCipher, err := aes.NewCipher(incomingKey)
	if err != nil {
		return fmt.Errorf("failed to create incoming cipher: %w", err)
	}

	// Update keys and ciphers
	s.OutgoingKey = outgoingKey
	s.IncomingKey = incomingKey
	s.outgoingCipher = outCipher
	s.incomingCipher = inCipher

	// Reset sequence numbers and tracking
	s.OutgoingSeqNum = 1
	s.IncomingSeqNum = 0
	s.seenSeqNums = make(map[uint32]bool)
	s.LastRekey = time.Now()

	return nil
}

// GetStats returns session statistics
func (s *Session) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"peer":           s.Peer.String(),
		"established_at": s.EstablishedAt,
		"last_activity":  s.LastActivity,
		"last_rekey":     s.LastRekey,
		"packets_sent":   s.PacketsSent,
		"packets_recvd":  s.PacketsRecvd,
		"seq_num_out":    s.OutgoingSeqNum,
		"seq_num_in":     s.IncomingSeqNum,
		"age":            time.Since(s.EstablishedAt).String(),
	}
}

// IsActive returns true if the session has recent activity
func (s *Session) IsActive(timeout time.Duration) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.LastActivity) < timeout
}

// Helper functions

func uint32ToBytes(n uint32) []byte {
	return []byte{
		byte(n >> 24),
		byte(n >> 16),
		byte(n >> 8),
		byte(n),
	}
}

func bytesToUint32(b []byte) uint32 {
	return uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
}
