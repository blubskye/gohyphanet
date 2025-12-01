package fcp_server

import (
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/node/requests"
	"github.com/blubskye/gohyphanet/node/routing"
	"github.com/blubskye/gohyphanet/node/store"
)

const (
	// DefaultFCPPort is the standard FCP port
	DefaultFCPPort = 9481

	// ProtocolVersion is the FCP protocol version we support
	ProtocolVersion = "2.0"
)

// FCPServer handles incoming FCP (Freenet Client Protocol) connections
type FCPServer struct {
	mu sync.RWMutex

	// Configuration
	bindAddr   string
	port       int
	enabled    bool
	allowedIPs []string

	// Network
	listener net.Listener
	shutdown chan struct{}
	wg       sync.WaitGroup

	// Node dependencies
	datastore      store.FreenetStore
	locationMgr    *routing.LocationManager
	htlManager     *routing.HTLManager
	requestTracker requests.RequestTrackerInterface

	// Active connections
	connections   map[string]*FCPConnection
	connIDCounter uint64
}

// ServerConfig contains FCP server configuration
type ServerConfig struct {
	BindAddr   string
	Port       int
	Enabled    bool
	AllowedIPs []string
}

// DefaultServerConfig returns default FCP server configuration
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		BindAddr:   "127.0.0.1",
		Port:       DefaultFCPPort,
		Enabled:    true,
		AllowedIPs: []string{"127.0.0.1", "::1"},
	}
}

// NewFCPServer creates a new FCP server
func NewFCPServer(
	config *ServerConfig,
	datastore store.FreenetStore,
	locationMgr *routing.LocationManager,
	htlManager *routing.HTLManager,
	requestTracker requests.RequestTrackerInterface,
) *FCPServer {
	if config == nil {
		config = DefaultServerConfig()
	}

	return &FCPServer{
		bindAddr:       config.BindAddr,
		port:           config.Port,
		enabled:        config.Enabled,
		allowedIPs:     config.AllowedIPs,
		shutdown:       make(chan struct{}),
		datastore:      datastore,
		locationMgr:    locationMgr,
		htlManager:     htlManager,
		requestTracker: requestTracker,
		connections:    make(map[string]*FCPConnection),
	}
}

// Start starts the FCP server
func (s *FCPServer) Start() error {
	if !s.enabled {
		log.Printf("FCP server disabled, not starting")
		return nil
	}

	addr := fmt.Sprintf("%s:%d", s.bindAddr, s.port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", addr, err)
	}

	s.listener = listener
	log.Printf("FCP server listening on %s", addr)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop stops the FCP server
func (s *FCPServer) Stop() error {
	if !s.enabled {
		return nil
	}

	log.Printf("Stopping FCP server...")

	// Signal shutdown
	close(s.shutdown)

	// Close listener
	if s.listener != nil {
		s.listener.Close()
	}

	// Close all connections
	s.mu.Lock()
	for _, conn := range s.connections {
		conn.Close()
	}
	s.mu.Unlock()

	// Wait for all goroutines
	s.wg.Wait()

	log.Printf("FCP server stopped")
	return nil
}

// acceptLoop accepts incoming connections
func (s *FCPServer) acceptLoop() {
	defer s.wg.Done()

	for {
		select {
		case <-s.shutdown:
			return
		default:
		}

		// Set accept deadline
		if tcpListener, ok := s.listener.(*net.TCPListener); ok {
			tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
		}

		conn, err := s.listener.Accept()
		if err != nil {
			// Check if it's a timeout (expected due to deadline)
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}

			// Check if we're shutting down
			select {
			case <-s.shutdown:
				return
			default:
				log.Printf("FCP accept error: %v", err)
				continue
			}
		}

		// Check if IP is allowed
		remoteAddr := conn.RemoteAddr().String()
		if !s.isIPAllowed(remoteAddr) {
			log.Printf("FCP connection from unauthorized IP: %s", remoteAddr)
			conn.Close()
			continue
		}

		// Handle connection
		s.handleConnection(conn)
	}
}

// handleConnection handles a new FCP connection
func (s *FCPServer) handleConnection(netConn net.Conn) {
	s.mu.Lock()
	s.connIDCounter++
	connID := fmt.Sprintf("fcp-%d", s.connIDCounter)
	s.mu.Unlock()

	log.Printf("New FCP connection: %s from %s", connID, netConn.RemoteAddr())

	conn := NewFCPConnection(connID, netConn, s)

	s.mu.Lock()
	s.connections[connID] = conn
	s.mu.Unlock()

	// Start connection handler
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		conn.Handle()

		// Remove from active connections
		s.mu.Lock()
		delete(s.connections, connID)
		s.mu.Unlock()

		log.Printf("FCP connection closed: %s", connID)
	}()
}

// isIPAllowed checks if an IP address is allowed to connect
func (s *FCPServer) isIPAllowed(addr string) bool {
	// Extract IP from address (addr:port format)
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// If no port, assume it's just the IP
		host = addr
	}

	// Check against allowed IPs
	for _, allowedIP := range s.allowedIPs {
		if host == allowedIP {
			return true
		}
	}

	return false
}

// GetStats returns FCP server statistics
func (s *FCPServer) GetStats() FCPServerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return FCPServerStats{
		Enabled:           s.enabled,
		ListenAddr:        fmt.Sprintf("%s:%d", s.bindAddr, s.port),
		ActiveConnections: len(s.connections),
	}
}

// FCPServerStats contains FCP server statistics
type FCPServerStats struct {
	Enabled           bool
	ListenAddr        string
	ActiveConnections int
}

// String returns a formatted string of server statistics
func (fss FCPServerStats) String() string {
	status := "disabled"
	if fss.Enabled {
		status = "enabled"
	}
	return fmt.Sprintf("FCP Server: %s, listening on %s, %d active connections",
		status, fss.ListenAddr, fss.ActiveConnections)
}
