// GoHyphanet - Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package main

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
)

var debugMode bool

func debugLog(format string, args ...interface{}) {
	if debugMode {
		log.Printf("[TORRENTPROXY] "+format, args...)
	}
}

// TorrentInfo stores information about a torrent
type TorrentInfo struct {
	InfoHash    string            `json:"info_hash"`
	HyphanetURI string            `json:"hyphanet_uri"`
	Name        string            `json:"name"`
	Size        int64             `json:"size"`
	PieceLength int               `json:"piece_length"`
	Pieces      int               `json:"pieces"`
	Files       []TorrentFile     `json:"files"`
	Peers       []string          `json:"peers"`
	Created     time.Time         `json:"created"`
	Metadata    map[string]string `json:"metadata"`
}

// TorrentFile represents a file in the torrent
type TorrentFile struct {
	Path   string `json:"path"`
	Length int64  `json:"length"`
}

// TorrentRegistry manages known torrents
type TorrentRegistry struct {
	torrents map[string]*TorrentInfo // infohash -> torrent info
	mu       sync.RWMutex
	dbFile   string
	client   *fcp.Client
}

// NewTorrentRegistry creates a new torrent registry
func NewTorrentRegistry(dbFile string, client *fcp.Client) (*TorrentRegistry, error) {
	tr := &TorrentRegistry{
		torrents: make(map[string]*TorrentInfo),
		dbFile:   dbFile,
		client:   client,
	}

	if err := tr.Load(); err != nil {
		debugLog("Failed to load registry: %v", err)
	}

	return tr, nil
}

// Load loads torrents from database
func (tr *TorrentRegistry) Load() error {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	data, err := os.ReadFile(tr.dbFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if len(data) == 0 {
		return nil
	}

	return json.Unmarshal(data, &tr.torrents)
}

// Save saves torrents to database
func (tr *TorrentRegistry) Save() error {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	data, err := json.MarshalIndent(tr.torrents, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(tr.dbFile, data, 0644)
}

// Register adds a torrent to the registry
func (tr *TorrentRegistry) Register(info *TorrentInfo) error {
	tr.mu.Lock()
	tr.torrents[info.InfoHash] = info
	tr.mu.Unlock()

	return tr.Save()
}

// Get retrieves torrent info by hash
func (tr *TorrentRegistry) Get(infoHash string) (*TorrentInfo, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	info, exists := tr.torrents[infoHash]
	return info, exists
}

// List returns all registered torrents
func (tr *TorrentRegistry) List() []*TorrentInfo {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	result := make([]*TorrentInfo, 0, len(tr.torrents))
	for _, info := range tr.torrents {
		result = append(result, info)
	}
	return result
}

// TrackerProxy implements a BitTorrent tracker that uses Hyphanet
type TrackerProxy struct {
	listenAddr string
	registry   *TorrentRegistry
	listener   net.Listener
}

// NewTrackerProxy creates a new tracker proxy
func NewTrackerProxy(listenAddr string, registry *TorrentRegistry) *TrackerProxy {
	return &TrackerProxy{
		listenAddr: listenAddr,
		registry:   registry,
	}
}

// Start starts the tracker proxy
func (tp *TrackerProxy) Start() error {
	listener, err := net.Listen("tcp", tp.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to start tracker: %w", err)
	}

	tp.listener = listener
	log.Printf("Tracker listening on: %s", tp.listenAddr)

	go tp.acceptLoop()
	return nil
}

// acceptLoop accepts incoming connections
func (tp *TrackerProxy) acceptLoop() {
	for {
		conn, err := tp.listener.Accept()
		if err != nil {
			debugLog("Accept error: %v", err)
			continue
		}

		go tp.handleConnection(conn)
	}
}

// handleConnection handles a tracker request
func (tp *TrackerProxy) handleConnection(conn net.Conn) {
	defer conn.Close()

	debugLog("New tracker connection from: %s", conn.RemoteAddr())

	// Read HTTP request
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		debugLog("Read error: %v", err)
		return
	}

	request := string(buf[:n])
	debugLog("Tracker request:\n%s", request)

	// Parse the request (simplified HTTP GET parser)
	lines := strings.Split(request, "\r\n")
	if len(lines) < 1 {
		return
	}

	// Parse request line: GET /announce?info_hash=... HTTP/1.1
	parts := strings.Fields(lines[0])
	if len(parts) < 2 || parts[0] != "GET" {
		tp.sendError(conn, "Invalid request")
		return
	}

	// Extract query parameters
	path := parts[1]
	if !strings.HasPrefix(path, "/announce") {
		tp.sendError(conn, "Invalid endpoint")
		return
	}

	// Parse query string
	queryStart := strings.Index(path, "?")
	if queryStart == -1 {
		tp.sendError(conn, "Missing query parameters")
		return
	}

	params := parseQuery(path[queryStart+1:])
	infoHash := params["info_hash"]

	if infoHash == "" {
		tp.sendError(conn, "Missing info_hash")
		return
	}

	// Convert info_hash to hex
	infoHashHex := hex.EncodeToString([]byte(infoHash))
	debugLog("Info hash: %s", infoHashHex)

	// Look up torrent
	torrentInfo, exists := tp.registry.Get(infoHashHex)
	if !exists {
		// Try to fetch from Hyphanet
		debugLog("Torrent not found locally, searching Hyphanet...")
		tp.sendError(conn, "Torrent not found")
		return
	}

	// Build tracker response
	response := tp.buildTrackerResponse(torrentInfo, params)

	// Send response
	httpResponse := fmt.Sprintf(
		"HTTP/1.1 200 OK\r\n"+
			"Content-Type: text/plain\r\n"+
			"Content-Length: %d\r\n"+
			"\r\n%s",
		len(response), response)

	conn.Write([]byte(httpResponse))
}

// buildTrackerResponse builds a bencoded tracker response
func (tp *TrackerProxy) buildTrackerResponse(info *TorrentInfo, params map[string]string) string {
	// Simplified bencode response
	// In production, use proper bencode library
	peers := ""
	for _, peer := range info.Peers {
		peers += peer + ","
	}
	peers = strings.TrimSuffix(peers, ",")

	return fmt.Sprintf("d8:completei0e10:incompletei0e8:intervali1800e5:peers0:e")
}

// sendError sends an error response
func (tp *TrackerProxy) sendError(conn net.Conn, message string) {
	response := fmt.Sprintf("d14:failure reason%d:%se", len(message), message)
	httpResponse := fmt.Sprintf(
		"HTTP/1.1 200 OK\r\n"+
			"Content-Type: text/plain\r\n"+
			"Content-Length: %d\r\n"+
			"\r\n%s",
		len(response), response)

	conn.Write([]byte(httpResponse))
}

// parseQuery parses URL query string
func parseQuery(query string) map[string]string {
	params := make(map[string]string)
	for _, pair := range strings.Split(query, "&") {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			// URL decode would go here
			params[kv[0]] = kv[1]
		}
	}
	return params
}

