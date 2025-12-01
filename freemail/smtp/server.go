// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

// Package smtp implements an SMTP server for Freemail.
package smtp

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

// Server configuration
const (
	DefaultPort     = 3025
	DefaultHostname = "localhost"
	ReadTimeout     = 5 * time.Minute
	WriteTimeout    = 5 * time.Minute
	MaxMessageSize  = 10 * 1024 * 1024 // 10MB
	MaxRecipients   = 100
)

// Server represents an SMTP server
type Server struct {
	mu sync.RWMutex

	// Configuration
	hostname string
	port     int
	address  string

	// Listener
	listener net.Listener

	// State
	running bool

	// Handler for processing messages
	handler MessageHandler

	// Authenticator for validating credentials
	auth Authenticator

	// Active connections
	connections map[*Connection]struct{}

	// Shutdown
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// MessageHandler interface for handling submitted messages
type MessageHandler interface {
	// HandleMessage processes a submitted email message
	HandleMessage(from string, to []string, data []byte) error
}

// Authenticator interface for validating credentials
type Authenticator interface {
	// Authenticate validates username and password
	Authenticate(username, password string) (bool, error)

	// GetAccountID returns the account ID for a username
	GetAccountID(username string) string
}

// NewServer creates a new SMTP server
func NewServer(port int, hostname string) *Server {
	if hostname == "" {
		hostname = DefaultHostname
	}
	if port <= 0 {
		port = DefaultPort
	}

	return &Server{
		hostname:    hostname,
		port:        port,
		address:     fmt.Sprintf(":%d", port),
		connections: make(map[*Connection]struct{}),
		stopChan:    make(chan struct{}),
	}
}

// SetHandler sets the message handler
func (s *Server) SetHandler(handler MessageHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = handler
}

// SetAuthenticator sets the authenticator
func (s *Server) SetAuthenticator(auth Authenticator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.auth = auth
}

// Start starts the SMTP server
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}

	listener, err := net.Listen("tcp", s.address)
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to listen on %s: %w", s.address, err)
	}

	s.listener = listener
	s.running = true
	s.stopChan = make(chan struct{})
	s.mu.Unlock()

	log.Printf("SMTP server listening on %s", s.address)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop stops the SMTP server
func (s *Server) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	close(s.stopChan)

	// Close listener
	if s.listener != nil {
		s.listener.Close()
	}

	// Close all connections
	for conn := range s.connections {
		conn.Close()
	}
	s.mu.Unlock()

	s.wg.Wait()
	log.Printf("SMTP server stopped")
	return nil
}

// acceptLoop accepts incoming connections
func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopChan:
				return
			default:
				log.Printf("SMTP accept error: %v", err)
				continue
			}
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

// handleConnection handles a single client connection
func (s *Server) handleConnection(netConn net.Conn) {
	defer s.wg.Done()

	conn := NewConnection(netConn, s)

	s.mu.Lock()
	s.connections[conn] = struct{}{}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.connections, conn)
		s.mu.Unlock()
		conn.Close()
	}()

	conn.Handle()
}

// GetHostname returns the server hostname
func (s *Server) GetHostname() string {
	return s.hostname
}

// GetHandler returns the message handler
func (s *Server) GetHandler() MessageHandler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.handler
}

// GetAuthenticator returns the authenticator
func (s *Server) GetAuthenticator() Authenticator {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.auth
}

// Connection represents a client SMTP connection
type Connection struct {
	mu sync.Mutex

	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	server *Server

	// Session state
	state       SessionState
	helo        string
	authenticated bool
	username    string
	mailFrom    string
	rcptTo      []string
	data        []byte

	// Connection state
	closed bool
}

// SessionState represents the state of an SMTP session
type SessionState int

const (
	StateInit SessionState = iota
	StateGreeted
	StateAuthenticated
	StateMailFrom
	StateRcptTo
	StateData
)

// NewConnection creates a new SMTP connection
func NewConnection(conn net.Conn, server *Server) *Connection {
	return &Connection{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
		server: server,
		state:  StateInit,
		rcptTo: make([]string, 0),
	}
}

// Handle handles the SMTP session
func (c *Connection) Handle() {
	// Send greeting
	c.writeResponse(220, fmt.Sprintf("%s GoFreemail SMTP ready", c.server.GetHostname()))

	for {
		// Set read timeout
		c.conn.SetReadDeadline(time.Now().Add(ReadTimeout))

		// Read command
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Parse command
		cmd, args := c.parseCommand(line)

		// Handle command
		if !c.handleCommand(cmd, args) {
			return
		}
	}
}

// parseCommand parses an SMTP command line
func (c *Connection) parseCommand(line string) (string, string) {
	parts := strings.SplitN(line, " ", 2)
	cmd := strings.ToUpper(parts[0])
	args := ""
	if len(parts) > 1 {
		args = parts[1]
	}
	return cmd, args
}

