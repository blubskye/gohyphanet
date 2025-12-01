// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package fcp

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
)

var DebugMode bool

func init() {
	// Check for DEBUG environment variable
	if os.Getenv("FCP_DEBUG") != "" {
		DebugMode = true
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
	}
}

func debugLog(format string, args ...interface{}) {
	if DebugMode {
		log.Printf("[FCP] "+format, args...)
	}
}

// Message represents an FCP message with fields and optional payload
type Message struct {
	Name   string
	Fields map[string]string
	Data   []byte
}

// Client represents a connection to a Freenet node
type Client struct {
	conn             net.Conn
	reader           *bufio.Reader
	writer           *bufio.Writer
	mu               sync.Mutex
	name             string
	handlers         map[string]MessageHandler
	progressHandlers map[string]func(int, int)
	progressLock     sync.RWMutex
}

// MessageHandler is a function that processes incoming messages
type MessageHandler func(*Message) error

// Config holds configuration for connecting to Freenet node
type Config struct {
	Host    string
	Port    int
	Name    string
	Version string
}

// DefaultConfig returns a config with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Host:    "localhost",
		Port:    9481,
		Name:    "GoFreenet",
		Version: "2.0",
	}
}

// Connect establishes a connection to the Freenet node
func Connect(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	debugLog("Connecting to %s...", addr)
	
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}
	debugLog("TCP connection established")

	client := &Client{
		conn:             conn,
		reader:           bufio.NewReader(conn),
		writer:           bufio.NewWriter(conn),
		name:             config.Name,
		handlers:         make(map[string]MessageHandler),
		progressHandlers: make(map[string]func(int, int)),
	}

	// Send ClientHello
	debugLog("Sending ClientHello...")
	hello := &Message{
		Name: "ClientHello",
		Fields: map[string]string{
			"Name":            config.Name,
			"ExpectedVersion": config.Version,
		},
	}

	if err := client.SendMessage(hello); err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to send ClientHello: %w", err)
	}

	// Wait for NodeHello
	debugLog("Waiting for NodeHello...")
	response, err := client.ReceiveMessage()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to receive NodeHello: %w", err)
	}

	if response.Name != "NodeHello" {
		conn.Close()
		return nil, fmt.Errorf("unexpected response: %s", response.Name)
	}

	debugLog("NodeHello received, connection established")
	if DebugMode {
		debugLog("Node version: %s", response.Fields["Version"])
		debugLog("Node build: %s", response.Fields["Build"])
	}

	return client, nil
}

// SendMessage sends an FCP message to the node
func (c *Client) SendMessage(msg *Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	debugLog(">>> Sending: %s (identifier: %s)", msg.Name, msg.Fields["Identifier"])
	if DebugMode {
		for k, v := range msg.Fields {
			if k != "Identifier" {
				debugLog("    %s=%s", k, v)
			}
		}
		if len(msg.Data) > 0 {
			debugLog("    DataLength=%d", len(msg.Data))
		}
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

	// If there's data, include DataLength field
	if len(msg.Data) > 0 {
		dataLenLine := fmt.Sprintf("DataLength=%d\n", len(msg.Data))
		if _, err := c.writer.WriteString(dataLenLine); err != nil {
			return err
		}
		if _, err := c.writer.WriteString("Data\n"); err != nil {
			return err
		}
		if _, err := c.writer.Write(msg.Data); err != nil {
			return err
		}
		// End message marker must still be sent
		if _, err := c.writer.WriteString("EndMessage\n"); err != nil {
			return err
		}
	} else {
		// End message marker
		if _, err := c.writer.WriteString("EndMessage\n"); err != nil {
			return err
		}
	}

	if err := c.writer.Flush(); err != nil {
		return err
	}
	
	debugLog("Message sent successfully")
	return nil
}

// ReceiveMessage reads an FCP message from the node
func (c *Client) ReceiveMessage() (*Message, error) {
	msg := &Message{
		Fields: make(map[string]string),
	}

	// Read message name
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	msg.Name = strings.TrimSpace(line)
	
	debugLog("<<< Received: %s", msg.Name)

	// Read fields
	for {
		line, err := c.reader.ReadString('\n')
		if err != nil {
			return nil, err
		}

		line = strings.TrimSpace(line)

		// Check for end of message
		if line == "EndMessage" {
			break
		}

		// Check for data section
		if line == "Data" {
			dataLengthStr, ok := msg.Fields["DataLength"]
			if !ok {
				return nil, fmt.Errorf("Data section without DataLength field")
			}

			dataLength, err := strconv.ParseInt(dataLengthStr, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid DataLength: %w", err)
			}

			debugLog("    Reading %d bytes of data...", dataLength)
			msg.Data = make([]byte, dataLength)
			if _, err := io.ReadFull(c.reader, msg.Data); err != nil {
				return nil, fmt.Errorf("failed to read data: %w", err)
			}
			debugLog("    Data read successfully")
			
			// We must consume the final EndMessage line
			line, err := c.reader.ReadString('\n')
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(line) != "EndMessage" {
				return nil, fmt.Errorf("expected EndMessage after data, got: %s", line)
			}
			break
		}

		// Parse field
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue // Skip malformed lines
		}
		msg.Fields[parts[0]] = parts[1]
		
		if DebugMode && parts[0] != "Data" {
			// Log important fields, but not all to avoid spam
			if parts[0] == "Identifier" || parts[0] == "Code" || parts[0] == "CodeDescription" || 
			   parts[0] == "Succeeded" || parts[0] == "Total" || parts[0] == "URI" {
				debugLog("    %s=%s", parts[0], parts[1])
			}
		}
	}

	return msg, nil
}

