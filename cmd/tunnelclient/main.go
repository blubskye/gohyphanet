// GoHyphanet - Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
)

var debugMode bool

func debugLog(format string, args ...interface{}) {
	if debugMode {
		log.Printf("[TUNNELCLIENT] "+format, args...)
	}
}

// TunnelRequest represents a request to send through Hyphanet
type TunnelRequest struct {
	ID      string            `json:"id"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body,omitempty"`
}

// TunnelResponse represents a response received from the tunnel server
type TunnelResponse struct {
	ID         string            `json:"id"`
	StatusCode int               `json:"status_code"`
	Status     string            `json:"status"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
	Error      string            `json:"error,omitempty"`
}

// TunnelClient handles local proxy and communication with tunnel server
type TunnelClient struct {
	serverKey      string
	fcpClient      *fcp.Client
	proxyAddr      string
	pendingReqs    map[string]chan *TunnelResponse
	mu             sync.RWMutex
	stats          *ClientStats
}

// ClientStats tracks usage statistics
type ClientStats struct {
	RequestsSent     int64
	ResponsesReceived int64
	BytesSent        int64
	BytesReceived    int64
	Errors           int64
	mu               sync.Mutex
}

// NewTunnelClient creates a new tunnel client
func NewTunnelClient(serverKey string, fcpClient *fcp.Client, proxyAddr string) *TunnelClient {
	return &TunnelClient{
		serverKey:   serverKey,
		fcpClient:   fcpClient,
		proxyAddr:   proxyAddr,
		pendingReqs: make(map[string]chan *TunnelResponse),
		stats:       &ClientStats{},
	}
}

// Start starts the tunnel client
func (tc *TunnelClient) Start() error {
	log.Printf("Tunnel Client Started")
	log.Printf("Server Key: %s", tc.serverKey)
	log.Printf("Proxy Address: %s", tc.proxyAddr)
	log.Println()

	// Start response polling loop
	go tc.pollResponses()

	// Start proxy server
	return tc.startProxyServer()
}

// startProxyServer starts the local HTTP proxy
func (tc *TunnelClient) startProxyServer() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", tc.handleProxyRequest)

	server := &http.Server{
		Addr:         tc.proxyAddr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	log.Printf("Proxy server listening on: http://%s", tc.proxyAddr)
	log.Println()
	log.Println("Configure your browser to use this HTTP proxy:")
	log.Printf("  HTTP Proxy: %s\n", strings.Split(tc.proxyAddr, ":")[0])
	log.Printf("  Port: %s\n", strings.Split(tc.proxyAddr, ":")[1])
	log.Println()

	return server.ListenAndServe()
}

// handleProxyRequest handles incoming proxy requests from the browser
func (tc *TunnelClient) handleProxyRequest(w http.ResponseWriter, r *http.Request) {
	debugLog("Proxy request: %s %s", r.Method, r.URL.String())

	// Generate request ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	requestID := hex.EncodeToString(idBytes)

	// Build full URL
	targetURL := r.URL.String()
	if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
		// Reconstruct URL from request
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		targetURL = fmt.Sprintf("%s://%s%s", scheme, r.Host, r.RequestURI)
	}

	// Build tunnel request
	headers := make(map[string]string)
	for key, values := range r.Header {
		if len(values) > 0 {
			// Skip proxy-specific headers
			if key == "Proxy-Connection" || key == "Proxy-Authorization" {
				continue
			}
			headers[key] = values[0]
		}
	}

	tunnelReq := &TunnelRequest{
		ID:      requestID,
		Method:  r.Method,
		URL:     targetURL,
		Headers: headers,
	}

	// Read body if present
	if r.Body != nil {
		body, err := io.ReadAll(r.Body)
		if err == nil && len(body) > 0 {
			tunnelReq.Body = body
		}
	}

	// Create response channel
	respChan := make(chan *TunnelResponse, 1)
	tc.mu.Lock()
	tc.pendingReqs[requestID] = respChan
	tc.mu.Unlock()

	// Send request through Hyphanet
	debugLog("Sending request through Hyphanet: %s", requestID)
	if err := tc.sendRequest(tunnelReq); err != nil {
		debugLog("Failed to send request: %v", err)
		tc.stats.mu.Lock()
		tc.stats.Errors++
		tc.stats.mu.Unlock()
		http.Error(w, "Failed to send request through Hyphanet", http.StatusBadGateway)
		return
	}

	tc.stats.mu.Lock()
	tc.stats.RequestsSent++
	tc.stats.BytesSent += int64(len(tunnelReq.Body))
	tc.stats.mu.Unlock()

	// Wait for response (with timeout)
	select {
	case resp := <-respChan:
		debugLog("Received response for: %s (%d %s)", requestID, resp.StatusCode, resp.Status)

		tc.stats.mu.Lock()
		tc.stats.ResponsesReceived++
		tc.stats.BytesReceived += int64(len(resp.Body))
		tc.stats.mu.Unlock()

		// Handle error response
		if resp.Error != "" {
			http.Error(w, resp.Error, resp.StatusCode)
			return
		}

		// Set response headers
		for key, value := range resp.Headers {
			w.Header().Set(key, value)
		}

		// Write response
		w.WriteHeader(resp.StatusCode)
		w.Write(resp.Body)

	case <-time.After(2 * time.Minute):
		debugLog("Request timeout: %s", requestID)
		tc.stats.mu.Lock()
		tc.stats.Errors++
		tc.stats.mu.Unlock()
		
		// Clean up
		tc.mu.Lock()
		delete(tc.pendingReqs, requestID)
		tc.mu.Unlock()

		http.Error(w, "Request timeout", http.StatusGatewayTimeout)
	}
}

// sendRequest sends a request through Hyphanet to the tunnel server
func (tc *TunnelClient) sendRequest(req *TunnelRequest) error {
	// Serialize request
	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Publish to tunnel server's request queue
	// URI format: SSK@<server-key>/requests/<request-id>
	uri := fmt.Sprintf("%s/requests/%s", tc.serverKey, req.ID)

	ops := fcp.NewOperations(tc.fcpClient)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, err := ops.Put(ctx, uri, data)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("request send failed: %s", result.Error)
	}

	debugLog("Request sent to Hyphanet")
	return nil
}

// pollResponses polls for responses from the tunnel server
func (tc *TunnelClient) pollResponses() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		tc.mu.RLock()
		requestIDs := make([]string, 0, len(tc.pendingReqs))
		for id := range tc.pendingReqs {
			requestIDs = append(requestIDs, id)
		}
		tc.mu.RUnlock()

		for _, requestID := range requestIDs {
			tc.checkForResponse(requestID)
		}
	}
}

// checkForResponse checks if a response is available for a request
func (tc *TunnelClient) checkForResponse(requestID string) {
	// Try to fetch response from Hyphanet
	uri := fmt.Sprintf("%s/response-%s", tc.serverKey, requestID)

	ops := fcp.NewOperations(tc.fcpClient)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := ops.Get(ctx, uri)
	if err != nil {
		// Response not available yet (expected)
		return
	}

	if !result.Success {
		// Response not ready
		return
	}

	// Parse response
	var resp TunnelResponse
	if err := json.Unmarshal(result.Data, &resp); err != nil {
		debugLog("Failed to parse response: %v", err)
		return
	}

	// Send to waiting handler
	tc.mu.RLock()
	respChan, exists := tc.pendingReqs[requestID]
	tc.mu.RUnlock()

	if exists {
		select {
		case respChan <- &resp:
			debugLog("Delivered response for: %s", requestID)
		default:
			// Channel full or closed
		}

		// Clean up
		tc.mu.Lock()
		delete(tc.pendingReqs, requestID)
		tc.mu.Unlock()
	}
}

// GetStats returns current statistics
func (tc *TunnelClient) GetStats() map[string]interface{} {
	tc.stats.mu.Lock()
	defer tc.stats.mu.Unlock()

	return map[string]interface{}{
		"requests_sent":      tc.stats.RequestsSent,
		"responses_received": tc.stats.ResponsesReceived,
		"bytes_sent":         tc.stats.BytesSent,
		"bytes_received":     tc.stats.BytesReceived,
		"errors":             tc.stats.Errors,
		"pending_requests":   len(tc.pendingReqs),
	}
}

func main() {
	serverKey := flag.String("server", "", "Tunnel server public key (required)")
	proxyAddr := flag.String("proxy", "127.0.0.1:8890", "Local proxy address")
	fcpHost := flag.String("fcp-host", "localhost", "FCP host")
	fcpPort := flag.Int("fcp-port", 9481, "FCP port")
	debug := flag.Bool("debug", false, "Enable debug logging")
	verbose := flag.Bool("v", false, "Verbose output")
	showVersion := flag.Bool("version", false, "Show version information")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "tunnelclient - Local Proxy Client for Hyphanet Tunnel v%s\n\n", fcp.Version)
		fmt.Fprintf(os.Stderr, "Connect to a tunnel server through Hyphanet to access the clearnet anonymously.\n\n")
		fmt.Fprintf(os.Stderr, "Usage: tunnelclient -server <server_public_key> [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  tunnelclient -server SSK@abc123.../\n\n")
		fmt.Fprintf(os.Stderr, "Then configure your browser:\n")
		fmt.Fprintf(os.Stderr, "  HTTP Proxy: 127.0.0.1\n")
		fmt.Fprintf(os.Stderr, "  Port: 8890\n\n")
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

	if *serverKey == "" {
		fmt.Fprintf(os.Stderr, "Error: -server flag is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	log.Println("Hyphanet Tunnel Client Starting...")
	log.Println()

	// Connect to Hyphanet
	config := &fcp.Config{
		Host:    *fcpHost,
		Port:    *fcpPort,
		Name:    "tunnelclient",
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
	log.Println()

	// Create tunnel client
	tunnelClient := NewTunnelClient(*serverKey, client, *proxyAddr)

	// Start statistics display
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			stats := tunnelClient.GetStats()
			log.Printf("Stats: Requests=%d, Responses=%d, Pending=%d, Errors=%d",
				stats["requests_sent"],
				stats["responses_received"],
				stats["pending_requests"],
				stats["errors"])
		}
	}()

	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("Starting Tunnel Client...")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println()

	// Start tunnel client
	if err := tunnelClient.Start(); err != nil {
		log.Fatalf("Failed to start tunnel client: %v", err)
	}
}
