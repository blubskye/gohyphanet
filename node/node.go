// GoHyphanet - Hyphanet Node Implementation
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package node

import (
	"crypto/rand"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/node/crypto"
	"github.com/blubskye/gohyphanet/node/peer"
	"github.com/blubskye/gohyphanet/node/protocol"
	"github.com/blubskye/gohyphanet/node/protocol/npf"
	"github.com/blubskye/gohyphanet/node/session"
	"github.com/blubskye/gohyphanet/node/transport"
)

// Node represents a Hyphanet node
type Node struct {
	identity      *crypto.NodeIdentity
	transport     *transport.UDPTransport
	transientKey  []byte // Key for HMAC in JFK
	ecdhContexts  []*crypto.ECDHContext
	port          int
	debugMode     bool

	// Peer and session management
	PeerManager    *peer.Manager
	SessionTracker *session.Tracker

	// NPF messaging
	NPFConnManager *npf.ConnectionManager

	// Handshake tracking
	activeHandshakes map[string]*protocol.JFKContext // key: peer address
	handshakeMu      sync.RWMutex

	// Event loop
	stopChan       chan struct{}
	packetSendTick *time.Ticker
	running        bool
	runMu          sync.Mutex
}

// Config holds node configuration
type Config struct {
	Port      int
	DebugMode bool
}

// NewNode creates a new Hyphanet node
func NewNode(config *Config) (*Node, error) {
	// Generate node identity
	identity, err := crypto.NewNodeIdentity()
	if err != nil {
		return nil, fmt.Errorf("failed to create identity: %w", err)
	}

	// Create UDP transport
	transport, err := transport.NewUDPTransport(config.Port)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}

	// Generate transient key for HMAC
	transientKey := make([]byte, 32)
	if _, err := rand.Read(transientKey); err != nil {
		return nil, fmt.Errorf("failed to generate transient key: %w", err)
	}

	node := &Node{
		identity:         identity,
		transport:        transport,
		transientKey:     transientKey,
		ecdhContexts:     make([]*crypto.ECDHContext, 0, 20),
		port:             config.Port,
		debugMode:        config.DebugMode,
		PeerManager:      peer.NewManager(config.DebugMode),
		SessionTracker:   session.NewTracker(config.DebugMode),
		NPFConnManager:   npf.NewConnectionManager(config.DebugMode),
		activeHandshakes: make(map[string]*protocol.JFKContext),
		stopChan:         make(chan struct{}),
	}

	// Register NPF message handlers
	node.registerMessageHandlers()

	// Pre-generate some ECDH contexts
	for i := 0; i < 5; i++ {
		ctx, err := crypto.NewECDHContext(identity)
		if err != nil {
			return nil, fmt.Errorf("failed to create ECDH context: %w", err)
		}
		node.ecdhContexts = append(node.ecdhContexts, ctx)
	}

	if node.debugMode {
		log.Printf("[NODE] Node created with identity hash: %x", identity.GetIdentityHash()[:8])
		log.Printf("[NODE] Listening on port %d", config.Port)
	}

	return node, nil
}

// ConnectToSeedNode initiates a connection to a seed node
func (n *Node) ConnectToSeedNode(host string, port int) error {
	return n.ConnectToSeedNodeWithIdentity(host, port, "")
}

// ConnectToSeedNodeWithIdentity initiates a connection to a seed node with known identity
func (n *Node) ConnectToSeedNodeWithIdentity(host string, port int, identityBase64 string) error {
	if n.debugMode {
		log.Printf("[NODE] Connecting to seed node %s:%d", host, port)
	}

	// Resolve address
	addr, err := net.ResolveUDPAddr("udp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return fmt.Errorf("failed to resolve seed node address: %w", err)
	}

	// Add seed node to peer manager
	if identityBase64 != "" {
		if err := n.PeerManager.AddSeedNode(addr, identityBase64); err != nil {
			return fmt.Errorf("failed to add seed node: %w", err)
		}
	}

	// Get or create peer
	p := n.PeerManager.GetPeer(addr)
	if p == nil {
		p = peer.NewPeer(addr, true)
		p.IsOpennet = true
		if identityBase64 != "" {
			identity, identityHash, err := crypto.DecodeFreenetIdentity(identityBase64)
			if err != nil {
				return fmt.Errorf("failed to decode seed identity: %w", err)
			}
			p.Identity = identity
			p.IdentityHash = identityHash
		}
		n.PeerManager.AddPeer(p)
	}

	// Start handshake
	return n.initiateHandshake(p)
}

