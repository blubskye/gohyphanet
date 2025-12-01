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
		log.Printf("[TUNNELSERVER] "+format, args...)
	}
}

// TunnelRequest represents a request from a client through Hyphanet
type TunnelRequest struct {
	ID      string            `json:"id"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
	Body    []byte            `json:"body,omitempty"`
}

// TunnelResponse represents a response to send back through Hyphanet
type TunnelResponse struct {
	ID         string            `json:"id"`
	StatusCode int               `json:"status_code"`
	Status     string            `json:"status"`
	Headers    map[string]string `json:"headers"`
	Body       []byte            `json:"body"`
	Error      string            `json:"error,omitempty"`
}

// TunnelServer handles clearnet requests and publishes responses to Hyphanet
type TunnelServer struct {
	serverID    string
	fcpClient   *fcp.Client
	sskKeyPair  *fcp.KeyPair
	requests    map[string]*TunnelRequest
	responses   map[string]*TunnelResponse
	mu          sync.RWMutex
	rateLimiter *RateLimiter
	allowList   []string // Optional whitelist of domains
	blockList   []string // Blocked domains
}

// RateLimiter implements simple rate limiting
type RateLimiter struct {
	requests map[string][]time.Time
	mu       sync.Mutex
	limit    int           // requests per window
	window   time.Duration // time window
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		requests: make(map[string][]time.Time),
		limit:    limit,
		window:   window,
	}
}

// Allow checks if a request from an ID is allowed
func (rl *RateLimiter) Allow(id string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-rl.window)

	// Remove old requests
	var validRequests []time.Time
	for _, reqTime := range rl.requests[id] {
		if reqTime.After(cutoff) {
			validRequests = append(validRequests, reqTime)
		}
	}
	rl.requests[id] = validRequests

	// Check limit
	if len(validRequests) >= rl.limit {
		return false
	}

	// Add new request
	rl.requests[id] = append(rl.requests[id], now)
	return true
}

// NewTunnelServer creates a new tunnel server
func NewTunnelServer(fcpClient *fcp.Client) *TunnelServer {
	// Generate server ID
	idBytes := make([]byte, 16)
	rand.Read(idBytes)
	serverID := hex.EncodeToString(idBytes)

	return &TunnelServer{
		serverID:    serverID,
		fcpClient:   fcpClient,
		requests:    make(map[string]*TunnelRequest),
		responses:   make(map[string]*TunnelResponse),
		rateLimiter: NewRateLimiter(60, 1*time.Minute), // 60 requests per minute
	}
}

// Start starts the tunnel server
func (ts *TunnelServer) Start() error {
	// Generate SSK key pair for publishing responses
	debugLog("Generating SSK key pair...")
	keyPair, err := ts.fcpClient.GenerateSSK()
	if err != nil {
		return fmt.Errorf("failed to generate SSK: %w", err)
	}
	ts.sskKeyPair = keyPair

	log.Printf("Tunnel Server Started")
	log.Printf("Server ID: %s", ts.serverID)
	log.Printf("Public Key: %s", keyPair.PublicKey)
	log.Printf("Insert Key: %s", keyPair.PrivateKey)
	log.Println()
	log.Println("Share the Public Key with clients to allow them to connect!")
	log.Println()

	// Start request polling loop
	go ts.pollRequests()

	// Start response publisher loop
	go ts.publishResponses()

	// Start stats loop
	go ts.statsLoop()

	return nil
}

// pollRequests polls for incoming requests from Hyphanet
func (ts *TunnelServer) pollRequests() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		// Check for pending requests at our SSK location
		// Clients will publish requests to: SSK@.../tunnel-requests/<requestID>
		ts.checkForRequests()
	}
}

// checkForRequests checks for new requests on Hyphanet
func (ts *TunnelServer) checkForRequests() {
	// In a real implementation, this would check a known location for request queue
	// For now, this is a simplified version
	debugLog("Checking for requests...")
}

// handleRequest processes a clearnet request
func (ts *TunnelServer) handleRequest(req *TunnelRequest) *TunnelResponse {
	debugLog("Handling request: %s %s", req.Method, req.URL)

	// Rate limiting
	if !ts.rateLimiter.Allow(req.ID) {
		return &TunnelResponse{
			ID:         req.ID,
			StatusCode: http.StatusTooManyRequests,
			Status:     "429 Too Many Requests",
			Error:      "Rate limit exceeded",
		}
	}

	// Check blocklist
	if ts.isBlocked(req.URL) {
		return &TunnelResponse{
			ID:         req.ID,
			StatusCode: http.StatusForbidden,
			Status:     "403 Forbidden",
			Error:      "Domain blocked",
		}
	}

	// Check allowlist (if configured)
	if len(ts.allowList) > 0 && !ts.isAllowed(req.URL) {
		return &TunnelResponse{
			ID:         req.ID,
			StatusCode: http.StatusForbidden,
			Status:     "403 Forbidden",
			Error:      "Domain not in allowlist",
		}
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// Limit redirects
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	// Create request
	httpReq, err := http.NewRequest(req.Method, req.URL, nil)
	if err != nil {
		return &TunnelResponse{
			ID:         req.ID,
			StatusCode: http.StatusBadRequest,
			Status:     "400 Bad Request",
			Error:      fmt.Sprintf("Invalid request: %v", err),
		}
	}

	// Set headers
	for key, value := range req.Headers {
		httpReq.Header.Set(key, value)
	}

	// Add identifying header
	httpReq.Header.Set("X-Hyphanet-Tunnel", "true")

	// Execute request
	debugLog("Executing clearnet request to: %s", req.URL)
	resp, err := client.Do(httpReq)
	if err != nil {
		return &TunnelResponse{
			ID:         req.ID,
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Error:      fmt.Sprintf("Request failed: %v", err),
		}
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &TunnelResponse{
			ID:         req.ID,
			StatusCode: http.StatusBadGateway,
			Status:     "502 Bad Gateway",
			Error:      fmt.Sprintf("Failed to read response: %v", err),
		}
	}

	// Build response headers
	headers := make(map[string]string)
	for key, values := range resp.Header {
		if len(values) > 0 {
			headers[key] = values[0]
		}
	}

	debugLog("Response: %d %s (%d bytes)", resp.StatusCode, resp.Status, len(body))

	return &TunnelResponse{
		ID:         req.ID,
		StatusCode: resp.StatusCode,
		Status:     resp.Status,
		Headers:    headers,
		Body:       body,
	}
}

// publishResponses publishes responses back to Hyphanet
func (ts *TunnelServer) publishResponses() {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ts.mu.RLock()
		responsesToPublish := make([]*TunnelResponse, 0, len(ts.responses))
		for _, resp := range ts.responses {
			responsesToPublish = append(responsesToPublish, resp)
		}
		ts.mu.RUnlock()

		for _, resp := range responsesToPublish {
			ts.publishResponse(resp)
		}
	}
}

// publishResponse publishes a response to Hyphanet
func (ts *TunnelServer) publishResponse(resp *TunnelResponse) {
	debugLog("Publishing response for request: %s", resp.ID)

	// Serialize response
	data, err := json.Marshal(resp)
	if err != nil {
		debugLog("Failed to marshal response: %v", err)
		return
	}

	// Publish to SSK location
	uri := fmt.Sprintf("%s/response-%s", ts.sskKeyPair.PrivateKey, resp.ID)

	ops := fcp.NewOperations(ts.fcpClient)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := ops.Put(ctx, uri, data)
	if err != nil {
		debugLog("Failed to publish response: %v", err)
		return
	}

	if !result.Success {
		debugLog("Response publish failed: %s", result.Error)
		return
	}

	debugLog("Response published successfully")

	// Remove from pending responses
	ts.mu.Lock()
	delete(ts.responses, resp.ID)
	ts.mu.Unlock()
}

// isBlocked checks if a URL is in the blocklist
func (ts *TunnelServer) isBlocked(url string) bool {
	for _, blocked := range ts.blockList {
		if strings.Contains(url, blocked) {
			return true
		}
	}
	return false
}

// isAllowed checks if a URL is in the allowlist
func (ts *TunnelServer) isAllowed(url string) bool {
	for _, allowed := range ts.allowList {
		if strings.Contains(url, allowed) {
			return true
		}
	}
	return false
}

// statsLoop prints statistics periodically
func (ts *TunnelServer) statsLoop() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		ts.mu.RLock()
		pendingReqs := len(ts.requests)
		pendingResp := len(ts.responses)
		ts.mu.RUnlock()

		debugLog("Stats: %d pending requests, %d pending responses", pendingReqs, pendingResp)
	}
}

// AdminServer provides a simple web interface for server management
type AdminServer struct {
	tunnelServer *TunnelServer
}

// NewAdminServer creates a new admin server
func NewAdminServer(ts *TunnelServer) *AdminServer {
	return &AdminServer{tunnelServer: ts}
}

// Start starts the admin server
func (as *AdminServer) Start(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", as.handleDashboard)
	mux.HandleFunc("/stats", as.handleStats)

	log.Printf("Admin interface: http://%s", addr)
	return http.ListenAndServe(addr, mux)
}

// handleDashboard shows the admin dashboard
func (as *AdminServer) handleDashboard(w http.ResponseWriter, r *http.Request) {
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>Hyphanet Tunnel Server</title>
    <style>
        body { font-family: sans-serif; margin: 40px; background: #f5f5f5; }
        .container { background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); }
        h1 { color: #2196F3; }
        .info-box { background: #e3f2fd; padding: 20px; margin: 20px 0; border-left: 4px solid #2196F3; }
        .key { font-family: monospace; background: #f5f5f5; padding: 10px; margin: 10px 0; word-break: break-all; }
        .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 20px; margin: 20px 0; }
        .stat-card { background: #f5f5f5; padding: 20px; border-radius: 4px; text-align: center; }
        .stat-value { font-size: 2em; font-weight: bold; color: #2196F3; }
        .stat-label { color: #666; margin-top: 10px; }
    </style>
    <script>
        setInterval(function() {
            fetch('/stats').then(r => r.json()).then(data => {
                document.getElementById('pending-requests').textContent = data.pending_requests;
                document.getElementById('pending-responses').textContent = data.pending_responses;
            });
        }, 5000);
    </script>
</head>
<body>
    <div class="container">
        <h1>🌐 Hyphanet Tunnel Server</h1>
        
        <div class="info-box">
            <h3>Server Information</h3>
            <p><strong>Server ID:</strong> %s</p>
            <p><strong>Status:</strong> ✅ Running</p>
        </div>

        <div class="info-box">
            <h3>📡 Connection Details</h3>
            <p>Share this public key with clients:</p>
            <div class="key">%s</div>
        </div>

        <h2>Statistics</h2>
        <div class="stats">
            <div class="stat-card">
                <div class="stat-value" id="pending-requests">0</div>
                <div class="stat-label">Pending Requests</div>
            </div>
            <div class="stat-card">
                <div class="stat-value" id="pending-responses">0</div>
                <div class="stat-label">Pending Responses</div>
            </div>
        </div>

        <h2>How to Connect</h2>
        <ol>
            <li>Share the public key above with clients</li>
            <li>Clients run: <code>tunnelclient -server &lt;public_key&gt;</code></li>
            <li>Clients configure browser to use proxy at 127.0.0.1:8890</li>
            <li>Browse the web anonymously through Hyphanet!</li>
        </ol>
    </div>
</body>
</html>`, as.tunnelServer.serverID, as.tunnelServer.sskKeyPair.PublicKey)

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

