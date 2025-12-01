// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

// Package imap implements an IMAP server for Freemail.
package imap

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/freemail"
)

// Server configuration
const (
	DefaultPort    = 3143
	ReadTimeout    = 30 * time.Minute
	WriteTimeout   = 5 * time.Minute
	MaxLineLength  = 8192
)

// Server represents an IMAP server
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

	// Account manager for authentication and mailboxes
	accountManager *freemail.AccountManager

	// Storage for message persistence
	storage *freemail.Storage

	// Active connections
	connections map[*Connection]struct{}

	// Shutdown
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// NewServer creates a new IMAP server
func NewServer(port int, hostname string) *Server {
	if hostname == "" {
		hostname = "localhost"
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

// SetAccountManager sets the account manager
func (s *Server) SetAccountManager(am *freemail.AccountManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accountManager = am
}

// SetStorage sets the storage
func (s *Server) SetStorage(storage *freemail.Storage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storage = storage
}

// Start starts the IMAP server
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

	log.Printf("IMAP server listening on %s", s.address)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// Stop stops the IMAP server
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
	log.Printf("IMAP server stopped")
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
				log.Printf("IMAP accept error: %v", err)
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

// GetAccountManager returns the account manager
func (s *Server) GetAccountManager() *freemail.AccountManager {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.accountManager
}

// GetStorage returns the storage
func (s *Server) GetStorage() *freemail.Storage {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.storage
}

// GetHostname returns the hostname
func (s *Server) GetHostname() string {
	return s.hostname
}

// Connection represents an IMAP client connection
type Connection struct {
	mu sync.Mutex

	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer
	server *Server

	// Session state
	state    SessionState
	account  *freemail.Account
	selected *freemail.Folder

	// Connection state
	closed bool
}

// SessionState represents the state of an IMAP session
type SessionState int

const (
	StateNotAuthenticated SessionState = iota
	StateAuthenticated
	StateSelected
	StateLogout
)

// NewConnection creates a new IMAP connection
func NewConnection(conn net.Conn, server *Server) *Connection {
	return &Connection{
		conn:   conn,
		reader: bufio.NewReader(conn),
		writer: bufio.NewWriter(conn),
		server: server,
		state:  StateNotAuthenticated,
	}
}

// Handle handles the IMAP session
func (c *Connection) Handle() {
	// Send greeting
	c.writeUntagged("OK", fmt.Sprintf("%s GoFreemail IMAP4rev1 ready", c.server.GetHostname()))

	for c.state != StateLogout {
		// Set read timeout
		c.conn.SetReadDeadline(time.Now().Add(ReadTimeout))

		// Read command
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}

		// Parse command
		tag, cmd, args := c.parseCommand(line)
		if tag == "" {
			c.writeUntagged("BAD", "Invalid command")
			continue
		}

		// Handle command
		c.handleCommand(tag, cmd, args)
	}
}

// parseCommand parses an IMAP command line
func (c *Connection) parseCommand(line string) (string, string, string) {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) < 2 {
		return "", "", ""
	}

	tag := parts[0]
	cmd := strings.ToUpper(parts[1])
	args := ""
	if len(parts) > 2 {
		args = parts[2]
	}

	return tag, cmd, args
}

// handleCommand dispatches to the appropriate command handler
func (c *Connection) handleCommand(tag, cmd, args string) {
	switch cmd {
	// Any state
	case "CAPABILITY":
		c.handleCapability(tag)
	case "NOOP":
		c.handleNoop(tag)
	case "LOGOUT":
		c.handleLogout(tag)

	// Not authenticated
	case "LOGIN":
		c.handleLogin(tag, args)

	// Authenticated state
	case "SELECT":
		c.handleSelect(tag, args, false)
	case "EXAMINE":
		c.handleSelect(tag, args, true)
	case "CREATE":
		c.handleCreate(tag, args)
	case "DELETE":
		c.handleDelete(tag, args)
	case "RENAME":
		c.handleRename(tag, args)
	case "SUBSCRIBE":
		c.handleSubscribe(tag, args)
	case "UNSUBSCRIBE":
		c.handleUnsubscribe(tag, args)
	case "LIST":
		c.handleList(tag, args)
	case "LSUB":
		c.handleLsub(tag, args)
	case "STATUS":
		c.handleStatus(tag, args)
	case "APPEND":
		c.handleAppend(tag, args)

	// Selected state
	case "CHECK":
		c.handleCheck(tag)
	case "CLOSE":
		c.handleClose(tag)
	case "EXPUNGE":
		c.handleExpunge(tag)
	case "SEARCH":
		c.handleSearch(tag, args, false)
	case "FETCH":
		c.handleFetch(tag, args, false)
	case "STORE":
		c.handleStore(tag, args, false)
	case "COPY":
		c.handleCopy(tag, args, false)
	case "UID":
		c.handleUID(tag, args)

	default:
		c.writeTagged(tag, "BAD", fmt.Sprintf("Unknown command: %s", cmd))
	}
}