// initiateHandshake starts a JFK handshake with a peer
func (n *Node) initiateHandshake(p *peer.Peer) error {
	// Create JFK context
	jfkCtx, err := protocol.NewJFKContext(n.identity, true, protocol.SetupOpennetSeednode)
	if err != nil {
		return fmt.Errorf("failed to create JFK context: %w", err)
	}

	// Build Message 1
	msg1Data, nonce, err := jfkCtx.BuildMessage1(n.identity, true)
	if err != nil {
		return fmt.Errorf("failed to build JFK message 1: %w", err)
	}

	// Store JFK context for this peer
	n.handshakeMu.Lock()
	n.activeHandshakes[p.Address.String()] = jfkCtx
	n.handshakeMu.Unlock()

	// Update peer state
	p.SetState(peer.StateHandshaking)
	p.JFKContext = jfkCtx

	if n.debugMode {
		log.Printf("[HANDSHAKE] Sending Message 1 to %s (nonce: %x)", p.Address, nonce[:8])
	}

	// Wrap in auth packet and send
	packet := n.buildAuthPacket(1, 10, 0, protocol.SetupOpennetSeednode, msg1Data)
	if err := n.transport.SendTo(packet, p.Address); err != nil {
		return fmt.Errorf("failed to send message 1: %w", err)
	}

	p.UpdateLastSent()
	return nil
}

// buildAuthPacket constructs an auth packet with the JFK message
func (n *Node) buildAuthPacket(version, negType, phase, setupType int, payload []byte) []byte {
	// Simplified packet format
	// In full implementation, this would include encryption and proper formatting
	// For now, just wrap the payload with a header

	// Packet format (simplified):
	// [4 bytes: version/negType/phase/setupType] + payload

	packet := make([]byte, 4+len(payload))
	packet[0] = byte(version)
	packet[1] = byte(negType)
	packet[2] = byte(phase)
	packet[3] = byte(setupType)
	copy(packet[4:], payload)

	return packet
}

// handleIncomingPacket processes an incoming packet
func (n *Node) handleIncomingPacket(data []byte, addr *net.UDPAddr) {
	// Check if we have an established session
	sess := n.SessionTracker.GetSession(addr)
	if sess != nil && sess.IsActive(5*time.Minute) {
		// We have an active session, this should be an NPF packet
		n.handleNPFPacket(data, addr, sess)
		return
	}

	// No active session, handle as JFK handshake
	// Minimum packet size: 4-byte header
	if len(data) < 4 {
		return
	}

	// Parse header
	version := int(data[0])
	negType := int(data[1])
	phase := int(data[2])
	setupType := int(data[3])
	payload := data[4:]

	if n.debugMode {
		log.Printf("[PACKET] From %s: version=%d negType=%d phase=%d setupType=%d len=%d",
			addr, version, negType, phase, setupType, len(payload))
	}

	// Get peer
	p := n.PeerManager.GetPeer(addr)

	// Handle based on phase
	switch phase {
	case protocol.JFKPhase1: // Message 2
		n.handleMessage2(p, payload, addr)
	case protocol.JFKPhase3: // Message 4
		n.handleMessage4(p, payload, addr)
	default:
		if n.debugMode {
			log.Printf("[PACKET] Unknown phase %d from %s", phase, addr)
		}
	}
}