// PeerProxy handles peer-to-peer connections over Hyphanet
type PeerProxy struct {
	listenAddr string
	registry   *TorrentRegistry
	client     *fcp.Client
	listener   net.Listener
	peers      map[string]*PeerConnection
	mu         sync.RWMutex
}

// PeerConnection represents a connection to a peer
type PeerConnection struct {
	peerID      string
	infoHash    string
	hyphanetURI string
	conn        net.Conn
	lastActive  time.Time
}

// NewPeerProxy creates a new peer proxy
func NewPeerProxy(listenAddr string, registry *TorrentRegistry, client *fcp.Client) *PeerProxy {
	return &PeerProxy{
		listenAddr: listenAddr,
		registry:   registry,
		client:     client,
		peers:      make(map[string]*PeerConnection),
	}
}

// Start starts the peer proxy
func (pp *PeerProxy) Start() error {
	listener, err := net.Listen("tcp", pp.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to start peer proxy: %w", err)
	}

	pp.listener = listener
	log.Printf("Peer proxy listening on: %s", pp.listenAddr)

	go pp.acceptLoop()
	go pp.cleanupLoop()

	return nil
}

// acceptLoop accepts incoming peer connections
func (pp *PeerProxy) acceptLoop() {
	for {
		conn, err := pp.listener.Accept()
		if err != nil {
			debugLog("Peer accept error: %v", err)
			continue
		}

		go pp.handlePeerConnection(conn)
	}
}