// handleCapability handles the CAPABILITY command
func (c *Connection) handleCapability(tag string) {
	c.writeUntagged("CAPABILITY", "IMAP4rev1 AUTH=PLAIN AUTH=LOGIN")
	c.writeTagged(tag, "OK", "CAPABILITY completed")
}

// handleNoop handles the NOOP command
func (c *Connection) handleNoop(tag string) {
	// Check for new messages if mailbox is selected
	if c.state == StateSelected && c.selected != nil {
		c.writeUntagged("", fmt.Sprintf("%d EXISTS", c.selected.Count()))
	}
	c.writeTagged(tag, "OK", "NOOP completed")
}

// handleLogout handles the LOGOUT command
func (c *Connection) handleLogout(tag string) {
	c.writeUntagged("BYE", "GoFreemail IMAP server logging out")
	c.writeTagged(tag, "OK", "LOGOUT completed")
	c.state = StateLogout
}

// handleLogin handles the LOGIN command
func (c *Connection) handleLogin(tag, args string) {
	if c.state != StateNotAuthenticated {
		c.writeTagged(tag, "BAD", "Already authenticated")
		return
	}

	// Parse arguments: username password
	parts := parseArgs(args)
	if len(parts) < 2 {
		c.writeTagged(tag, "BAD", "LOGIN requires username and password")
		return
	}

	username := unquote(parts[0])
	password := unquote(parts[1])

	am := c.server.GetAccountManager()
	if am == nil {
		c.writeTagged(tag, "NO", "Authentication not available")
		return
	}

	account, valid := am.Authenticate(username, password)
	if !valid {
		c.writeTagged(tag, "NO", "LOGIN failed")
		return
	}

	c.account = account
	c.state = StateAuthenticated
	c.writeTagged(tag, "OK", "LOGIN completed")
}

// handleSelect handles the SELECT/EXAMINE command
func (c *Connection) handleSelect(tag, args string, readOnly bool) {
	if c.state < StateAuthenticated {
		c.writeTagged(tag, "NO", "Not authenticated")
		return
	}

	mailbox := unquote(args)
	if mailbox == "" {
		c.writeTagged(tag, "BAD", "SELECT requires mailbox name")
		return
	}

	folder := c.account.GetFolder(mailbox)
	if folder == nil {
		c.writeTagged(tag, "NO", "Mailbox does not exist")
		return
	}

	c.selected = folder
	c.state = StateSelected

	// Send mailbox info
	c.writeUntagged("", fmt.Sprintf("%d EXISTS", folder.Count()))
	c.writeUntagged("", fmt.Sprintf("%d RECENT", countRecent(folder)))
	c.writeUntagged("OK", fmt.Sprintf("[UIDVALIDITY %d] UIDs valid", folder.UIDValidity))
	c.writeUntagged("OK", fmt.Sprintf("[UIDNEXT %d] Predicted next UID", folder.NextUID))
	c.writeUntagged("FLAGS", "(\\Answered \\Flagged \\Deleted \\Seen \\Draft)")
	c.writeUntagged("OK", "[PERMANENTFLAGS (\\Answered \\Flagged \\Deleted \\Seen \\Draft \\*)] Flags permitted")

	if readOnly {
		c.writeTagged(tag, "OK", "[READ-ONLY] EXAMINE completed")
	} else {
		c.writeTagged(tag, "OK", "[READ-WRITE] SELECT completed")
	}
}

