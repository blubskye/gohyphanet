package fproxy

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/node/keys"
	"github.com/blubskye/gohyphanet/node/store"
)

const (
	// DefaultFProxyPort is the standard FProxy HTTP port
	DefaultFProxyPort = 8888

	// MaxContentSize is the maximum size of content to serve
	MaxContentSize = 100 * 1024 * 1024 // 100MB
)

// FProxyServer serves Hyphanet content over HTTP
type FProxyServer struct {
	mu sync.RWMutex

	// Configuration
	bindAddr string
	port     int
	enabled  bool

	// HTTP server
	httpServer *http.Server

	// Node dependencies
	datastore store.FreenetStore

	// Statistics
	requestCount int64
	errorCount   int64

	// Shutdown callback
	shutdownCallback func()
}

// ServerConfig contains FProxy server configuration
type ServerConfig struct {
	BindAddr string
	Port     int
	Enabled  bool
}

// DefaultServerConfig returns default FProxy server configuration
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		BindAddr: "127.0.0.1",
		Port:     DefaultFProxyPort,
		Enabled:  true,
	}
}

// NewFProxyServer creates a new FProxy HTTP server
func NewFProxyServer(
	config *ServerConfig,
	datastore store.FreenetStore,
) *FProxyServer {
	if config == nil {
		config = DefaultServerConfig()
	}

	server := &FProxyServer{
		bindAddr:  config.BindAddr,
		port:      config.Port,
		enabled:   config.Enabled,
		datastore: datastore,
	}

	// Create HTTP server
	mux := http.NewServeMux()

	// Register single handler for all paths
	// This mimics fred-next's FProxyToadlet behavior
	mux.HandleFunc("/", server.handleRequest)

	server.httpServer = &http.Server{
		Addr:           fmt.Sprintf("%s:%d", config.BindAddr, config.Port),
		Handler:        mux,
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	return server
}

// Start starts the FProxy HTTP server
func (s *FProxyServer) Start() error {
	if !s.enabled {
		log.Printf("FProxy server disabled, not starting")
		return nil
	}

	log.Printf("FProxy server starting on http://%s:%d", s.bindAddr, s.port)
	log.Printf("Browse Hyphanet at: http://%s:%d/", s.bindAddr, s.port)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("FProxy server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the FProxy HTTP server
func (s *FProxyServer) Stop() error {
	if !s.enabled || s.httpServer == nil {
		return nil
	}

	log.Printf("Stopping FProxy server...")
	return s.httpServer.Close()
}

// handleRequest handles all HTTP requests
// Based on fred-next's FProxyToadlet.handleMethodGET
func (s *FProxyServer) handleRequest(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	s.requestCount++
	s.mu.Unlock()

	path := r.URL.Path

	// Handle POST requests with "key" parameter (from welcome page form)
	if r.Method == "POST" && path == "/" {
		if err := r.ParseForm(); err == nil {
			if keyParam := r.FormValue("key"); keyParam != "" {
				// Use query parameter to avoid path normalization issues
				http.Redirect(w, r, "/?key="+url.QueryEscape(keyParam), http.StatusSeeOther)
				return
			}
		}
		// No key provided, show welcome page
		s.serveWelcomePage(w, r)
		return
	}

	// Handle GET requests
	if path == "/" {
		// Check if there's a "key" query parameter
		keyParam := r.URL.Query().Get("key")
		if keyParam != "" {
			log.Printf("[FProxy] GET request with key parameter: %s", keyParam)
			// Fetch and serve the content
			s.fetchAndServe(w, r, keyParam)
			return
		}
		// No key, serve welcome page
		s.serveWelcomePage(w, r)
		return
	}

	// Check if this is a navigation/admin path (not a Freenet URI)
	// Normalize path by removing leading/trailing slashes
	normalizedPath := strings.Trim(path, "/")

	// Route to specific handlers
	switch {
	case normalizedPath == "queue" || strings.HasPrefix(normalizedPath, "queue/"):
		s.serveQueuePage(w, r)
		return
	case normalizedPath == "uploads" || strings.HasPrefix(normalizedPath, "uploads/"):
		s.serveUploadsPage(w, r)
		return
	case normalizedPath == "downloads" || strings.HasPrefix(normalizedPath, "downloads/"):
		s.serveDownloadsPage(w, r)
		return
	case normalizedPath == "config" || strings.HasPrefix(normalizedPath, "config/"):
		s.serveConfigPage(w, r)
		return
	case normalizedPath == "plugins" || strings.HasPrefix(normalizedPath, "plugins/"):
		s.servePluginsPage(w, r)
		return
	case normalizedPath == "friends" || strings.HasPrefix(normalizedPath, "friends/"):
		s.servePeersPage(w, r, "friends")
		return
	case normalizedPath == "strangers" || strings.HasPrefix(normalizedPath, "strangers/"):
		s.servePeersPage(w, r, "strangers")
		return
	case normalizedPath == "alerts" || strings.HasPrefix(normalizedPath, "alerts/"):
		s.serveAlertsPage(w, r)
		return
	case normalizedPath == "statistics" || strings.HasPrefix(normalizedPath, "statistics/"):
		s.serveStatisticsPage(w, r)
		return
	case normalizedPath == "bookmarkEditor" || strings.HasPrefix(normalizedPath, "bookmarkEditor/"):
		s.serveNavigationPlaceholder(w, r, "bookmarkEditor")
		return
	case normalizedPath == "shutdown" || strings.HasPrefix(normalizedPath, "shutdown/"):
		s.handleShutdown(w, r)
		return
	}

	// For paths like /CHK@..., decode and handle
	// Strip leading "/" and URL-decode to get the Freenet URI
	uriStr := path[1:]

	// Try URL decoding in case it's encoded
	if decoded, err := url.PathUnescape(uriStr); err == nil {
		uriStr = decoded
	}

	// Fix double-slash issue: Go's ServeMux normalizes paths
	// If this looks like a Freenet URI but is missing slashes, check raw path
	if strings.Contains(uriStr, "@") && !strings.Contains(uriStr, "//") {
		// Try to get the original non-normalized path
		// In HTTP/1.1, Go normalizes the path but we can detect the issue
		// by checking if the URI parsing fails
		if _, err := keys.ParseFreenetURI(uriStr); err != nil {
			// URI is malformed due to normalization
			// Suggest using query parameter instead
			s.serveError(w, http.StatusBadRequest,
				fmt.Sprintf("Path normalization may have corrupted the URI. "+
					"Please use the home page form or /?key=<URI> format instead. Error: %v", err))
			return
		}
	}

	// Try to parse as Freenet URI and fetch content
	s.fetchAndServe(w, r, uriStr)
}

// serveNavigationPlaceholder serves a placeholder page for navigation menu items
func (s *FProxyServer) serveNavigationPlaceholder(w http.ResponseWriter, r *http.Request, pageName string) {
	// Capitalize first letter for display
	displayName := pageName
	if len(displayName) > 0 {
		displayName = strings.ToUpper(string(displayName[0])) + displayName[1:]
	}

	htmlTemplate := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>%s - Freenet</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 700px;
            margin: 50px auto;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .placeholder-box {
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            border-left: 5px solid #ffc107;
        }
        h1 {
            color: #ffc107;
            margin-top: 0;
        }
        a {
            color: #667eea;
            text-decoration: none;
        }
        a:hover {
            text-decoration: underline;
        }
    </style>
</head>
<body>
    <div class="placeholder-box">
        <h1>%s Page</h1>
        <p>The <strong>%s</strong> page is not yet implemented in GoHyphanet.</p>
        <p>This is a work in progress. The following features are planned:</p>
        <ul>
            <li>Full Freenet 0.7 protocol compatibility</li>
            <li>Queue management for downloads and uploads</li>
            <li>Configuration interface</li>
            <li>Plugin system</li>
            <li>Peer management</li>
        </ul>
        <p><a href="/">← Back to Home</a></p>
    </div>
</body>
</html>`

	html := fmt.Sprintf(htmlTemplate, displayName, displayName, displayName)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, html)
}

// fetchAndServe fetches content from Hyphanet and serves it
func (s *FProxyServer) fetchAndServe(w http.ResponseWriter, r *http.Request, uriStr string) {
	log.Printf("[FProxy] Request: %s from %s", uriStr, r.RemoteAddr)

	// Parse the Freenet URI
	uri, err := keys.ParseFreenetURI(uriStr)
	if err != nil {
		s.serveError(w, http.StatusBadRequest, fmt.Sprintf("Invalid Freenet URI: %v", err))
		return
	}

	// Extract the key
	var key keys.Key
	switch uri.KeyType {
	case "CHK":
		clientCHK, err := uri.ToClientCHK()
		if err != nil {
			s.serveError(w, http.StatusBadRequest, fmt.Sprintf("Invalid CHK: %v", err))
			return
		}
		key = clientCHK.GetNodeCHK()

	case "SSK", "KSK":
		clientSSK, err := uri.ToClientSSK()
		if err != nil {
			s.serveError(w, http.StatusBadRequest, fmt.Sprintf("Invalid SSK/KSK: %v", err))
			return
		}
		nodeSSK, _ := clientSSK.GetNodeKey(false)
		key = nodeSSK

	case "USK":
		s.serveError(w, http.StatusNotImplemented, "USK support not yet implemented")
		return

	default:
		s.serveError(w, http.StatusBadRequest, "Unsupported key type: "+uri.KeyType)
		return
	}

	// Fetch from datastore
	meta := store.NewBlockMetadata()
	block, err := s.datastore.Fetch(
		key.GetRoutingKey(),
		key.GetFullKey(),
		false, // dontPromote
		true,  // canReadClientCache
		true,  // canReadSlashdotCache
		false, // ignoreOldBlocks
		meta,
	)

	if err != nil || block == nil {
		s.serveNotFound(w, uriStr)
		return
	}

	// Verify it's a KeyBlock
	keyBlock, ok := block.(store.KeyBlock)
	if !ok {
		s.serveError(w, http.StatusInternalServerError, "Invalid block type")
		return
	}

	// Serve the content
	s.serveContent(w, r, keyBlock, uri)
}

// GetStats returns FProxy server statistics
func (s *FProxyServer) GetStats() FProxyServerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return FProxyServerStats{
		Enabled:      s.enabled,
		ListenAddr:   fmt.Sprintf("http://%s:%d", s.bindAddr, s.port),
		RequestCount: s.requestCount,
		ErrorCount:   s.errorCount,
	}
}

// FProxyServerStats contains FProxy server statistics
type FProxyServerStats struct {
	Enabled      bool
	ListenAddr   string
	RequestCount int64
	ErrorCount   int64
}

// String returns a formatted string of server statistics
func (fps FProxyServerStats) String() string {
	status := "disabled"
	if fps.Enabled {
		status = "enabled"
	}
	return fmt.Sprintf("FProxy Server: %s, %s, %d requests, %d errors",
		status, fps.ListenAddr, fps.RequestCount, fps.ErrorCount)
}

// SetShutdownCallback sets the callback to call when shutdown is requested
func (s *FProxyServer) SetShutdownCallback(callback func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shutdownCallback = callback
}

// handleShutdown handles shutdown requests from the web UI
func (s *FProxyServer) handleShutdown(w http.ResponseWriter, r *http.Request) {
	// Only accept POST requests
	if r.Method != "POST" {
		// Show shutdown confirmation page
		s.serveShutdownPage(w, r)
		return
	}

	// Check if shutdown was confirmed
	if err := r.ParseForm(); err == nil {
		if r.FormValue("confirm") == "yes" {
			// Send response before shutting down
			html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Shutting Down - Freenet</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 50px; text-align: center; background-color: #f5f5f5; }
        .shutdown-box { max-width: 500px; margin: 50px auto; padding: 30px; background: white; border-radius: 8px; box-shadow: 0 2px 4px rgba(0,0,0,0.1); border-left: 5px solid #e74c3c; }
        h1 { color: #e74c3c; }
        p { margin: 20px 0; color: #333; }
    </style>
</head>
<body>
    <div class="shutdown-box">
        <h1>Node Shutting Down</h1>
        <p>The Freenet node is shutting down...</p>
        <p>You can close this window.</p>
    </div>
</body>
</html>`
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, html)

			// Trigger shutdown after a brief delay
			go func() {
				time.Sleep(500 * time.Millisecond)
				s.mu.RLock()
				callback := s.shutdownCallback
				s.mu.RUnlock()
				if callback != nil {
					log.Println("Shutdown requested via web UI")
					callback()
				} else {
					log.Println("Shutdown requested but no callback set")
				}
			}()
			return
		}
	}

	// If not confirmed, redirect back to shutdown page
	http.Redirect(w, r, "/shutdown/", http.StatusSeeOther)
}

