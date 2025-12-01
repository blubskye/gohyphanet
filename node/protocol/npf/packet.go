// GoHyphanet - NewPacketFormat Implementation
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package npf

import (
	"crypto/rand"
	"fmt"
	"sort"
)

const (
	// Packet constants
	MaxPacketSize      = 1280 // MTU-safe packet size
	HMACLength         = 10   // HMAC length in NPF
	MaxMessageIDDelta  = 4096 // Max delta for compressed message IDs
	MaxFragmentLength  = 255  // Max length for short messages
	MaxAckRanges       = 254  // Max number of ack ranges
	MaxAckRangeDelta   = 254  // Max delta between acks

	// Flag bits for message header
	FlagShortMessage  = 0x80 // Message length < 256 bytes
	FlagFragmented    = 0x40 // Message is fragmented
	FlagFirstFragment = 0x20 // This is the first fragment
	FlagFullMessageID = 0x10 // Full 28-bit message ID follows

	// Lossy message marker
	LossyMessageMarker = 0x1F
)

// Packet represents an NPF packet
type Packet struct {
	SequenceNumber int32
	Acks           []int32
	Fragments      []*MessageFragment
	LossyMessages  [][]byte
}

// MessageFragment represents a message fragment within a packet
type MessageFragment struct {
	ShortMessage  bool
	IsFragmented  bool
	FirstFragment bool
	MessageID     int32
	FragmentLength int
	MessageLength  int  // Total message length (only for first fragment)
	FragmentOffset int  // Offset of this fragment (only for non-first fragments)
	Data          []byte
}

// NewPacket creates a new empty packet
func NewPacket(sequenceNumber int32) *Packet {
	return &Packet{
		SequenceNumber: sequenceNumber,
		Acks:           make([]int32, 0),
		Fragments:      make([]*MessageFragment, 0),
		LossyMessages:  make([][]byte, 0),
	}
}

// AddAck adds an acknowledgment to the packet
func (p *Packet) AddAck(ack int32) bool {
	// Check if already present
	for _, existing := range p.Acks {
		if existing == ack {
			return true
		}
	}

	p.Acks = append(p.Acks, ack)
	sort.Slice(p.Acks, func(i, j int) bool {
		return p.Acks[i] < p.Acks[j]
	})

	return true
}

// AddFragment adds a message fragment to the packet
func (p *Packet) AddFragment(frag *MessageFragment) {
	p.Fragments = append(p.Fragments, frag)

	// Sort fragments by message ID
	sort.Slice(p.Fragments, func(i, j int) bool {
		return p.Fragments[i].MessageID < p.Fragments[j].MessageID
	})
}

// AddLossyMessage adds a lossy (per-packet) message
func (p *Packet) AddLossyMessage(data []byte) error {
	if len(data) > 255 {
		return fmt.Errorf("lossy message too large: %d bytes", len(data))
	}
	p.LossyMessages = append(p.LossyMessages, data)
	return nil
}

// EstimateSize estimates the serialized size of the packet
func (p *Packet) EstimateSize() int {
	size := 4 // Sequence number
	size += 1 // Ack count

	// Estimate ack size
	if len(p.Acks) > 0 {
		size += 5 // First ack (4 bytes + range size)
		prevAck := p.Acks[0]
		for i := 1; i < len(p.Acks); i++ {
			delta := p.Acks[i] - prevAck
			if delta >= MaxAckRangeDelta {
				size += 6 // Far offset (1 byte marker + 4 bytes + range size)
			} else {
				size += 2 // Near offset (1 byte delta + range size)
			}
			prevAck = p.Acks[i]
		}
	}

	// Estimate fragment size
	prevMsgID := int32(-1)
	for _, frag := range p.Fragments {
		// Message ID
		if prevMsgID == -1 || (frag.MessageID-prevMsgID >= MaxMessageIDDelta) {
			size += 4 // Full message ID
		} else {
			size += 2 // Compressed message ID
		}
		prevMsgID = frag.MessageID

		// Fragment header
		if frag.ShortMessage {
			size += 1 // Fragment length
		} else {
			size += 2 // Fragment length
		}

		if frag.IsFragmented {
			if frag.ShortMessage {
				size += 1 // Message length or fragment offset
			} else {
				size += 2 // Message length or fragment offset
			}
		}

		// Fragment data
		size += len(frag.Data)
	}

	// Lossy messages
	for _, msg := range p.LossyMessages {
		size += 2 + len(msg) // Marker + length + data
	}

	return size
}