// handleCreate handles the CREATE command
func (c *Connection) handleCreate(tag, args string) {
	if c.state < StateAuthenticated {
		c.writeTagged(tag, "NO", "Not authenticated")
		return
	}

	mailbox := unquote(args)
	if mailbox == "" {
		c.writeTagged(tag, "BAD", "CREATE requires mailbox name")
		return
	}

	_, err := c.account.CreateFolder(mailbox)
	if err != nil {
		c.writeTagged(tag, "NO", fmt.Sprintf("CREATE failed: %v", err))
		return
	}

	c.writeTagged(tag, "OK", "CREATE completed")
}

// handleDelete handles the DELETE command
func (c *Connection) handleDelete(tag, args string) {
	if c.state < StateAuthenticated {
		c.writeTagged(tag, "NO", "Not authenticated")
		return
	}

	mailbox := unquote(args)
	if mailbox == "" {
		c.writeTagged(tag, "BAD", "DELETE requires mailbox name")
		return
	}

	err := c.account.DeleteFolder(mailbox)
	if err != nil {
		c.writeTagged(tag, "NO", fmt.Sprintf("DELETE failed: %v", err))
		return
	}

	c.writeTagged(tag, "OK", "DELETE completed")
}

// handleRename handles the RENAME command
func (c *Connection) handleRename(tag, args string) {
	if c.state < StateAuthenticated {
		c.writeTagged(tag, "NO", "Not authenticated")
		return
	}

	parts := parseArgs(args)
	if len(parts) < 2 {
		c.writeTagged(tag, "BAD", "RENAME requires old and new mailbox names")
		return
	}

	// Not implemented - would need to add rename support
	c.writeTagged(tag, "NO", "RENAME not supported")
}

// handleSubscribe handles the SUBSCRIBE command
func (c *Connection) handleSubscribe(tag, args string) {
	if c.state < StateAuthenticated {
		c.writeTagged(tag, "NO", "Not authenticated")
		return
	}

	// All folders are subscribed by default
	c.writeTagged(tag, "OK", "SUBSCRIBE completed")
}

// handleUnsubscribe handles the UNSUBSCRIBE command
func (c *Connection) handleUnsubscribe(tag, args string) {
	if c.state < StateAuthenticated {
		c.writeTagged(tag, "NO", "Not authenticated")
		return
	}

	c.writeTagged(tag, "OK", "UNSUBSCRIBE completed")
}

// handleList handles the LIST command
func (c *Connection) handleList(tag, args string) {
	if c.state < StateAuthenticated {
		c.writeTagged(tag, "NO", "Not authenticated")
		return
	}

	parts := parseArgs(args)
	if len(parts) < 2 {
		c.writeTagged(tag, "BAD", "LIST requires reference and mailbox")
		return
	}

	pattern := unquote(parts[1])
	if pattern == "" {
		// Return hierarchy delimiter
		c.writeUntagged("LIST", "(\\Noselect) \".\" \"\"")
	} else {
		// List matching mailboxes
		for _, name := range c.account.ListFolders() {
			if matchPattern(name, pattern) {
				c.writeUntagged("LIST", fmt.Sprintf("() \".\" \"%s\"", name))
			}
		}
	}

	c.writeTagged(tag, "OK", "LIST completed")
}

// handleLsub handles the LSUB command
func (c *Connection) handleLsub(tag, args string) {
	// Same as LIST for now
	c.handleList(tag, args)
}

