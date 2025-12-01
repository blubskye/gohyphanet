// GoHyphanet - NPF Connection Manager
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package npf

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

// Connection represents an NPF connection to a peer
type Connection struct {
	// Peer information
	PeerAddr *net.UDPAddr

	// Sequence numbers
	nextSeqNum     int32
	lastRecvSeqNum int32
	seqNumMu       sync.Mutex

	// Message tracking
	nextMessageID  int32
	messageIDMu    sync.Mutex

	// ACKs to send
	pendingAcks    []int32
	pendingAcksMu  sync.RWMutex

	// Outgoing message queue (by priority)
	outgoingQueues [NumPriorities][]*NPFMessage
	queueMu        sync.RWMutex

	// Message reassembly
	reassembler *MessageReassembler

	// Statistics
	packetsSent     uint64
	packetsReceived uint64
	messagesSent    uint64
	messagesReceived uint64
	statsMu         sync.RWMutex

	// Configuration
	debugMode bool
}

// NewConnection creates a new NPF connection
func NewConnection(peerAddr *net.UDPAddr, debugMode bool) *Connection {
	conn := &Connection{
		PeerAddr:       peerAddr,
		nextSeqNum:     1,
		lastRecvSeqNum: 0,
		nextMessageID:  1,
		pendingAcks:    make([]int32, 0),
		reassembler:    NewMessageReassembler(),
		debugMode:      debugMode,
	}

	// Initialize outgoing queues
	for i := 0; i < NumPriorities; i++ {
		conn.outgoingQueues[i] = make([]*NPFMessage, 0)
	}

	return conn
}

// Send queues a message for sending
func (c *Connection) Send(msg *NPFMessage) error {
	if msg.Priority < 0 || msg.Priority >= NumPriorities {
		return fmt.Errorf("invalid priority: %d", msg.Priority)
	}

	c.queueMu.Lock()
	c.outgoingQueues[msg.Priority] = append(c.outgoingQueues[msg.Priority], msg)
	c.queueMu.Unlock()

	if c.debugMode {
		log.Printf("[NPF] Queued message type %d with priority %d for %s",
			msg.Type, msg.Priority, c.PeerAddr)
	}

	return nil
}

// BuildOutgoingPacket builds a packet with queued messages
func (c *Connection) BuildOutgoingPacket(maxSize int) (*Packet, error) {
	c.seqNumMu.Lock()
	seqNum := c.nextSeqNum
	c.nextSeqNum++
	c.seqNumMu.Unlock()

	packet := NewPacket(seqNum)

	// Add pending ACKs
	c.pendingAcksMu.Lock()
	for _, ack := range c.pendingAcks {
		packet.AddAck(ack)
	}
	c.pendingAcks = make([]int32, 0) // Clear pending ACKs
	c.pendingAcksMu.Unlock()

	// Add messages from queues (highest priority first)
	c.queueMu.Lock()
	defer c.queueMu.Unlock()

	for priority := 0; priority < NumPriorities; priority++ {
		queue := c.outgoingQueues[priority]
		remaining := make([]*NPFMessage, 0)

		for _, msg := range queue {
			// Serialize message
			msgData, err := msg.Serialize()
			if err != nil {
				if c.debugMode {
					log.Printf("[NPF] Failed to serialize message: %v", err)
				}
				continue
			}

			// Assign message ID
			c.messageIDMu.Lock()
			msgID := c.nextMessageID
			c.nextMessageID++
			c.messageIDMu.Unlock()

			// Fragment message if necessary
			message := &Message{
				ID:       msgID,
				Data:     msgData,
				Priority: priority,
			}

			fragments, err := FragmentMessage(message, 1000) // Max 1KB fragments
			if err != nil {
				if c.debugMode {
					log.Printf("[NPF] Failed to fragment message: %v", err)
				}
				remaining = append(remaining, msg)
				continue
			}

			// Try to add fragments to packet
			added := false
			for _, frag := range fragments {
				packet.AddFragment(frag)

				// Check if packet is getting too large
				if packet.EstimateSize() > maxSize-200 { // Leave room for encryption overhead
					// Packet full, save remaining messages for next packet
					remaining = append(remaining, msg)
					break
				}
				added = true
			}

			if !added {
				remaining = append(remaining, msg)
			} else {
				c.statsMu.Lock()
				c.messagesSent++
				c.statsMu.Unlock()
			}
		}

		c.outgoingQueues[priority] = remaining
	}

	c.statsMu.Lock()
	c.packetsSent++
	c.statsMu.Unlock()

	return packet, nil
}

// ProcessIncomingPacket processes a received NPF packet
func (c *Connection) ProcessIncomingPacket(packet *Packet) ([]*NPFMessage, error) {
	c.statsMu.Lock()
	c.packetsReceived++
	c.statsMu.Unlock()

	// Update last received sequence number
	c.seqNumMu.Lock()
	c.lastRecvSeqNum = packet.SequenceNumber
	c.seqNumMu.Unlock()

	// Queue ACK for this packet
	c.AddPendingAck(packet.SequenceNumber)

	// Process ACKs (for our sent packets)
	// TODO: Track sent packets and handle ACKs

	// Process message fragments
	messages := make([]*NPFMessage, 0)

	for _, frag := range packet.Fragments {
		// Try to reassemble
		completeData, err := c.reassembler.AddFragment(frag)
		if err != nil {
			if c.debugMode {
				log.Printf("[NPF] Error reassembling fragment: %v", err)
			}
			continue
		}

		if completeData != nil {
			// We have a complete message
			msg, err := ParseMessage(completeData)
			if err != nil {
				if c.debugMode {
					log.Printf("[NPF] Failed to parse message: %v", err)
				}
				continue
			}

			messages = append(messages, msg)

			c.statsMu.Lock()
			c.messagesReceived++
			c.statsMu.Unlock()

			if c.debugMode {
				log.Printf("[NPF] Received complete message type %d from %s",
					msg.Type, c.PeerAddr)
			}
		}
	}

	return messages, nil
}