// handleCommand handles a single SMTP command
func (c *Connection) handleCommand(cmd, args string) bool {
	switch cmd {
	case "HELO":
		return c.handleHELO(args)
	case "EHLO":
		return c.handleEHLO(args)
	case "AUTH":
		return c.handleAUTH(args)
	case "MAIL":
		return c.handleMAIL(args)
	case "RCPT":
		return c.handleRCPT(args)
	case "DATA":
		return c.handleDATA()
	case "RSET":
		return c.handleRSET()
	case "NOOP":
		return c.handleNOOP()
	case "QUIT":
		return c.handleQUIT()
	case "VRFY":
		return c.handleVRFY(args)
	case "HELP":
		return c.handleHELP()
	default:
		c.writeResponse(500, "Command not recognized")
		return true
	}
}

// handleHELO handles the HELO command
func (c *Connection) handleHELO(args string) bool {
	if args == "" {
		c.writeResponse(501, "HELO requires domain argument")
		return true
	}

	c.helo = args
	c.state = StateGreeted
	c.writeResponse(250, fmt.Sprintf("Hello %s", args))
	return true
}

// handleEHLO handles the EHLO command
func (c *Connection) handleEHLO(args string) bool {
	if args == "" {
		c.writeResponse(501, "EHLO requires domain argument")
		return true
	}

	c.helo = args
	c.state = StateGreeted

	// Send capabilities
	c.writeMultiResponse(250, []string{
		fmt.Sprintf("%s Hello %s", c.server.GetHostname(), args),
		"AUTH LOGIN PLAIN",
		fmt.Sprintf("SIZE %d", MaxMessageSize),
		"8BITMIME",
		"ENHANCEDSTATUSCODES",
		"PIPELINING",
	})
	return true
}

// handleAUTH handles the AUTH command
func (c *Connection) handleAUTH(args string) bool {
	if c.state < StateGreeted {
		c.writeResponse(503, "EHLO/HELO first")
		return true
	}

	if c.authenticated {
		c.writeResponse(503, "Already authenticated")
		return true
	}

	auth := c.server.GetAuthenticator()
	if auth == nil {
		c.writeResponse(504, "Authentication not available")
		return true
	}

	parts := strings.SplitN(args, " ", 2)
	mechanism := strings.ToUpper(parts[0])

	switch mechanism {
	case "PLAIN":
		return c.handleAuthPlain(parts)
	case "LOGIN":
		return c.handleAuthLogin()
	default:
		c.writeResponse(504, "Authentication mechanism not supported")
		return true
	}
}

// handleAuthPlain handles PLAIN authentication
func (c *Connection) handleAuthPlain(parts []string) bool {
	var credentials string
	if len(parts) > 1 {
		credentials = parts[1]
	} else {
		// Request credentials
		c.writeResponse(334, "")
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return false
		}
		credentials = strings.TrimSpace(line)
	}

	// Decode base64 credentials
	decoded, err := decodeBase64(credentials)
	if err != nil {
		c.writeResponse(501, "Invalid base64 encoding")
		return true
	}

	// Parse PLAIN credentials: \0username\0password
	parts2 := strings.Split(string(decoded), "\x00")
	if len(parts2) < 3 {
		c.writeResponse(501, "Invalid credentials format")
		return true
	}

	username := parts2[1]
	password := parts2[2]

	return c.authenticate(username, password)
}

// handleAuthLogin handles LOGIN authentication
func (c *Connection) handleAuthLogin() bool {
	// Request username
	c.writeResponse(334, encodeBase64("Username:"))
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return false
	}
	usernameB64 := strings.TrimSpace(line)
	usernameBytes, err := decodeBase64(usernameB64)
	if err != nil {
		c.writeResponse(501, "Invalid base64 encoding")
		return true
	}
	username := string(usernameBytes)

	// Request password
	c.writeResponse(334, encodeBase64("Password:"))
	line, err = c.reader.ReadString('\n')
	if err != nil {
		return false
	}
	passwordB64 := strings.TrimSpace(line)
	passwordBytes, err := decodeBase64(passwordB64)
	if err != nil {
		c.writeResponse(501, "Invalid base64 encoding")
		return true
	}
	password := string(passwordBytes)

	return c.authenticate(username, password)
}

// authenticate validates credentials
func (c *Connection) authenticate(username, password string) bool {
	auth := c.server.GetAuthenticator()
	if auth == nil {
		c.writeResponse(454, "Authentication not available")
		return true
	}

	valid, err := auth.Authenticate(username, password)
	if err != nil {
		c.writeResponse(454, "Authentication error")
		return true
	}

	if !valid {
		c.writeResponse(535, "Authentication failed")
		return true
	}

	c.authenticated = true
	c.username = username
	c.state = StateAuthenticated
	c.writeResponse(235, "Authentication successful")
	return true
}