// handleStatus handles the STATUS command
func (c *Connection) handleStatus(tag, args string) {
	if c.state < StateAuthenticated {
		c.writeTagged(tag, "NO", "Not authenticated")
		return
	}

	parts := parseArgs(args)
	if len(parts) < 2 {
		c.writeTagged(tag, "BAD", "STATUS requires mailbox and status items")
		return
	}

	mailbox := unquote(parts[0])
	folder := c.account.GetFolder(mailbox)
	if folder == nil {
		c.writeTagged(tag, "NO", "Mailbox does not exist")
		return
	}

	// Parse requested items
	items := strings.ToUpper(parts[1])
	var status []string

	if strings.Contains(items, "MESSAGES") {
		status = append(status, fmt.Sprintf("MESSAGES %d", folder.Count()))
	}
	if strings.Contains(items, "RECENT") {
		status = append(status, fmt.Sprintf("RECENT %d", countRecent(folder)))
	}
	if strings.Contains(items, "UIDNEXT") {
		status = append(status, fmt.Sprintf("UIDNEXT %d", folder.NextUID))
	}
	if strings.Contains(items, "UIDVALIDITY") {
		status = append(status, fmt.Sprintf("UIDVALIDITY %d", folder.UIDValidity))
	}
	if strings.Contains(items, "UNSEEN") {
		status = append(status, fmt.Sprintf("UNSEEN %d", countUnseen(folder)))
	}

	c.writeUntagged("STATUS", fmt.Sprintf("\"%s\" (%s)", mailbox, strings.Join(status, " ")))
	c.writeTagged(tag, "OK", "STATUS completed")
}

// handleAppend handles the APPEND command
func (c *Connection) handleAppend(tag, args string) {
	if c.state < StateAuthenticated {
		c.writeTagged(tag, "NO", "Not authenticated")
		return
	}

	// Parse: mailbox [flags] [date] literal
	parts := parseArgs(args)
	if len(parts) < 1 {
		c.writeTagged(tag, "BAD", "APPEND requires mailbox")
		return
	}

	mailbox := unquote(parts[0])
	folder := c.account.GetFolder(mailbox)
	if folder == nil {
		c.writeTagged(tag, "NO", "[TRYCREATE] Mailbox does not exist")
		return
	}

	// Find literal size
	literalSize := 0
	for _, part := range parts {
		if strings.HasPrefix(part, "{") && strings.HasSuffix(part, "}") {
			fmt.Sscanf(part, "{%d}", &literalSize)
			break
		}
	}

	if literalSize == 0 {
		c.writeTagged(tag, "BAD", "APPEND requires message literal")
		return
	}

	// Request literal
	c.writeContinuation("Ready for literal data")

	// Read literal
	data := make([]byte, literalSize)
	_, err := c.reader.Read(data)
	if err != nil {
		c.writeTagged(tag, "NO", "Failed to read message")
		return
	}

	// Create message
	msg := freemail.NewMessage()
	msg.Body = data
	msg.Size = int64(len(data))
	msg.Received = time.Now()

	// Add to folder
	folder.AddMessage(msg)

	c.writeTagged(tag, "OK", "APPEND completed")
}

// handleCheck handles the CHECK command
func (c *Connection) handleCheck(tag string) {
	if c.state != StateSelected {
		c.writeTagged(tag, "NO", "No mailbox selected")
		return
	}

	c.writeTagged(tag, "OK", "CHECK completed")
}

// handleClose handles the CLOSE command
func (c *Connection) handleClose(tag string) {
	if c.state != StateSelected {
		c.writeTagged(tag, "NO", "No mailbox selected")
		return
	}

	// Expunge deleted messages
	c.selected.Expunge()

	c.selected = nil
	c.state = StateAuthenticated
	c.writeTagged(tag, "OK", "CLOSE completed")
}

// handleExpunge handles the EXPUNGE command
func (c *Connection) handleExpunge(tag string) {
	if c.state != StateSelected {
		c.writeTagged(tag, "NO", "No mailbox selected")
		return
	}

	// Get message count before expunge
	count := c.selected.Count()
	expunged := c.selected.Expunge()

	// Report expunged messages
	for i, uid := range expunged {
		_ = uid // UID not needed for EXPUNGE response
		seq := count - len(expunged) + i + 1
		c.writeUntagged("", fmt.Sprintf("%d EXPUNGE", seq))
	}

	c.writeTagged(tag, "OK", "EXPUNGE completed")
}