// handlePeerConnection handles a BitTorrent peer protocol connection
func (pp *PeerProxy) handlePeerConnection(conn net.Conn) {
	defer conn.Close()

	debugLog("New peer connection from: %s", conn.RemoteAddr())

	// Read BitTorrent handshake
	handshake := make([]byte, 68)
	n, err := io.ReadFull(conn, handshake)
	if err != nil || n != 68 {
		debugLog("Failed to read handshake: %v", err)
		return
	}

	// Parse handshake
	if handshake[0] != 19 || string(handshake[1:20]) != "BitTorrent protocol" {
		debugLog("Invalid protocol")
		return
	}

	infoHash := handshake[28:48]
	peerID := handshake[48:68]

	infoHashHex := hex.EncodeToString(infoHash)
	peerIDHex := hex.EncodeToString(peerID)

	debugLog("Peer handshake: infohash=%s peerid=%s", infoHashHex, peerIDHex)

	// Look up torrent
	torrentInfo, exists := pp.registry.Get(infoHashHex)
	if !exists {
		debugLog("Torrent not found: %s", infoHashHex)
		return
	}

	// Send handshake response
	ourPeerID := make([]byte, 20)
	copy(ourPeerID, []byte("-HN0100-")) // Hyphanet client ID
	
	response := make([]byte, 68)
	response[0] = 19
	copy(response[1:20], "BitTorrent protocol")
	copy(response[28:48], infoHash)
	copy(response[48:68], ourPeerID)

	if _, err := conn.Write(response); err != nil {
		debugLog("Failed to send handshake: %v", err)
		return
	}

	debugLog("Handshake complete, proxying data via Hyphanet")

	// Create peer connection
	peer := &PeerConnection{
		peerID:      peerIDHex,
		infoHash:    infoHashHex,
		hyphanetURI: torrentInfo.HyphanetURI,
		conn:        conn,
		lastActive:  time.Now(),
	}

	pp.mu.Lock()
	pp.peers[peerIDHex] = peer
	pp.mu.Unlock()

	// Handle peer protocol messages
	pp.handlePeerMessages(peer)
}

// handlePeerMessages handles BitTorrent protocol messages
func (pp *PeerProxy) handlePeerMessages(peer *PeerConnection) {
	for {
		// Read message length (4 bytes)
		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(peer.conn, lenBuf); err != nil {
			debugLog("Peer disconnected: %v", err)
			return
		}

		msgLen := binary.BigEndian.Uint32(lenBuf)
		if msgLen == 0 {
			// Keep-alive
			continue
		}

		// Read message
		msg := make([]byte, msgLen)
		if _, err := io.ReadFull(peer.conn, msg); err != nil {
			debugLog("Failed to read message: %v", err)
			return
		}

		if len(msg) == 0 {
			continue
		}

		msgType := msg[0]
		debugLog("Peer message type: %d, length: %d", msgType, msgLen)

		// Handle message types
		switch msgType {
		case 0: // choke
			debugLog("Peer choked us")
		case 1: // unchoke
			debugLog("Peer unchoked us")
		case 2: // interested
			debugLog("Peer interested")
		case 3: // not interested
			debugLog("Peer not interested")
		case 4: // have
			debugLog("Peer has piece")
		case 5: // bitfield
			debugLog("Peer sent bitfield")
		case 6: // request
			pp.handleRequest(peer, msg[1:])
		case 7: // piece
			debugLog("Received piece data")
		case 8: // cancel
			debugLog("Peer cancelled request")
		default:
			debugLog("Unknown message type: %d", msgType)
		}

		peer.lastActive = time.Now()
	}
}

// handleRequest handles a piece request
func (pp *PeerProxy) handleRequest(peer *PeerConnection, payload []byte) {
	if len(payload) < 12 {
		return
	}

	index := binary.BigEndian.Uint32(payload[0:4])
	begin := binary.BigEndian.Uint32(payload[4:8])
	length := binary.BigEndian.Uint32(payload[8:12])

	debugLog("Piece request: index=%d begin=%d length=%d", index, begin, length)

	// Fetch piece from Hyphanet
	go pp.fetchPieceFromHyphanet(peer, index, begin, length)
}

// fetchPieceFromHyphanet fetches a piece from Hyphanet and sends it to peer
func (pp *PeerProxy) fetchPieceFromHyphanet(peer *PeerConnection, index, begin, length uint32) {
	debugLog("Fetching piece %d from Hyphanet", index)

	torrentInfo, exists := pp.registry.Get(peer.infoHash)
	if !exists {
		return
	}

	// Construct URI for specific piece
	pieceURI := fmt.Sprintf("%s/piece-%d", torrentInfo.HyphanetURI, index)

	// Fetch from Hyphanet
	ops := fcp.NewOperations(pp.client)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := ops.Get(ctx, pieceURI)
	if err != nil {
		debugLog("Failed to fetch piece from Hyphanet: %v", err)
		return
	}

	if !result.Success {
		debugLog("Piece fetch failed: %s", result.Error)
		return
	}

	// Extract requested range
	pieceData := result.Data
	if begin+length > uint32(len(pieceData)) {
		debugLog("Invalid range requested")
		return
	}

	data := pieceData[begin : begin+length]

	// Send piece message to peer
	// Message format: <len><id><index><begin><block>
	msgLen := 9 + len(data)
	msg := make([]byte, 4+msgLen)
	binary.BigEndian.PutUint32(msg[0:4], uint32(msgLen))
	msg[4] = 7 // piece message ID
	binary.BigEndian.PutUint32(msg[5:9], index)
	binary.BigEndian.PutUint32(msg[9:13], begin)
	copy(msg[13:], data)

	if _, err := peer.conn.Write(msg); err != nil {
		debugLog("Failed to send piece: %v", err)
		return
	}

	debugLog("Sent piece %d to peer", index)
}

