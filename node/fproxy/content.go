package fproxy

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/blubskye/gohyphanet/node/keys"
	"github.com/blubskye/gohyphanet/node/store"
)

// serveContent serves fetched content to the HTTP client
func (s *FProxyServer) serveContent(w http.ResponseWriter, r *http.Request, block store.KeyBlock, uri *keys.FreenetURI) {
	data := block.GetRawData()

	// Determine MIME type from query parameter or default
	mimeType := r.URL.Query().Get("type")
	if mimeType == "" {
		mimeType = detectMIMEType(data)
	}

	// Set headers
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
	w.Header().Set("X-Freenet-URI", uri.String())

	// Add cache headers for immutable content (CHK)
	if uri.KeyType == "CHK" {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}

	// Write content
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}


// serveError serves an error page
func (s *FProxyServer) serveError(w http.ResponseWriter, statusCode int, message string) {
	s.mu.Lock()
	s.errorCount++
	s.mu.Unlock()

	htmlTemplate := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Error - Hyphanet</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 700px;
            margin: 50px auto;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .error-box {
            background: white;
            padding: 30px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            border-left: 5px solid #dc3545;
        }
        h1 {
            color: #dc3545;
            margin-top: 0;
        }
        .error-code {
            font-size: 1.2em;
            color: #666;
            margin-bottom: 15px;
        }
        .message {
            font-size: 1.1em;
            margin-bottom: 20px;
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
    <div class="error-box">
        <h1>Error</h1>
        <div class="error-code">HTTP %d</div>
        <div class="message">%s</div>
        <p><a href="/">← Back to Home</a></p>
    </div>
</body>
</html>`

	html := fmt.Sprintf(htmlTemplate, statusCode, html.EscapeString(message))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(statusCode)
	w.Write([]byte(html))
}

// serveNotFound serves a "content not found" page
func (s *FProxyServer) serveNotFound(w http.ResponseWriter, uri string) {
	s.mu.Lock()
	s.errorCount++
	s.mu.Unlock()

	htmlTemplate := `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Not Found - Hyphanet</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 700px;
            margin: 50px auto;
            padding: 20px;
            background-color: #f5f5f5;
        }
        .error-box {
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
        .uri {
            background: #f8f9fa;
            padding: 15px;
            border-radius: 5px;
            font-family: monospace;
            word-break: break-all;
            margin: 15px 0;
        }
        a {
            color: #667eea;
            text-decoration: none;
        }
        a:hover {
            text-decoration: underline;
        }
        ul {
            margin-top: 15px;
        }
        li {
            margin-bottom: 10px;
        }
    </style>
</head>
<body>
    <div class="error-box">
        <h1>Content Not Found</h1>
        <p>The requested content could not be found in the local datastore:</p>
        <div class="uri">%s</div>
        <h3>Possible Reasons:</h3>
        <ul>
            <li>The content has not been inserted into the network yet</li>
            <li>This node doesn't have the data cached locally</li>
            <li>The URI might be incorrect</li>
            <li>Network routing is not yet connected to peers</li>
        </ul>
        <h3>What to do:</h3>
        <ul>
            <li>Verify the URI is correct</li>
            <li>Wait for the content to propagate if recently inserted</li>
            <li>Connect to more peers to increase data availability</li>
        </ul>
        <p><a href="/">← Back to Home</a></p>
    </div>
</body>
</html>`

	html := fmt.Sprintf(htmlTemplate, html.EscapeString(uri))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(html))
}

// detectMIMEType attempts to detect the MIME type of data
func detectMIMEType(data []byte) string {
	if len(data) == 0 {
		return "application/octet-stream"
	}

	// Check for common file signatures
	if len(data) >= 2 {
		// PNG
		if data[0] == 0x89 && data[1] == 0x50 {
			return "image/png"
		}
		// JPEG
		if data[0] == 0xFF && data[1] == 0xD8 {
			return "image/jpeg"
		}
		// GIF
		if data[0] == 0x47 && data[1] == 0x49 {
			return "image/gif"
		}
	}

	if len(data) >= 5 {
		// HTML
		htmlStart := string(data[:5])
		if strings.Contains(strings.ToLower(htmlStart), "<!doc") ||
			strings.Contains(strings.ToLower(htmlStart), "<html") {
			return "text/html"
		}
	}

	// Check for text
	if isText(data[:min(512, len(data))]) {
		return "text/plain; charset=utf-8"
	}

	return "application/octet-stream"
}

// isText checks if data appears to be text
func isText(data []byte) bool {
	for _, b := range data {
		if b < 0x20 && b != '\t' && b != '\n' && b != '\r' {
			return false
		}
		if b >= 0x7F && b < 0xA0 {
			return false
		}
	}
	return true
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
