// GoHyphanet - NPF Packet Parsing
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package npf

import (
	"fmt"
)

// Parse parses an NPF packet from bytes
func Parse(data []byte) (*Packet, error) {
	if len(data) < 5 {
		return nil, fmt.Errorf("packet too short: %d bytes", len(data))
	}

	packet := &Packet{
		Acks:          make([]int32, 0),
		Fragments:     make([]*MessageFragment, 0),
		LossyMessages: make([][]byte, 0),
	}

	offset := 0

	// Parse sequence number (4 bytes)
	packet.SequenceNumber = int32(data[offset])<<24 |
		int32(data[offset+1])<<16 |
		int32(data[offset+2])<<8 |
		int32(data[offset+3])
	offset += 4

	// Parse acks
	numAckRanges := int(data[offset])
	offset++

	if numAckRanges > 0 {
		var err error
		offset, err = parseAcks(data, offset, numAckRanges, packet)
		if err != nil {
			return nil, fmt.Errorf("failed to parse acks: %w", err)
		}
	}

	// Parse message fragments
	prevMsgID := int32(-1)
	for offset < len(data) {
		if offset >= len(data) {
			break
		}

		flagsByte := data[offset]

		shortMessage := (flagsByte & FlagShortMessage) != 0
		isFragmented := (flagsByte & FlagFragmented) != 0
		firstFragment := (flagsByte & FlagFirstFragment) != 0

		// Check for padding or lossy messages
		if !isFragmented && !firstFragment {
			// This is either padding or lossy messages
			offset = parseLossyMessages(data, offset, packet)
			break
		}

		// Parse message ID
		var messageID int32
		fullMessageID := (flagsByte & FlagFullMessageID) != 0

		if fullMessageID {
			// Full 28-bit message ID
			if offset+4 > len(data) {
				return nil, fmt.Errorf("incomplete message ID at offset %d", offset)
			}
			messageID = int32(flagsByte&0x0F)<<24 |
				int32(data[offset+1])<<16 |
				int32(data[offset+2])<<8 |
				int32(data[offset+3])
			offset += 4
		} else {
			// Compressed message ID (delta from previous)
			if offset+2 > len(data) {
				return nil, fmt.Errorf("incomplete compressed message ID at offset %d", offset)
			}
			if prevMsgID == -1 {
				return nil, fmt.Errorf("compressed message ID without previous message ID")
			}
			delta := int32(flagsByte&0x0F)<<8 | int32(data[offset+1])
			messageID = prevMsgID + delta
			offset += 2
		}
		prevMsgID = messageID

		// Parse fragment length
		var fragmentLength int
		if shortMessage {
			if offset+1 > len(data) {
				return nil, fmt.Errorf("incomplete fragment length at offset %d", offset)
			}
			fragmentLength = int(data[offset])
			offset++
		} else {
			if offset+2 > len(data) {
				return nil, fmt.Errorf("incomplete fragment length at offset %d", offset)
			}
			fragmentLength = int(data[offset])<<8 | int(data[offset+1])
			offset += 2
		}

		// Parse message length or fragment offset (if fragmented)
		messageLength := -1
		fragmentOffset := 0

		if isFragmented {
			var value int
			if shortMessage {
				if offset+1 > len(data) {
					return nil, fmt.Errorf("incomplete fragment info at offset %d", offset)
				}
				value = int(data[offset])
				offset++
			} else {
				if offset+2 > len(data) {
					return nil, fmt.Errorf("incomplete fragment info at offset %d", offset)
				}
				value = int(data[offset])<<8 | int(data[offset+1])
				offset += 2
			}

			if firstFragment {
				messageLength = value
			} else {
				fragmentOffset = value
			}
		} else {
			messageLength = fragmentLength
		}

		// Parse fragment data
		if offset+fragmentLength > len(data) {
			return nil, fmt.Errorf("fragment data extends beyond packet: offset=%d length=%d total=%d",
				offset, fragmentLength, len(data))
		}

		fragmentData := make([]byte, fragmentLength)
		copy(fragmentData, data[offset:offset+fragmentLength])
		offset += fragmentLength

		// Create fragment
		fragment := &MessageFragment{
			ShortMessage:   shortMessage,
			IsFragmented:   isFragmented,
			FirstFragment:  firstFragment,
			MessageID:      messageID,
			FragmentLength: fragmentLength,
			MessageLength:  messageLength,
			FragmentOffset: fragmentOffset,
			Data:           fragmentData,
		}

		packet.Fragments = append(packet.Fragments, fragment)
	}

	return packet, nil
}

// parseAcks parses the ack ranges from the packet
func parseAcks(data []byte, offset int, numAckRanges int, packet *Packet) (int, error) {
	prevAck := int32(0)

	for i := 0; i < numAckRanges; i++ {
		var ack int32

		if i == 0 {
			// First ack is always full 4 bytes
			if offset+4 > len(data) {
				return offset, fmt.Errorf("incomplete first ack at offset %d", offset)
			}
			ack = int32(data[offset])<<24 |
				int32(data[offset+1])<<16 |
				int32(data[offset+2])<<8 |
				int32(data[offset+3])
			offset += 4
		} else {
			// Subsequent acks can be compressed
			if offset+1 > len(data) {
				return offset, fmt.Errorf("incomplete ack delta at offset %d", offset)
			}
			delta := int(data[offset])
			offset++

			if delta == 0 {
				// Far offset - full 4 bytes
				if offset+4 > len(data) {
					return offset, fmt.Errorf("incomplete far ack at offset %d", offset)
				}
				ack = int32(data[offset])<<24 |
					int32(data[offset+1])<<16 |
					int32(data[offset+2])<<8 |
					int32(data[offset+3])
				offset += 4
			} else {
				// Near offset - delta from previous
				ack = prevAck + int32(delta)
			}
		}

		// Parse range size
		if offset+1 > len(data) {
			return offset, fmt.Errorf("incomplete range size at offset %d", offset)
		}
		rangeSize := int(data[offset])
		offset++

		// Add all acks in this range
		for j := 0; j < rangeSize; j++ {
			packet.Acks = append(packet.Acks, ack)
			ack++
		}

		prevAck = ack - 1
	}

	return offset, nil
}

// parseLossyMessages parses lossy (per-packet) messages
func parseLossyMessages(data []byte, offset int, packet *Packet) int {
	origOffset := offset

	for offset < len(data) {
		if data[offset] != LossyMessageMarker {
			// This is padding, we're done
			return offset
		}

		offset++
		if offset >= len(data) {
			// Invalid lossy message, discard all and return original offset
			packet.LossyMessages = make([][]byte, 0)
			return origOffset
		}

		// Read length
		length := int(data[offset])
		offset++

		if length > len(data)-offset {
			// Invalid length, discard all and return original offset
			packet.LossyMessages = make([][]byte, 0)
			return origOffset
		}

		// Read message data
		msgData := make([]byte, length)
		copy(msgData, data[offset:offset+length])
		packet.LossyMessages = append(packet.LossyMessages, msgData)
		offset += length

		if offset == len(data) {
			return offset
		}
	}

	return offset
}