// AddPendingAck adds a sequence number to acknowledge
func (c *Connection) AddPendingAck(seqNum int32) {
	c.pendingAcksMu.Lock()
	defer c.pendingAcksMu.Unlock()

	// Check if already pending
	for _, ack := range c.pendingAcks {
		if ack == seqNum {
			return
		}
	}

	c.pendingAcks = append(c.pendingAcks, seqNum)
}

// HasQueuedMessages returns true if there are queued messages
func (c *Connection) HasQueuedMessages() bool {
	c.queueMu.RLock()
	defer c.queueMu.RUnlock()

	for _, queue := range c.outgoingQueues {
		if len(queue) > 0 {
			return true
		}
	}

	return false
}

// HasPendingAcks returns true if there are pending ACKs
func (c *Connection) HasPendingAcks() bool {
	c.pendingAcksMu.RLock()
	defer c.pendingAcksMu.RUnlock()

	return len(c.pendingAcks) > 0
}

// GetStats returns connection statistics
func (c *Connection) GetStats() map[string]interface{} {
	c.statsMu.RLock()
	defer c.statsMu.RUnlock()

	c.queueMu.RLock()
	queuedMsgs := 0
	for _, queue := range c.outgoingQueues {
		queuedMsgs += len(queue)
	}
	c.queueMu.RUnlock()

	c.pendingAcksMu.RLock()
	pendingAcks := len(c.pendingAcks)
	c.pendingAcksMu.RUnlock()

	reassemblerStats := c.reassembler.GetStats()

	return map[string]interface{}{
		"peer":              c.PeerAddr.String(),
		"packets_sent":      c.packetsSent,
		"packets_received":  c.packetsReceived,
		"messages_sent":     c.messagesSent,
		"messages_received": c.messagesReceived,
		"queued_messages":   queuedMsgs,
		"pending_acks":      pendingAcks,
		"reassembler":       reassemblerStats,
	}
}

// ConnectionManager manages NPF connections to multiple peers
type ConnectionManager struct {
	connections map[string]*Connection // key: peer address
	dispatcher  *Dispatcher
	mu          sync.RWMutex
	debugMode   bool
}

// NewConnectionManager creates a new connection manager
func NewConnectionManager(debugMode bool) *ConnectionManager {
	return &ConnectionManager{
		connections: make(map[string]*Connection),
		dispatcher:  NewDispatcher(debugMode),
		debugMode:   debugMode,
	}
}

// GetOrCreateConnection gets or creates a connection for a peer
func (cm *ConnectionManager) GetOrCreateConnection(peerAddr *net.UDPAddr) *Connection {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	key := peerAddr.String()
	conn, exists := cm.connections[key]
	if !exists {
		conn = NewConnection(peerAddr, cm.debugMode)
		cm.connections[key] = conn

		if cm.debugMode {
			log.Printf("[NPF] Created new connection for %s", peerAddr)
		}
	}

	return conn
}

// GetConnection gets a connection for a peer
func (cm *ConnectionManager) GetConnection(peerAddr *net.UDPAddr) *Connection {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	key := peerAddr.String()
	return cm.connections[key]
}

// Send sends a message to a peer
func (cm *ConnectionManager) Send(peerAddr *net.UDPAddr, msg *NPFMessage) error {
	conn := cm.GetOrCreateConnection(peerAddr)
	return conn.Send(msg)
}

// ProcessPacket processes an incoming packet and dispatches messages
func (cm *ConnectionManager) ProcessPacket(packet *Packet, from *net.UDPAddr) error {
	conn := cm.GetOrCreateConnection(from)

	messages, err := conn.ProcessIncomingPacket(packet)
	if err != nil {
		return err
	}

	// Dispatch messages to handlers
	for _, msg := range messages {
		if err := cm.dispatcher.Dispatch(msg, from); err != nil {
			if cm.debugMode {
				log.Printf("[NPF] Dispatch error: %v", err)
			}
		}
	}

	return nil
}

// Register registers a message handler
func (cm *ConnectionManager) Register(msgType MessageType, handler MessageHandler) {
	cm.dispatcher.Register(msgType, handler)
}

// GetStats returns statistics for all connections
func (cm *ConnectionManager) GetStats() map[string]interface{} {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	stats := make(map[string]interface{})
	stats["num_connections"] = len(cm.connections)
	stats["dispatcher"] = cm.dispatcher.GetStats()

	connStats := make([]interface{}, 0, len(cm.connections))
	for _, conn := range cm.connections {
		connStats = append(connStats, conn.GetStats())
	}
	stats["connections"] = connStats

	return stats
}

// SendKeepAlives sends void messages to all connections that need keepalives
func (cm *ConnectionManager) SendKeepAlives(interval time.Duration) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	for _, conn := range cm.connections {
		// Send void message if no recent activity
		if !conn.HasQueuedMessages() && !conn.HasPendingAcks() {
			voidMsg := CreateVoidMessage()
			conn.Send(voidMsg)
		}
	}
}
