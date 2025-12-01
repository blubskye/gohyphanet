package fcp_server

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// FCPConnection represents a single FCP client connection
type FCPConnection struct {
	mu sync.RWMutex

	// Connection info
	id         string
	conn       net.Conn
	reader     *bufio.Reader
	writer     *bufio.Writer
	server     *FCPServer
	clientName string

	// State
	authenticated bool
	closed        bool
	lastActivity  time.Time

	// Active requests
	activeRequests map[string]*FCPRequest
}

// NewFCPConnection creates a new FCP connection handler
func NewFCPConnection(id string, conn net.Conn, server *FCPServer) *FCPConnection {
	return &FCPConnection{
		id:             id,
		conn:           conn,
		reader:         bufio.NewReader(conn),
		writer:         bufio.NewWriter(conn),
		server:         server,
		lastActivity:   time.Now(),
		activeRequests: make(map[string]*FCPRequest),
	}
}

// Handle processes FCP messages from the client
func (c *FCPConnection) Handle() {
	defer c.Close()

	// First message MUST be ClientHello
	msg, err := c.ReceiveMessage()
	if err != nil {
		log.Printf("[%s] Failed to receive ClientHello: %v", c.id, err)
		return
	}

	if msg.Name != "ClientHello" {
		c.SendProtocolError(
			ProtocolErrorClientHelloMustBeFirst,
			"First message must be ClientHello",
			"",
			false,
		)
		return
	}

	// Handle ClientHello
	if err := c.handleClientHello(msg); err != nil {
		log.Printf("[%s] ClientHello failed: %v", c.id, err)
		return
	}

	log.Printf("[%s] Client authenticated: %s", c.id, c.clientName)

	// Main message loop
	for {
		msg, err := c.ReceiveMessage()
		if err != nil {
			log.Printf("[%s] Connection closed: %v", c.id, err)
			return
		}

		// Update activity time
		c.mu.Lock()
		c.lastActivity = time.Now()
		c.mu.Unlock()

		// Dispatch message
		if err := c.handleMessage(msg); err != nil {
			log.Printf("[%s] Message handling error: %v", c.id, err)
			// Continue processing other messages
		}
	}
}

// ReceiveMessage reads an FCP message from the connection
func (c *FCPConnection) ReceiveMessage() (*FCPMessage, error) {
	msg := &FCPMessage{
		Fields: make(map[string]string),
	}

	// Read message name
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	msg.Name = strings.TrimSpace(line)

	// Read fields
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		line = strings.TrimSpace(line)

		// Check for end marker
		if line == "EndMessage" {
			break
		}

		// Check for data section
		if line == "Data" {
			dataLengthStr, ok := msg.Fields["DataLength"]
			if !ok {
				return nil, fmt.Errorf("Data section without DataLength")
			}

			dataLength, err := strconv.ParseInt(dataLengthStr, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid DataLength: %w", err)
			}

			// Read data
			msg.Data = make([]byte, dataLength)
			_, err = c.reader.Read(msg.Data)
			if err != nil {
				return nil, fmt.Errorf("failed to read data: %w", err)
			}

			// Read EndMessage after data
			line, err = c.reader.ReadString('\n')
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(line) != "EndMessage" {
				return nil, fmt.Errorf("expected EndMessage after data")
			}
			break
		}

		// Parse field
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			msg.Fields[parts[0]] = parts[1]
		}
	}

	return msg, nil
}

// SendMessage sends an FCP message to the client
func (c *FCPConnection) SendMessage(msg *FCPMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("connection closed")
	}

	// Write message name
	if _, err := c.writer.WriteString(msg.Name + "\n"); err != nil {
		return err
	}

	// Write fields
	for key, value := range msg.Fields {
		line := fmt.Sprintf("%s=%s\n", key, value)
		if _, err := c.writer.WriteString(line); err != nil {
			return err
		}
	}

	// Write data if present
	if len(msg.Data) > 0 {
		// Add DataLength field
		dataLenLine := fmt.Sprintf("DataLength=%d\n", len(msg.Data))
		if _, err := c.writer.WriteString(dataLenLine); err != nil {
			return err
		}

		// Data marker
		if _, err := c.writer.WriteString("Data\n"); err != nil {
			return err
		}

		// Write data
		if _, err := c.writer.Write(msg.Data); err != nil {
			return err
		}
	}

	// End marker
	if _, err := c.writer.WriteString("EndMessage\n"); err != nil {
		return err
	}

	return c.writer.Flush()
}

// Close closes the FCP connection
func (c *FCPConnection) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return
	}

	c.closed = true
	c.conn.Close()

	// Cancel all active requests
	for _, req := range c.activeRequests {
		req.Cancel()
	}
}

// handleClientHello processes the ClientHello message
func (c *FCPConnection) handleClientHello(msg *FCPMessage) error {
	name, ok := msg.Fields["Name"]
	if !ok {
		return c.SendProtocolError(
			ProtocolErrorMissingField,
			"ClientHello must contain a Name field",
			"",
			false,
		)
	}

	expectedVersion, ok := msg.Fields["ExpectedVersion"]
	if !ok {
		return c.SendProtocolError(
			ProtocolErrorMissingField,
			"ClientHello must contain an ExpectedVersion field",
			"",
			false,
		)
	}

	// Check version compatibility (simplified)
	if !strings.HasPrefix(expectedVersion, "2.") {
		log.Printf("[%s] Warning: Client expects version %s, we support %s",
			c.id, expectedVersion, ProtocolVersion)
	}

	c.clientName = name
	c.authenticated = true

	// Send NodeHello response
	return c.SendMessage(&FCPMessage{
		Name: "NodeHello",
		Fields: map[string]string{
			"ConnectionIdentifier": c.id,
			"Version":              ProtocolVersion,
			"Build":                "GoHyphanet-0.1",
			"Testnet":              "false",
			"Node":                 "GoHyphanet",
		},
	})
}

// handleMessage dispatches a message to the appropriate handler
func (c *FCPConnection) handleMessage(msg *FCPMessage) error {
	switch msg.Name {
	case "ClientGet":
		return c.handleClientGet(msg)
	case "ClientPut":
		return c.handleClientPut(msg)
	case "GenerateSSK":
		return c.handleGenerateSSK(msg)
	case "GetNode":
		return c.handleGetNode(msg)
	case "Disconnect":
		return c.handleDisconnect(msg)
	case "RemoveRequest":
		return c.handleRemoveRequest(msg)
	default:
		log.Printf("[%s] Unsupported message: %s", c.id, msg.Name)
		return c.SendProtocolError(
			ProtocolErrorMessageParseError,
			fmt.Sprintf("Unsupported message: %s", msg.Name),
			msg.Fields["Identifier"],
			msg.Fields["Global"] == "true",
		)
	}
}

// SendProtocolError sends a protocol error message
func (c *FCPConnection) SendProtocolError(code int, description string, identifier string, fatal bool) error {
	msg := &FCPMessage{
		Name: "ProtocolError",
		Fields: map[string]string{
			"Code":            strconv.Itoa(code),
			"CodeDescription": GetProtocolErrorDescription(code),
			"Fatal":           strconv.FormatBool(fatal),
		},
	}

	if description != "" {
		msg.Fields["ExtraDescription"] = description
	}

	if identifier != "" {
		msg.Fields["Identifier"] = identifier
	}

	err := c.SendMessage(msg)

	if fatal {
		c.Close()
	}

	return err
}

// GetID returns the connection ID
func (c *FCPConnection) GetID() string {
	return c.id
}

// GetClientName returns the client name
func (c *FCPConnection) GetClientName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.clientName
}