// handleSearch handles the SEARCH command
func (c *Connection) handleSearch(tag, args string, useUID bool) {
	if c.state != StateSelected {
		c.writeTagged(tag, "NO", "No mailbox selected")
		return
	}

	// Simple search implementation - returns all messages for "ALL"
	args = strings.ToUpper(args)

	var results []string
	folder := c.selected

	for i, msg := range folder.Messages {
		seq := i + 1
		uid := msg.UID

		match := false

		if strings.Contains(args, "ALL") {
			match = true
		} else if strings.Contains(args, "UNSEEN") && !msg.HasFlag(freemail.FlagSeen) {
			match = true
		} else if strings.Contains(args, "SEEN") && msg.HasFlag(freemail.FlagSeen) {
			match = true
		} else if strings.Contains(args, "DELETED") && msg.HasFlag(freemail.FlagDeleted) {
			match = true
		} else if strings.Contains(args, "FLAGGED") && msg.HasFlag(freemail.FlagFlagged) {
			match = true
		}

		if match {
			if useUID {
				results = append(results, fmt.Sprintf("%d", uid))
			} else {
				results = append(results, fmt.Sprintf("%d", seq))
			}
		}
	}

	c.writeUntagged("SEARCH", strings.Join(results, " "))
	c.writeTagged(tag, "OK", "SEARCH completed")
}

// handleFetch handles the FETCH command
func (c *Connection) handleFetch(tag, args string, useUID bool) {
	if c.state != StateSelected {
		c.writeTagged(tag, "NO", "No mailbox selected")
		return
	}

	// Parse: sequence-set items
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		c.writeTagged(tag, "BAD", "FETCH requires sequence and items")
		return
	}

	seqSet := parts[0]
	items := strings.ToUpper(parts[1])

	// Parse sequence set
	sequences := parseSequenceSet(seqSet, c.selected.Count())

	for _, seq := range sequences {
		var msg *freemail.Message
		if useUID {
			msg = c.selected.GetMessage(uint32(seq))
		} else {
			msg = c.selected.GetMessageBySeq(seq)
		}

		if msg == nil {
			continue
		}

		response := c.buildFetchResponse(msg, seq, items, useUID)
		c.writeUntagged("", response)
	}

	c.writeTagged(tag, "OK", "FETCH completed")
}

// buildFetchResponse builds a FETCH response for a message
func (c *Connection) buildFetchResponse(msg *freemail.Message, seq int, items string, useUID bool) string {
	var parts []string

	if useUID || strings.Contains(items, "UID") {
		parts = append(parts, fmt.Sprintf("UID %d", msg.UID))
	}

	if strings.Contains(items, "FLAGS") {
		parts = append(parts, fmt.Sprintf("FLAGS (%s)", msg.Flags.String()))
	}

	if strings.Contains(items, "RFC822.SIZE") {
		parts = append(parts, fmt.Sprintf("RFC822.SIZE %d", msg.Size))
	}

	if strings.Contains(items, "INTERNALDATE") {
		parts = append(parts, fmt.Sprintf("INTERNALDATE \"%s\"", msg.Received.Format("02-Jan-2006 15:04:05 -0700")))
	}

	if strings.Contains(items, "ENVELOPE") {
		parts = append(parts, c.buildEnvelope(msg))
	}

	if strings.Contains(items, "BODY") || strings.Contains(items, "RFC822") {
		body := msg.Body
		if strings.Contains(items, "BODY.PEEK") || strings.Contains(items, "BODY[]") {
			parts = append(parts, fmt.Sprintf("BODY[] {%d}\r\n%s", len(body), body))
		} else if strings.Contains(items, "BODY[HEADER]") {
			// Return headers only
			headers := c.extractHeaders(msg)
			parts = append(parts, fmt.Sprintf("BODY[HEADER] {%d}\r\n%s", len(headers), headers))
		} else if strings.Contains(items, "BODY[TEXT]") {
			parts = append(parts, fmt.Sprintf("BODY[TEXT] {%d}\r\n%s", len(body), body))
		} else if strings.Contains(items, "RFC822") {
			parts = append(parts, fmt.Sprintf("RFC822 {%d}\r\n%s", len(body), body))
			// Mark as seen
			msg.SetFlag(freemail.FlagSeen)
		}
	}

	return fmt.Sprintf("%d FETCH (%s)", seq, strings.Join(parts, " "))
}