// Serialize serializes the packet to bytes
func (p *Packet) Serialize(maxSize int) ([]byte, error) {
	buf := make([]byte, maxSize)
	offset := 0

	// Sequence number (4 bytes)
	buf[offset] = byte(p.SequenceNumber >> 24)
	buf[offset+1] = byte(p.SequenceNumber >> 16)
	buf[offset+2] = byte(p.SequenceNumber >> 8)
	buf[offset+3] = byte(p.SequenceNumber)
	offset += 4

	// Acks
	ackRanges, err := p.compressAcks()
	if err != nil {
		return nil, err
	}

	buf[offset] = byte(len(ackRanges))
	offset++

	for i, ackRange := range ackRanges {
		// Write ack start
		if i == 0 {
			// First ack is always full
			buf[offset] = byte(ackRange.Start >> 24)
			buf[offset+1] = byte(ackRange.Start >> 16)
			buf[offset+2] = byte(ackRange.Start >> 8)
			buf[offset+3] = byte(ackRange.Start)
			offset += 4
		} else {
			// Subsequent acks can be compressed
			delta := ackRange.Start - ackRanges[i-1].End
			if delta >= MaxAckRangeDelta {
				// Far offset
				buf[offset] = 0
				offset++
				buf[offset] = byte(ackRange.Start >> 24)
				buf[offset+1] = byte(ackRange.Start >> 16)
				buf[offset+2] = byte(ackRange.Start >> 8)
				buf[offset+3] = byte(ackRange.Start)
				offset += 4
			} else {
				// Near offset
				buf[offset] = byte(delta)
				offset++
			}
		}

		// Write range size
		buf[offset] = byte(ackRange.Size)
		offset++
	}

	// Fragments
	prevMsgID := int32(-1)
	for _, frag := range p.Fragments {
		flagsByte := byte(0)

		if frag.ShortMessage {
			flagsByte |= FlagShortMessage
		}
		if frag.IsFragmented {
			flagsByte |= FlagFragmented
		}
		if frag.FirstFragment {
			flagsByte |= FlagFirstFragment
		}

		// Write message ID
		if prevMsgID == -1 || (frag.MessageID-prevMsgID >= MaxMessageIDDelta) {
			// Full message ID
			flagsByte |= FlagFullMessageID
			buf[offset] = flagsByte | byte((frag.MessageID>>24)&0x0F)
			buf[offset+1] = byte(frag.MessageID >> 16)
			buf[offset+2] = byte(frag.MessageID >> 8)
			buf[offset+3] = byte(frag.MessageID)
			offset += 4
		} else {
			// Compressed message ID (delta from previous)
			delta := frag.MessageID - prevMsgID
			buf[offset] = flagsByte | byte((delta>>8)&0x0F)
			buf[offset+1] = byte(delta)
			offset += 2
		}
		prevMsgID = frag.MessageID

		// Write fragment length
		if frag.ShortMessage {
			buf[offset] = byte(frag.FragmentLength)
			offset++
		} else {
			buf[offset] = byte(frag.FragmentLength >> 8)
			buf[offset+1] = byte(frag.FragmentLength)
			offset += 2
		}

		// Write message length or fragment offset (if fragmented)
		if frag.IsFragmented {
			value := frag.MessageLength
			if !frag.FirstFragment {
				value = frag.FragmentOffset
			}

			if frag.ShortMessage {
				buf[offset] = byte(value)
				offset++
			} else {
				buf[offset] = byte(value >> 8)
				buf[offset+1] = byte(value)
				offset += 2
			}
		}

		// Write fragment data
		copy(buf[offset:], frag.Data)
		offset += len(frag.Data)
	}

	// Lossy messages
	for _, msg := range p.LossyMessages {
		buf[offset] = LossyMessageMarker
		offset++
		buf[offset] = byte(len(msg))
		offset++
		copy(buf[offset:], msg)
		offset += len(msg)
	}

	// Add padding if needed
	if offset < maxSize {
		rand.Read(buf[offset:maxSize])
		// Make sure padding doesn't look like a valid fragment or lossy message
		buf[offset] = buf[offset] & 0x9F
		if buf[offset] == LossyMessageMarker {
			buf[offset] = 0x9F
		}
	}

	return buf[:maxSize], nil
}

// AckRange represents a range of consecutive acknowledgments
type AckRange struct {
	Start int32
	End   int32
	Size  int32
}

// compressAcks compresses acks into ranges
func (p *Packet) compressAcks() ([]*AckRange, error) {
	if len(p.Acks) == 0 {
		return []*AckRange{}, nil
	}

	ranges := make([]*AckRange, 0)
	start := p.Acks[0]
	end := start

	for i := 1; i < len(p.Acks); i++ {
		if p.Acks[i] == end+1 && (end-start) < MaxAckRangeDelta {
			// Extend current range
			end = p.Acks[i]
		} else {
			// Start new range
			ranges = append(ranges, &AckRange{
				Start: start,
				End:   end,
				Size:  end - start + 1,
			})
			start = p.Acks[i]
			end = start
		}
	}

	// Add final range
	ranges = append(ranges, &AckRange{
		Start: start,
		End:   end,
		Size:  end - start + 1,
	})

	if len(ranges) > MaxAckRanges {
		return nil, fmt.Errorf("too many ack ranges: %d", len(ranges))
	}

	return ranges, nil
}

// String returns a string representation of the packet
func (p *Packet) String() string {
	return fmt.Sprintf("Packet %d: %d acks, %d fragments, %d lossy messages",
		p.SequenceNumber, len(p.Acks), len(p.Fragments), len(p.LossyMessages))
}