// handleMAIL handles the MAIL FROM command
func (c *Connection) handleMAIL(args string) bool {
	if c.state < StateGreeted {
		c.writeResponse(503, "EHLO/HELO first")
		return true
	}

	if !c.authenticated {
		c.writeResponse(530, "Authentication required")
		return true
	}

	args = strings.ToUpper(args)
	if !strings.HasPrefix(args, "FROM:") {
		c.writeResponse(501, "Syntax: MAIL FROM:<address>")
		return true
	}

	from := strings.TrimPrefix(args, "FROM:")
	from = strings.TrimSpace(from)
	from = extractAddress(from)

	if from == "" {
		c.writeResponse(501, "Invalid sender address")
		return true
	}

	c.mailFrom = from
	c.state = StateMailFrom
	c.writeResponse(250, "OK")
	return true
}

// handleRCPT handles the RCPT TO command
func (c *Connection) handleRCPT(args string) bool {
	if c.state < StateMailFrom {
		c.writeResponse(503, "MAIL FROM first")
		return true
	}

	args = strings.ToUpper(args)
	if !strings.HasPrefix(args, "TO:") {
		c.writeResponse(501, "Syntax: RCPT TO:<address>")
		return true
	}

	to := strings.TrimPrefix(args, "TO:")
	to = strings.TrimSpace(to)
	to = extractAddress(to)

	if to == "" {
		c.writeResponse(501, "Invalid recipient address")
		return true
	}

	// Check recipient limit
	if len(c.rcptTo) >= MaxRecipients {
		c.writeResponse(452, "Too many recipients")
		return true
	}

	// Validate it's a Freemail address
	if !strings.HasSuffix(strings.ToLower(to), ".freemail") {
		c.writeResponse(550, "Only Freemail addresses accepted")
		return true
	}

	c.rcptTo = append(c.rcptTo, to)
	c.state = StateRcptTo
	c.writeResponse(250, "OK")
	return true
}

// handleDATA handles the DATA command
func (c *Connection) handleDATA() bool {
	if c.state < StateRcptTo || len(c.rcptTo) == 0 {
		c.writeResponse(503, "RCPT TO first")
		return true
	}

	c.writeResponse(354, "End data with <CR><LF>.<CR><LF>")

	// Read message data
	var data []byte
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return false
		}

		// Check for end of data
		if line == ".\r\n" || line == ".\n" {
			break
		}

		// Handle dot-stuffing
		if strings.HasPrefix(line, "..") {
			line = line[1:]
		}

		// Check size limit
		if len(data)+len(line) > MaxMessageSize {
			c.writeResponse(552, "Message too large")
			c.reset()
			return true
		}

		data = append(data, []byte(line)...)
	}

	c.data = data

	// Submit message
	handler := c.server.GetHandler()
	if handler == nil {
		c.writeResponse(451, "Message handler not available")
		c.reset()
		return true
	}

	err := handler.HandleMessage(c.mailFrom, c.rcptTo, c.data)
	if err != nil {
		c.writeResponse(451, fmt.Sprintf("Message delivery failed: %v", err))
		c.reset()
		return true
	}

	c.writeResponse(250, "Message accepted for delivery")
	c.reset()
	return true
}

// handleRSET handles the RSET command
func (c *Connection) handleRSET() bool {
	c.reset()
	c.writeResponse(250, "OK")
	return true
}

// handleNOOP handles the NOOP command
func (c *Connection) handleNOOP() bool {
	c.writeResponse(250, "OK")
	return true
}

// handleQUIT handles the QUIT command
func (c *Connection) handleQUIT() bool {
	c.writeResponse(221, "Bye")
	return false
}

// handleVRFY handles the VRFY command
func (c *Connection) handleVRFY(args string) bool {
	c.writeResponse(252, "Cannot verify user")
	return true
}

// handleHELP handles the HELP command
func (c *Connection) handleHELP() bool {
	c.writeMultiResponse(214, []string{
		"Commands:",
		"HELO EHLO AUTH MAIL RCPT DATA",
		"RSET NOOP VRFY HELP QUIT",
	})
	return true
}

// reset resets the session state
func (c *Connection) reset() {
	c.mailFrom = ""
	c.rcptTo = make([]string, 0)
	c.data = nil
	if c.state > StateAuthenticated {
		c.state = StateAuthenticated
	} else if c.state > StateGreeted {
		c.state = StateGreeted
	}
}

// writeResponse writes an SMTP response
func (c *Connection) writeResponse(code int, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
	fmt.Fprintf(c.writer, "%d %s\r\n", code, message)
	c.writer.Flush()
}

// writeMultiResponse writes a multi-line SMTP response
func (c *Connection) writeMultiResponse(code int, messages []string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
	for i, msg := range messages {
		if i < len(messages)-1 {
			fmt.Fprintf(c.writer, "%d-%s\r\n", code, msg)
		} else {
			fmt.Fprintf(c.writer, "%d %s\r\n", code, msg)
		}
	}
	c.writer.Flush()
}

// Close closes the connection
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true
	return c.conn.Close()
}

// extractAddress extracts an email address from brackets
func extractAddress(s string) string {
	s = strings.TrimSpace(s)

	// Handle <address>
	if strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">") {
		s = s[1 : len(s)-1]
	}

	// Remove SIZE parameter if present
	if idx := strings.Index(s, " "); idx > 0 {
		s = s[:idx]
	}

	return strings.ToLower(s)
}