// RegisterHandler registers a handler for a specific message type
func (c *Client) RegisterHandler(msgType string, handler MessageHandler) {
	debugLog("Registering handler for: %s", msgType)
	c.handlers[msgType] = handler
}

// Listen starts listening for incoming messages and dispatches to handlers
func (c *Client) Listen() error {
	debugLog("Listen loop started")
	messageCount := 0
	
	for {
		msg, err := c.ReceiveMessage()
		if err != nil {
			if err == io.EOF {
				debugLog("Listen loop ended: EOF")
				return nil
			}
			debugLog("Listen loop error: %v", err)
			return err
		}

		messageCount++
		if messageCount%10 == 0 {
			debugLog("Processed %d messages so far", messageCount)
		}

		// Dispatch to handler if registered
		if handler, ok := c.handlers[msg.Name]; ok {
			debugLog("Dispatching %s to handler", msg.Name)
			if err := handler(msg); err != nil {
				debugLog("Handler error for %s: %v", msg.Name, err)
				return fmt.Errorf("handler error for %s: %w", msg.Name, err)
			}
		} else {
			debugLog("No handler registered for: %s", msg.Name)
		}
	}
}

// SendRawData sends raw bytes to the node (used for ClientPutComplexDir file data)
func (c *Client) SendRawData(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	debugLog("Sending %d bytes of raw data", len(data))
	_, err := c.writer.Write(data)
	if err != nil {
		return err
	}
	return c.writer.Flush()
}

// Close closes the connection to the Freenet node
func (c *Client) Close() error {
	debugLog("Closing connection")
	return c.conn.Close()
}

// ClientGet retrieves data from Freenet
func (c *Client) ClientGet(uri string, identifier string) error {
	msg := &Message{
		Name: "ClientGet",
		Fields: map[string]string{
			"URI":        uri,
			"Identifier": identifier,
		},
	}
	return c.SendMessage(msg)
}

// ClientPut inserts data into Freenet
func (c *Client) ClientPut(uri string, identifier string, data []byte) error {
	msg := &Message{
		Name: "ClientPut",
		Fields: map[string]string{
			"URI":        uri,
			"Identifier": identifier,
			"UploadFrom": "direct",
		},
		Data: data,
	}
	return c.SendMessage(msg)
}

// GetNode returns information about the node
func (c *Client) GetNode(withPrivate, withVolatile bool) error {
	msg := &Message{
		Name: "GetNode",
		Fields: map[string]string{
			"WithPrivate":  strconv.FormatBool(withPrivate),
			"WithVolatile": strconv.FormatBool(withVolatile),
		},
	}
	return c.SendMessage(msg)
}

// String returns a string representation of a Message
func (m *Message) String() string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("Message: %s\n", m.Name))
	for k, v := range m.Fields {
		buf.WriteString(fmt.Sprintf("  %s: %s\n", k, v))
	}
	if len(m.Data) > 0 {
		buf.WriteString(fmt.Sprintf("  Data: %d bytes\n", len(m.Data)))
	}
	return buf.String()
}