// handleMessage2 processes JFK Message 2
func (n *Node) handleMessage2(p *peer.Peer, data []byte, addr *net.UDPAddr) {
	if p == nil {
		if n.debugMode {
			log.Printf("[HANDSHAKE] Received Message 2 from unknown peer %s", addr)
		}
		return
	}

	// Get JFK context
	n.handshakeMu.RLock()
	jfkCtx := n.activeHandshakes[addr.String()]
	n.handshakeMu.RUnlock()

	if jfkCtx == nil {
		if n.debugMode {
			log.Printf("[HANDSHAKE] No active handshake for %s", addr)
		}
		return
	}

	if n.debugMode {
		log.Printf("[HANDSHAKE] Processing Message 2 from %s", addr)
	}

	// Process Message 2
	if err := jfkCtx.ProcessMessage2(data, n.transientKey, addr.IP); err != nil {
		log.Printf("[HANDSHAKE] Failed to process Message 2: %v", err)
		p.SetState(peer.StateFailed)
		return
	}

	if n.debugMode {
		log.Printf("[HANDSHAKE] ✓ Message 2 verified from %s", addr)
	}

	// Build and send Message 3
	nodeRef := n.buildNodeRef()
	msg3Data, err := jfkCtx.BuildMessage3(nodeRef, n.transientKey, addr.IP)
	if err != nil {
		log.Printf("[HANDSHAKE] Failed to build Message 3: %v", err)
		p.SetState(peer.StateFailed)
		return
	}

	if n.debugMode {
		log.Printf("[HANDSHAKE] Sending Message 3 to %s", addr)
	}

	packet := n.buildAuthPacket(1, 10, 2, protocol.SetupOpennetSeednode, msg3Data)
	if err := n.transport.SendTo(packet, addr); err != nil {
		log.Printf("[HANDSHAKE] Failed to send Message 3: %v", err)
		p.SetState(peer.StateFailed)
		return
	}

	p.UpdateLastSent()
}

// handleMessage4 processes JFK Message 4 and completes handshake
func (n *Node) handleMessage4(p *peer.Peer, data []byte, addr *net.UDPAddr) {
	if p == nil {
		if n.debugMode {
			log.Printf("[HANDSHAKE] Received Message 4 from unknown peer %s", addr)
		}
		return
	}

	// Get JFK context
	n.handshakeMu.RLock()
	jfkCtx := n.activeHandshakes[addr.String()]
	n.handshakeMu.RUnlock()

	if jfkCtx == nil {
		if n.debugMode {
			log.Printf("[HANDSHAKE] No active handshake for %s", addr)
		}
		return
	}

	if n.debugMode {
		log.Printf("[HANDSHAKE] Processing Message 4 from %s", addr)
	}

	// Process Message 4
	nodeRef, err := jfkCtx.ProcessMessage4(data, n.transientKey, addr.IP)
	if err != nil {
		log.Printf("[HANDSHAKE] Failed to process Message 4: %v", err)
		p.SetState(peer.StateFailed)
		return
	}

	if n.debugMode {
		log.Printf("[HANDSHAKE] ✓ Message 4 verified, nodeRef length: %d", len(nodeRef))
	}

	// Derive session keys
	outgoingKey, incomingKey, err := jfkCtx.DeriveSessionKeys()
	if err != nil {
		log.Printf("[HANDSHAKE] Failed to derive session keys: %v", err)
		p.SetState(peer.StateFailed)
		return
	}

	// Create session
	sess, err := n.SessionTracker.CreateSession(p, outgoingKey, incomingKey)
	if err != nil {
		log.Printf("[HANDSHAKE] Failed to create session: %v", err)
		p.SetState(peer.StateFailed)
		return
	}

	// Update peer state
	p.SetSessionKeys(outgoingKey, incomingKey)
	p.SetState(peer.StateConnected)
	p.UpdateLastContact()

	// Remove from active handshakes
	n.handshakeMu.Lock()
	delete(n.activeHandshakes, addr.String())
	n.handshakeMu.Unlock()

	log.Printf("[HANDSHAKE] ✓✓✓ HANDSHAKE COMPLETE with %s ✓✓✓", addr)
	log.Printf("[SESSION] Session established with %s", addr)

	if n.debugMode {
		stats := sess.GetStats()
		log.Printf("[SESSION] Stats: %+v", stats)
	}
}

// buildNodeRef creates a node reference for handshake
func (n *Node) buildNodeRef() []byte {
	// Simplified node ref for now
	// In full implementation, this would include:
	// - Physical addresses
	// - Crypto info
	// - Version info
	// - Capabilities
	return []byte(fmt.Sprintf("node-%x", n.identity.GetIdentityHash()[:8]))
}

