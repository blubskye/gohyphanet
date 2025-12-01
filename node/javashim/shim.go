// GoHyphanet - Java Shim Wrapper
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package javashim

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os/exec"
	"sync"
)

// Shim represents a connection to the Java handshake shim
type Shim struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser
	reader *bufio.Reader
	mu     sync.Mutex
	debug  bool
}

// Request represents a request to the Java shim
type Request struct {
	Command string                 `json:"command"`
	Params  map[string]interface{} `json:"params"`
}

// Response represents a response from the Java shim
type Response struct {
	Success bool                   `json:"success"`
	Error   string                 `json:"error,omitempty"`
	Data    map[string]interface{} `json:"data,omitempty"`
}

// HandshakeResult represents the result of a handshake operation
type HandshakeResult struct {
	Success        bool
	Message        string
	ResponseLength int
	RemoteAddress  string
	RemotePort     int
}

// NewShim creates and starts a new Java shim process
func NewShim(jarPath string, debug bool) (*Shim, error) {
	cmd := exec.Command("java", "-jar", jarPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start Java shim: %w", err)
	}

	shim := &Shim{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		reader: bufio.NewReader(stdout),
		debug:  debug,
	}

	// Start stderr reader in background
	go shim.readStderr()

	if debug {
		log.Printf("[SHIM] Java shim started (PID %d)", cmd.Process.Pid)
	}

	// Test connection with ping
	if err := shim.Ping(); err != nil {
		shim.Close()
		return nil, fmt.Errorf("failed to ping shim: %w", err)
	}

	return shim, nil
}

// readStderr reads and logs stderr from the Java process
func (s *Shim) readStderr() {
	scanner := bufio.NewScanner(s.stderr)
	for scanner.Scan() {
		if s.debug {
			log.Printf("[SHIM STDERR] %s", scanner.Text())
		}
	}
}

// sendRequest sends a request to the Java shim
func (s *Shim) sendRequest(req *Request) (*Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Encode request
	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to encode request: %w", err)
	}

	if s.debug {
		log.Printf("[SHIM] Sending: %s", string(data))
	}

	// Send request
	if _, err := s.stdin.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("failed to write request: %w", err)
	}

	// Read response
	line, err := s.reader.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if s.debug {
		log.Printf("[SHIM] Received: %s", line)
	}

	// Decode response
	var resp Response
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &resp, nil
}

// Ping tests the connection to the Java shim
func (s *Shim) Ping() error {
	req := &Request{
		Command: "PING",
		Params:  make(map[string]interface{}),
	}

	resp, err := s.sendRequest(req)
	if err != nil {
		return fmt.Errorf("ping failed: %w", err)
	}

	if !resp.Success {
		return fmt.Errorf("ping returned error: %s", resp.Error)
	}

	return nil
}

// Handshake performs a handshake with a seed node
func (s *Shim) Handshake(host string, port int) (*HandshakeResult, error) {
	return s.HandshakeWithIdentity(host, port, "")
}

// HandshakeWithIdentity performs a handshake with a seed node using the seed's identity
func (s *Shim) HandshakeWithIdentity(host string, port int, seedIdentity string) (*HandshakeResult, error) {
	req := &Request{
		Command: "HANDSHAKE",
		Params: map[string]interface{}{
			"host": host,
			"port": float64(port), // JSON numbers are floats
		},
	}

	if seedIdentity != "" {
		req.Params["seedIdentity"] = seedIdentity
	}

	resp, err := s.sendRequest(req)
	if err != nil {
		return nil, fmt.Errorf("handshake request failed: %w", err)
	}

	result := &HandshakeResult{
		Success: resp.Success,
	}

	if resp.Success {
		if msg, ok := resp.Data["message"].(string); ok {
			result.Message = msg
		}
		if length, ok := resp.Data["responseLength"].(float64); ok {
			result.ResponseLength = int(length)
		}
		if addr, ok := resp.Data["remoteAddress"].(string); ok {
			result.RemoteAddress = addr
		}
		if port, ok := resp.Data["remotePort"].(float64); ok {
			result.RemotePort = int(port)
		}
	} else {
		result.Message = resp.Error
	}

	return result, nil
}

// Close shuts down the Java shim
func (s *Shim) Close() error {
	if s.debug {
		log.Printf("[SHIM] Shutting down Java shim")
	}

	// Send EXIT command
	s.stdin.Write([]byte("EXIT\n"))

	// Close pipes
	s.stdin.Close()
	s.stdout.Close()
	s.stderr.Close()

	// Wait for process to exit
	if err := s.cmd.Wait(); err != nil {
		return fmt.Errorf("shim exit error: %w", err)
	}

	return nil
}