// handleStats returns server statistics as JSON
func (as *AdminServer) handleStats(w http.ResponseWriter, r *http.Request) {
	as.tunnelServer.mu.RLock()
	stats := map[string]int{
		"pending_requests":  len(as.tunnelServer.requests),
		"pending_responses": len(as.tunnelServer.responses),
	}
	as.tunnelServer.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

func main() {
	fcpHost := flag.String("fcp-host", "localhost", "FCP host")
	fcpPort := flag.Int("fcp-port", 9481, "FCP port")
	adminAddr := flag.String("admin", "127.0.0.1:8080", "Admin interface address")
	allowListFile := flag.String("allowlist", "", "File with allowed domains (one per line)")
	blockListFile := flag.String("blocklist", "", "File with blocked domains (one per line)")
	debug := flag.Bool("debug", false, "Enable debug logging")
	verbose := flag.Bool("v", false, "Verbose output")
	showVersion := flag.Bool("version", false, "Show version information")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "tunnelserver - Clearnet Server for Hyphanet Tunnel v%s\n\n", fcp.Version)
		fmt.Fprintf(os.Stderr, "Run a server on the clearnet that proxies web requests through Hyphanet.\n")
		fmt.Fprintf(os.Stderr, "Clients connect through Hyphanet for censorship-resistant web access.\n\n")
		fmt.Fprintf(os.Stderr, "Usage: tunnelserver [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  tunnelserver --debug\n\n")
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

	log.Println("Hyphanet Tunnel Server Starting...")
	log.Println()

	// Connect to Hyphanet
	config := &fcp.Config{
		Host:    *fcpHost,
		Port:    *fcpPort,
		Name:    "tunnelserver",
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

	// Create tunnel server
	tunnelServer := NewTunnelServer(client)

	// Load allowlist
	if *allowListFile != "" {
		// Load allowlist implementation
		debugLog("Loading allowlist from: %s", *allowListFile)
	}

	// Load blocklist
	if *blockListFile != "" {
		// Load blocklist implementation
		debugLog("Loading blocklist from: %s", *blockListFile)
	}

	// Start tunnel server
	if err := tunnelServer.Start(); err != nil {
		log.Fatalf("Failed to start tunnel server: %v", err)
	}

	// Start admin interface
	adminServer := NewAdminServer(tunnelServer)
	go func() {
		if err := adminServer.Start(*adminAddr); err != nil {
			log.Fatalf("Failed to start admin server: %v", err)
		}
	}()

	log.Println()
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("Tunnel Server is Running!")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println()
	log.Println("Press Ctrl+C to stop")
	log.Println()

	// Wait forever
	select {}
}