// Start starts the node event loop
func (n *Node) Start() error {
	n.runMu.Lock()
	if n.running {
		n.runMu.Unlock()
		return fmt.Errorf("node already running")
	}
	n.running = true
	n.runMu.Unlock()

	if n.debugMode {
		log.Printf("[NODE] Node started")
	}

	// Start packet receive loop
	go n.receiveLoop()

	// Start packet send loop
	go n.sendPacketLoop()

	return nil
}

// receiveLoop continuously receives and processes packets
func (n *Node) receiveLoop() {
	buffer := make([]byte, 4096)

	for {
		select {
		case <-n.stopChan:
			return
		default:
		}

		// Set read timeout so we can check stopChan periodically
		// Note: This is handled by the transport layer

		bytesRead, addr, err := n.transport.ReceiveFrom(buffer)
		if err != nil {
			// Check if we're shutting down
			select {
			case <-n.stopChan:
				return
			default:
			}

			if n.debugMode {
				log.Printf("[NODE] Error receiving packet: %v", err)
			}
			continue
		}

		if bytesRead == 0 {
			continue
		}

		// Copy packet data
		packet := make([]byte, bytesRead)
		copy(packet, buffer[:bytesRead])

		// Process in goroutine to avoid blocking receive
		go n.handleIncomingPacket(packet, addr)
	}
}

// Stop stops the node
func (n *Node) Stop() error {
	n.runMu.Lock()
	if !n.running {
		n.runMu.Unlock()
		return nil
	}
	n.running = false
	n.runMu.Unlock()

	// Signal stop
	close(n.stopChan)

	// Stop session tracker
	n.SessionTracker.Stop()

	// Close transport
	if err := n.transport.Close(); err != nil {
		return fmt.Errorf("failed to close transport: %w", err)
	}

	if n.debugMode {
		log.Printf("[NODE] Node stopped")
	}

	return nil
}

// GetIdentityHash returns the node's identity hash
func (n *Node) GetIdentityHash() []byte {
	return n.identity.GetIdentityHash()
}

// SendPing sends a ping message to a peer
func (n *Node) SendPing(addr *net.UDPAddr) error {
	ping := npf.CreatePingMessage(1, time.Now().Unix())
	return n.NPFConnManager.Send(addr, ping)
}

// registerMessageHandlers registers NPF message handlers
func (n *Node) registerMessageHandlers() {
	// Ping handler
	n.NPFConnManager.Register(npf.MsgTypePing, func(msg *npf.NPFMessage, from *net.UDPAddr) error {
		seqno, _ := msg.GetInt64("seqno")
		timestamp, _ := msg.GetInt64("timestamp")

		if n.debugMode {
			log.Printf("[NPF] Received ping #%d from %s", seqno, from)
		}

		// Send pong
		pong := npf.CreatePongMessage(seqno, timestamp)
		return n.NPFConnManager.Send(from, pong)
	})

	// Pong handler
	n.NPFConnManager.Register(npf.MsgTypePong, func(msg *npf.NPFMessage, from *net.UDPAddr) error {
		seqno, _ := msg.GetInt64("seqno")
		timestamp, _ := msg.GetInt64("timestamp")

		if n.debugMode {
			rtt := time.Now().Unix() - timestamp
			log.Printf("[NPF] Received pong #%d from %s (RTT: %ds)", seqno, from, rtt)
		}

		return nil
	})

	// Disconnect handler
	n.NPFConnManager.Register(npf.MsgTypeDisconnect, func(msg *npf.NPFMessage, from *net.UDPAddr) error {
		reason, _ := msg.GetString("reason")
		log.Printf("[NPF] Peer %s disconnecting: %s", from, reason)

		// Remove session
		n.SessionTracker.RemoveSession(from)

		return nil
	})

	// Void message handler (keepalive)
	n.NPFConnManager.Register(npf.MsgTypeVoid, func(msg *npf.NPFMessage, from *net.UDPAddr) error {
		if n.debugMode {
			log.Printf("[NPF] Received keepalive from %s", from)
		}
		return nil
	})
}