// serveShutdownPage serves the shutdown confirmation page
func (s *FProxyServer) serveShutdownPage(w http.ResponseWriter, r *http.Request) {
	html := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Shutdown Node - Freenet</title>
    <style>
        * { margin: 0; padding: 0; }
        body { font-family: Arial, sans-serif; font-size: 11pt; background-color: #f5f5f5; }
        #topbar { background: linear-gradient(to bottom, #667eea 0%, #764ba2 100%); border-bottom: 1px solid #ccc; min-height: 50px; padding: 0.333em; color: white; }
        #topbar h1 { font-size: 1.818em; font-weight: normal; padding-top: 0.35em; text-align: center; color: white; }
        #navbar { float: left; position: relative; left: 0.667em; top: 0.667em; width: 11.081em; border: 1px solid #ccc; background: #fff; }
        #navlist { list-style-type: none; }
        #navlist li { border-bottom: 1px dotted #ddd; }
        #navlist a { display: block; padding: 0.667em; text-decoration: none; color: #333; font-weight: bold; }
        #navlist a:hover { background-color: #f0f0f0; }
        #navlist li.navlist-selected a { background-color: #667eea; color: white; }
        #content { margin-top: 0.667em; margin-left: 12.5em; margin-right: 0.667em; position: relative; }
        div.infobox { background: #fff; border: 1px solid #ccc; margin-bottom: 0.667em; border-radius: 4px; }
        div.infobox-header { background: #f0f0f0; padding: 0.667em; border-bottom: 1px dotted #ccc; font-weight: bold; }
        div.infobox-content { padding: 0.667em; }
        div.infobox-warning { border-left: 4px solid #e74c3c; }
        .shutdown-warning { padding: 20px; background: #fff3cd; border-radius: 4px; margin: 15px 0; border-left: 3px solid #ffc107; }
        .button-group { margin-top: 20px; }
        input[type="submit"] { padding: 10px 30px; margin: 5px; font-size: 14px; border: none; border-radius: 4px; cursor: pointer; }
        .shutdown-btn { background: #e74c3c; color: white; }
        .shutdown-btn:hover { background: #c0392b; }
        .cancel-btn { background: #95a5a6; color: white; }
        .cancel-btn:hover { background: #7f8c8d; }
    </style>
</head>
<body>
    <div id="topbar"><h1>Freenet</h1></div>
    <div id="navbar">
        <ul id="navlist">
            <li><a href="/">Browse</a></li>
            <li><a href="/queue/">Queue</a></li>
            <li><a href="/uploads/">Uploads</a></li>
            <li><a href="/downloads/">Downloads</a></li>
            <li><a href="/config/">Configuration</a></li>
            <li><a href="/plugins/">Plugins</a></li>
            <li><a href="/friends/">Friends</a></li>
            <li><a href="/strangers/">Strangers</a></li>
            <li><a href="/alerts/">Alerts</a></li>
            <li><a href="/statistics/">Statistics</a></li>
        </ul>
    </div>
    <div id="content">
        <div class="infobox infobox-warning">
            <div class="infobox-header">Shutdown Confirmation</div>
            <div class="infobox-content">
                <div class="shutdown-warning">
                    <h3 style="margin-bottom: 10px; color: #856404;">⚠ Warning</h3>
                    <p>You are about to shut down the Freenet node.</p>
                </div>

                <p><strong>This will:</strong></p>
                <ul style="margin: 15px 0 15px 30px;">
                    <li>Stop the FProxy web interface (port 8888)</li>
                    <li>Stop the FCP interface (port 9481)</li>
                    <li>Terminate all active downloads and uploads</li>
                    <li>Close all network connections</li>
                    <li>Exit the node process</li>
                </ul>

                <p style="margin-top: 15px;">Are you sure you want to shut down the node?</p>

                <form method="POST" action="/shutdown/">
                    <div class="button-group">
                        <input type="hidden" name="confirm" value="yes">
                        <input type="submit" value="Shutdown Node" class="shutdown-btn">
                        <input type="button" value="Cancel" class="cancel-btn" onclick="window.location.href='/'">
                    </div>
                </form>
            </div>
        </div>
    </div>
</body>
</html>`

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Powered-By", "GoHyphanet/0.1")
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, html)
}