// cleanupLoop removes stale peer connections
func (pp *PeerProxy) cleanupLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		pp.mu.Lock()
		for id, peer := range pp.peers {
			if time.Since(peer.lastActive) > 10*time.Minute {
				debugLog("Removing stale peer: %s", id)
				peer.conn.Close()
				delete(pp.peers, id)
			}
		}
		pp.mu.Unlock()
	}
}

func main() {
	trackerAddr := flag.String("tracker", "127.0.0.1:6969", "Tracker listen address")
	peerAddr := flag.String("peer", "127.0.0.1:6881", "Peer proxy listen address")
	fcpHost := flag.String("fcp-host", "localhost", "FCP host")
	fcpPort := flag.Int("fcp-port", 9481, "FCP port")
	dbFile := flag.String("db", "torrentproxy.json", "Torrent database file")
	debug := flag.Bool("debug", false, "Enable debug logging")
	verbose := flag.Bool("v", false, "Verbose output")
	showVersion := flag.Bool("version", false, "Show version information")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "torrentproxy - BitTorrent over Hyphanet v%s\n\n", fcp.Version)
		fmt.Fprintf(os.Stderr, "A BitTorrent proxy that routes torrent traffic over Hyphanet for\n")
		fmt.Fprintf(os.Stderr, "anonymous and censorship-resistant file sharing.\n\n")
		fmt.Fprintf(os.Stderr, "Usage: torrentproxy [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nSetup:\n")
		fmt.Fprintf(os.Stderr, "  1. Start torrentproxy\n")
		fmt.Fprintf(os.Stderr, "  2. Configure your torrent client:\n")
		fmt.Fprintf(os.Stderr, "     - Tracker: http://127.0.0.1:6969/announce\n")
		fmt.Fprintf(os.Stderr, "     - Peer port: 6881\n")
		fmt.Fprintf(os.Stderr, "  3. Add torrents and start downloading!\n\n")
		fmt.Fprintf(os.Stderr, "Note: This proxies BitTorrent protocol over Hyphanet's anonymous network.\n")
		fmt.Fprintf(os.Stderr, "Performance will be slower than direct BitTorrent connections.\n\n")
		fmt.Fprintf(os.Stderr, "Source: %s\n", fcp.SourceURL)
	}

	flag.Parse()

	debugMode = *debug || *verbose

	if debugMode {
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
		debugLog("Debug mode enabled")
		fcp.DebugMode = true
	}

	if *showVersion {
		fmt.Println(fcp.GetFullVersionString())
		os.Exit(0)
	}

	log.Printf("TorrentProxy v%s - BitTorrent over Hyphanet", fcp.Version)
	log.Println()

	// Connect to Hyphanet
	config := &fcp.Config{
		Host:    *fcpHost,
		Port:    *fcpPort,
		Name:    "torrentproxy",
		Version: "2.0",
	}

	debugLog("Connecting to Hyphanet at %s:%d", *fcpHost, *fcpPort)
	client, err := fcp.Connect(config)
	if err != nil {
		log.Fatalf("Failed to connect to Hyphanet: %v", err)
	}
	defer client.Close()

	// Start FCP listener
	go func() {
		debugLog("Starting FCP listener...")
		if err := client.Listen(); err != nil && err != io.EOF {
			debugLog("FCP listener error: %v", err)
		}
	}()

	time.Sleep(200 * time.Millisecond)
	log.Println("✓ Connected to Hyphanet")

	// Create torrent registry
	registry, err := NewTorrentRegistry(*dbFile, client)
	if err != nil {
		log.Fatalf("Failed to create registry: %v", err)
	}

	log.Printf("Torrent database: %s", *dbFile)
	log.Printf("Registered torrents: %d", len(registry.List()))
	log.Println()

	// Start tracker proxy
	tracker := NewTrackerProxy(*trackerAddr, registry)
	if err := tracker.Start(); err != nil {
		log.Fatalf("Failed to start tracker: %v", err)
	}

	// Start peer proxy
	peerProxy := NewPeerProxy(*peerAddr, registry, client)
	if err := peerProxy.Start(); err != nil {
		log.Fatalf("Failed to start peer proxy: %v", err)
	}

	log.Println()
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("TorrentProxy is running!")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println()
	log.Println("Configure your BitTorrent client:")
	log.Printf("  Tracker URL: http://%s/announce\n", *trackerAddr)
	log.Printf("  Listen port: %s\n", strings.Split(*peerAddr, ":")[1])
	log.Println()
	log.Println("All torrent traffic will be routed through Hyphanet")
	log.Println("Press Ctrl+C to stop")
	log.Println()

	// Wait forever
	select {}
}