// handleNPFPacket processes an NPF packet from an established session
func (n *Node) handleNPFPacket(data []byte, addr *net.UDPAddr, sess *session.Session) {
	// Decrypt packet
	plaintext, err := sess.DecryptPacket(data)
	if err != nil {
		if n.debugMode {
			log.Printf("[NPF] Failed to decrypt packet from %s: %v", addr, err)
		}
		return
	}

	// Parse NPF packet
	packet, err := npf.Parse(plaintext)
	if err != nil {
		if n.debugMode {
			log.Printf("[NPF] Failed to parse packet from %s: %v", addr, err)
		}
		return
	}

	if n.debugMode {
		log.Printf("[NPF] Received packet from %s: %s", addr, packet)
	}

	// Process packet (will dispatch messages to handlers)
	if err := n.NPFConnManager.ProcessPacket(packet, addr); err != nil {
		if n.debugMode {
			log.Printf("[NPF] Error processing packet from %s: %v", addr, err)
		}
	}
}

// sendPacketLoop periodically sends NPF packets to connected peers
func (n *Node) sendPacketLoop() {
	n.packetSendTick = time.NewTicker(500 * time.Millisecond) // Send every 500ms
	defer n.packetSendTick.Stop()

	for {
		select {
		case <-n.packetSendTick.C:
			n.sendQueuedPackets()
		case <-n.stopChan:
			return
		}
	}
}

// sendQueuedPackets sends queued NPF packets to all connected peers
func (n *Node) sendQueuedPackets() {
	// Get all connected peers
	connectedPeers := n.PeerManager.GetConnectedPeers()

	for _, p := range connectedPeers {
		// Get NPF connection
		conn := n.NPFConnManager.GetConnection(p.Address)
		if conn == nil {
			continue
		}

		// Check if there's anything to send
		if !conn.HasQueuedMessages() && !conn.HasPendingAcks() {
			// Send keepalive every 30 seconds
			// TODO: Track last send time and only send if needed
			continue
		}

		// Build packet
		packet, err := conn.BuildOutgoingPacket(1280)
		if err != nil {
			if n.debugMode {
				log.Printf("[NPF] Failed to build packet for %s: %v", p.Address, err)
			}
			continue
		}

		// Serialize packet
		plaintext, err := packet.Serialize(1200) // Leave room for encryption overhead
		if err != nil {
			if n.debugMode {
				log.Printf("[NPF] Failed to serialize packet for %s: %v", p.Address, err)
			}
			continue
		}

		// Get session
		sess := n.SessionTracker.GetSession(p.Address)
		if sess == nil {
			if n.debugMode {
				log.Printf("[NPF] No session for %s", p.Address)
			}
			continue
		}

		// Encrypt packet
		encrypted, err := sess.EncryptPacket(plaintext)
		if err != nil {
			if n.debugMode {
				log.Printf("[NPF] Failed to encrypt packet for %s: %v", p.Address, err)
			}
			continue
		}

		// Send packet
		if err := n.transport.SendTo(encrypted, p.Address); err != nil {
			if n.debugMode {
				log.Printf("[NPF] Failed to send packet to %s: %v", p.Address, err)
			}
			continue
		}

		if n.debugMode {
			log.Printf("[NPF] Sent packet to %s: %s", p.Address, packet)
		}
	}
}

// GetStats returns node statistics
func (n *Node) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})

	// Peer stats
	peerStats := n.PeerManager.GetStats()
	stats["peers"] = peerStats

	// Session stats
	sessionStats := n.SessionTracker.GetStats()
	stats["sessions"] = sessionStats

	// NPF stats
	npfStats := n.NPFConnManager.GetStats()
	stats["npf"] = npfStats

	// Handshake stats
	n.handshakeMu.RLock()
	stats["active_handshakes"] = len(n.activeHandshakes)
	n.handshakeMu.RUnlock()

	return stats
}

// IsRunning returns true if the node is running
func (n *Node) IsRunning() bool {
	n.runMu.Lock()
	defer n.runMu.Unlock()
	return n.running
}