// buildEnvelope builds an ENVELOPE response
func (c *Connection) buildEnvelope(msg *freemail.Message) string {
	date := ""
	if !msg.Date.IsZero() {
		date = msg.Date.Format(time.RFC1123Z)
	}

	from := "NIL"
	if msg.From != nil {
		from = fmt.Sprintf("((\"%s\" NIL \"%s\" \"%s.freemail\"))", msg.From.Local, msg.From.Local, msg.From.Identity)
	}

	to := "NIL"
	if len(msg.To) > 0 {
		var addrs []string
		for _, addr := range msg.To {
			addrs = append(addrs, fmt.Sprintf("(\"%s\" NIL \"%s\" \"%s.freemail\")", addr.Local, addr.Local, addr.Identity))
		}
		to = "(" + strings.Join(addrs, " ") + ")"
	}

	return fmt.Sprintf("ENVELOPE (\"%s\" \"%s\" %s %s %s NIL NIL \"%s\" \"%s\")",
		date, msg.Subject, from, to, from, msg.InReplyTo, msg.MessageID)
}

// extractHeaders extracts headers from a message
func (c *Connection) extractHeaders(msg *freemail.Message) string {
	var buf strings.Builder
	for _, h := range msg.Headers {
		buf.WriteString(fmt.Sprintf("%s: %s\r\n", h.Name, h.Value))
	}
	buf.WriteString("\r\n")
	return buf.String()
}

// handleStore handles the STORE command
func (c *Connection) handleStore(tag, args string, useUID bool) {
	if c.state != StateSelected {
		c.writeTagged(tag, "NO", "No mailbox selected")
		return
	}

	// Parse: sequence-set flags-operation flags
	parts := strings.SplitN(args, " ", 3)
	if len(parts) < 3 {
		c.writeTagged(tag, "BAD", "STORE requires sequence, operation, and flags")
		return
	}

	seqSet := parts[0]
	operation := strings.ToUpper(parts[1])
	flagStr := parts[2]

	// Parse flags
	flags := freemail.ParseFlags(flagStr)

	// Parse sequence set
	sequences := parseSequenceSet(seqSet, c.selected.Count())

	for _, seq := range sequences {
		var msg *freemail.Message
		if useUID {
			msg = c.selected.GetMessage(uint32(seq))
		} else {
			msg = c.selected.GetMessageBySeq(seq)
		}

		if msg == nil {
			continue
		}

		// Apply operation
		switch {
		case strings.HasPrefix(operation, "+FLAGS"):
			msg.SetFlag(flags)
		case strings.HasPrefix(operation, "-FLAGS"):
			msg.ClearFlag(flags)
		case strings.HasPrefix(operation, "FLAGS"):
			msg.Flags = flags
		}

		// Report new flags unless .SILENT
		if !strings.Contains(operation, ".SILENT") {
			c.writeUntagged("", fmt.Sprintf("%d FETCH (FLAGS (%s))", seq, msg.Flags.String()))
		}
	}

	c.writeTagged(tag, "OK", "STORE completed")
}

// handleCopy handles the COPY command
func (c *Connection) handleCopy(tag, args string, useUID bool) {
	if c.state != StateSelected {
		c.writeTagged(tag, "NO", "No mailbox selected")
		return
	}

	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 2 {
		c.writeTagged(tag, "BAD", "COPY requires sequence and mailbox")
		return
	}

	seqSet := parts[0]
	destName := unquote(parts[1])

	dest := c.account.GetFolder(destName)
	if dest == nil {
		c.writeTagged(tag, "NO", "[TRYCREATE] Destination mailbox does not exist")
		return
	}

	sequences := parseSequenceSet(seqSet, c.selected.Count())

	for _, seq := range sequences {
		var msg *freemail.Message
		if useUID {
			msg = c.selected.GetMessage(uint32(seq))
		} else {
			msg = c.selected.GetMessageBySeq(seq)
		}

		if msg == nil {
			continue
		}

		// Copy message (shallow copy for now)
		copyMsg := *msg
		copyMsg.UID = 0 // Will be assigned by AddMessage
		dest.AddMessage(&copyMsg)
	}

	c.writeTagged(tag, "OK", "COPY completed")
}

