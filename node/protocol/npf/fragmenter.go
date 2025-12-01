// GoHyphanet - NPF Message Fragmentation
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package npf

import (
	"fmt"
	"sync"
)

const (
	// Fragment size for splitting large messages
	DefaultMaxFragmentSize = 1024
)

// Message represents a complete message to be sent
type Message struct {
	ID       int32
	Data     []byte
	Priority int
}

// FragmentMessage fragments a message into multiple fragments
func FragmentMessage(msg *Message, maxFragmentSize int) ([]*MessageFragment, error) {
	if maxFragmentSize <= 0 {
		maxFragmentSize = DefaultMaxFragmentSize
	}

	messageLength := len(msg.Data)
	shortMessage := messageLength <= MaxFragmentLength

	// If message fits in one fragment, don't fragment
	if messageLength <= maxFragmentSize {
		return []*MessageFragment{
			{
				ShortMessage:   shortMessage,
				IsFragmented:   false,
				FirstFragment:  true,
				MessageID:      msg.ID,
				FragmentLength: messageLength,
				MessageLength:  messageLength,
				FragmentOffset: 0,
				Data:           msg.Data,
			},
		}, nil
	}

	// Fragment the message
	fragments := make([]*MessageFragment, 0)
	offset := 0

	for offset < messageLength {
		fragmentLength := maxFragmentSize
		if offset+fragmentLength > messageLength {
			fragmentLength = messageLength - offset
		}

		fragmentData := make([]byte, fragmentLength)
		copy(fragmentData, msg.Data[offset:offset+fragmentLength])

		fragment := &MessageFragment{
			ShortMessage:   shortMessage,
			IsFragmented:   true,
			FirstFragment:  offset == 0,
			MessageID:      msg.ID,
			FragmentLength: fragmentLength,
			MessageLength:  messageLength,
			FragmentOffset: offset,
			Data:           fragmentData,
		}

		fragments = append(fragments, fragment)
		offset += fragmentLength
	}

	return fragments, nil
}

// MessageReassembler reassembles fragmented messages
type MessageReassembler struct {
	buffers map[int32]*PartialMessage
	mu      sync.RWMutex
}

// PartialMessage represents a partially received message
type PartialMessage struct {
	MessageID      int32
	MessageLength  int
	ReceivedBytes  int
	Fragments      map[int][]byte // offset -> data
	Complete       bool
	CompleteData   []byte
}

// NewMessageReassembler creates a new message reassembler
func NewMessageReassembler() *MessageReassembler {
	return &MessageReassembler{
		buffers: make(map[int32]*PartialMessage),
	}
}

// AddFragment adds a fragment to the reassembler
// Returns the complete message data if the message is now complete, nil otherwise
func (mr *MessageReassembler) AddFragment(frag *MessageFragment) ([]byte, error) {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	// Handle non-fragmented messages
	if !frag.IsFragmented {
		return frag.Data, nil
	}

	// Get or create partial message
	partial, exists := mr.buffers[frag.MessageID]
	if !exists {
		if !frag.FirstFragment {
			return nil, fmt.Errorf("received non-first fragment for unknown message %d", frag.MessageID)
		}

		partial = &PartialMessage{
			MessageID:     frag.MessageID,
			MessageLength: frag.MessageLength,
			ReceivedBytes: 0,
			Fragments:     make(map[int][]byte),
			Complete:      false,
		}
		mr.buffers[frag.MessageID] = partial
	}

	// Check if fragment already received
	if _, exists := partial.Fragments[frag.FragmentOffset]; exists {
		// Duplicate fragment, ignore
		return nil, nil
	}

	// Add fragment
	partial.Fragments[frag.FragmentOffset] = frag.Data
	partial.ReceivedBytes += len(frag.Data)

	// Check if message is complete
	if partial.ReceivedBytes == partial.MessageLength {
		// Reassemble message
		completeData := make([]byte, partial.MessageLength)
		for offset, data := range partial.Fragments {
			copy(completeData[offset:], data)
		}

		partial.Complete = true
		partial.CompleteData = completeData

		// Remove from buffers
		delete(mr.buffers, frag.MessageID)

		return completeData, nil
	}

	return nil, nil
}

// GetPartialMessage returns information about a partially received message
func (mr *MessageReassembler) GetPartialMessage(messageID int32) (*PartialMessage, bool) {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	partial, exists := mr.buffers[messageID]
	return partial, exists
}

// CleanupOldMessages removes partial messages that are too old
// This should be called periodically to prevent memory leaks
func (mr *MessageReassembler) CleanupOldMessages(maxAge int) int {
	mr.mu.Lock()
	defer mr.mu.Unlock()

	// For now, just clear all partial messages
	// In a full implementation, we'd track timestamps and only remove old ones
	count := len(mr.buffers)
	mr.buffers = make(map[int32]*PartialMessage)
	return count
}

// GetStats returns statistics about the reassembler
func (mr *MessageReassembler) GetStats() map[string]interface{} {
	mr.mu.RLock()
	defer mr.mu.RUnlock()

	totalFragments := 0
	totalBytes := 0

	for _, partial := range mr.buffers {
		totalFragments += len(partial.Fragments)
		totalBytes += partial.ReceivedBytes
	}

	return map[string]interface{}{
		"partial_messages": len(mr.buffers),
		"total_fragments":  totalFragments,
		"total_bytes":      totalBytes,
	}
}