// handleUID handles UID-prefixed commands
func (c *Connection) handleUID(tag, args string) {
	parts := strings.SplitN(args, " ", 2)
	if len(parts) < 1 {
		c.writeTagged(tag, "BAD", "UID requires command")
		return
	}

	cmd := strings.ToUpper(parts[0])
	cmdArgs := ""
	if len(parts) > 1 {
		cmdArgs = parts[1]
	}

	switch cmd {
	case "FETCH":
		c.handleFetch(tag, cmdArgs, true)
	case "STORE":
		c.handleStore(tag, cmdArgs, true)
	case "COPY":
		c.handleCopy(tag, cmdArgs, true)
	case "SEARCH":
		c.handleSearch(tag, cmdArgs, true)
	default:
		c.writeTagged(tag, "BAD", fmt.Sprintf("Unknown UID command: %s", cmd))
	}
}

// writeTagged writes a tagged response
func (c *Connection) writeTagged(tag, status, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
	fmt.Fprintf(c.writer, "%s %s %s\r\n", tag, status, message)
	c.writer.Flush()
}

// writeUntagged writes an untagged response
func (c *Connection) writeUntagged(status, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
	if status == "" {
		fmt.Fprintf(c.writer, "* %s\r\n", message)
	} else {
		fmt.Fprintf(c.writer, "* %s %s\r\n", status, message)
	}
	c.writer.Flush()
}

// writeContinuation writes a continuation response
func (c *Connection) writeContinuation(message string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
	fmt.Fprintf(c.writer, "+ %s\r\n", message)
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

// Helper functions

// parseArgs parses IMAP arguments handling quoted strings and parentheses
func parseArgs(args string) []string {
	var result []string
	var current strings.Builder
	inQuote := false
	inParen := 0

	for _, r := range args {
		switch {
		case r == '"' && inParen == 0:
			inQuote = !inQuote
			current.WriteRune(r)
		case r == '(' && !inQuote:
			inParen++
			current.WriteRune(r)
		case r == ')' && !inQuote:
			inParen--
			current.WriteRune(r)
		case r == ' ' && !inQuote && inParen == 0:
			if current.Len() > 0 {
				result = append(result, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		result = append(result, current.String())
	}

	return result
}

// unquote removes quotes from a string
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// matchPattern matches a mailbox name against an IMAP pattern
func matchPattern(name, pattern string) bool {
	if pattern == "*" || pattern == "%" {
		return true
	}
	// Simple matching - could be enhanced
	pattern = strings.ReplaceAll(pattern, "*", ".*")
	pattern = strings.ReplaceAll(pattern, "%", "[^.]*")
	return strings.Contains(strings.ToUpper(name), strings.ToUpper(strings.ReplaceAll(pattern, ".*", "")))
}

// parseSequenceSet parses an IMAP sequence set (e.g., "1:*", "1,3,5")
func parseSequenceSet(set string, max int) []int {
	var result []int

	for _, part := range strings.Split(set, ",") {
		if strings.Contains(part, ":") {
			// Range
			rangeParts := strings.Split(part, ":")
			start := parseSeqNum(rangeParts[0], max)
			end := parseSeqNum(rangeParts[1], max)
			for i := start; i <= end; i++ {
				result = append(result, i)
			}
		} else {
			// Single number
			num := parseSeqNum(part, max)
			if num > 0 {
				result = append(result, num)
			}
		}
	}

	return result
}

// parseSeqNum parses a sequence number or "*"
func parseSeqNum(s string, max int) int {
	s = strings.TrimSpace(s)
	if s == "*" {
		return max
	}
	var num int
	fmt.Sscanf(s, "%d", &num)
	return num
}

// countRecent counts recent messages in a folder
func countRecent(folder *freemail.Folder) int {
	count := 0
	for _, msg := range folder.Messages {
		if msg.HasFlag(freemail.FlagRecent) {
			count++
		}
	}
	return count
}

// countUnseen counts unseen messages in a folder
func countUnseen(folder *freemail.Folder) int {
	count := 0
	for _, msg := range folder.Messages {
		if !msg.HasFlag(freemail.FlagSeen) {
			count++
		}
	}
	return count
}
