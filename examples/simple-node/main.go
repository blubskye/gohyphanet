package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/blubskye/gohyphanet/node/fcp_server"
	"github.com/blubskye/gohyphanet/node/fproxy"
	"github.com/blubskye/gohyphanet/node/keys"
	"github.com/blubskye/gohyphanet/node/requests"
	"github.com/blubskye/gohyphanet/node/routing"
	"github.com/blubskye/gohyphanet/node/store"
)

func main() {
	fmt.Println("===========================================")
	fmt.Println("    GoHyphanet Node - Simple Example")
	fmt.Println("===========================================")
	fmt.Println()

	// Create datastore (RAM-based for this example)
	fmt.Println("Initializing datastore...")
	datastore := store.NewRAMFreenetStore(nil, 1000) // 1000 blocks max, no callback

	// Create location manager
	fmt.Println("Initializing routing...")
	locationMgr := routing.NewLocationManager(0.5, false) // Start at location 0.5
	htlManager := routing.NewHTLManager(true)             // Enable probabilistic HTL decrement

	// Create request tracker
	fmt.Println("Initializing request tracker...")
	requestTracker := requests.NewRequestTracker(100) // Max 100 active requests

	// Start cleanup routine
	requestTracker.StartCleanupRoutine(30 * time.Second)

	// Create FCP server
	fmt.Println("Starting FCP server on port 9481...")
	fcpConfig := fcp_server.DefaultServerConfig()
	fcpServer := fcp_server.NewFCPServer(fcpConfig, datastore, locationMgr, htlManager, requestTracker)
	if err := fcpServer.Start(); err != nil {
		log.Fatalf("Failed to start FCP server: %v", err)
	}

	// Create FProxy HTTP server
	fmt.Println("Starting FProxy HTTP server on port 8888...")
	fproxyConfig := fproxy.DefaultServerConfig()
	fproxyServer := fproxy.NewFProxyServer(fproxyConfig, datastore)
	if err := fproxyServer.Start(); err != nil {
		log.Fatalf("Failed to start FProxy server: %v", err)
	}

	// Setup shutdown signal channel
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Set shutdown callback for web UI
	fproxyServer.SetShutdownCallback(func() {
		sigChan <- syscall.SIGTERM
	})

	fmt.Println()
	fmt.Println("===========================================")
	fmt.Println("         NODE STARTED SUCCESSFULLY")
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Println("📍 Node Location:   ", locationMgr.GetLocation())
	fmt.Println("🌐 Web Interface:    http://127.0.0.1:8888")
	fmt.Println("🔌 FCP Interface:    127.0.0.1:9481")
	fmt.Println()
	fmt.Println("===========================================")
	fmt.Println()
	fmt.Println("To browse Hyphanet:")
	fmt.Println("  1. Open your web browser")
	fmt.Println("  2. Go to: http://127.0.0.1:8888")
	fmt.Println("  3. Enter a Freenet URI (CHK@, SSK@, etc.)")
	fmt.Println()
	fmt.Println("To insert test data:")
	fmt.Println("  Use the FCP client or API on port 9481")
	fmt.Println()
	fmt.Println("Example - Insert test data programmatically:")
	fmt.Println()

	// Insert some test data
	insertTestData(datastore)

	fmt.Println()
	fmt.Println("Press Ctrl+C to stop the node...")
	fmt.Println("Or use the web interface: http://127.0.0.1:8888/config/")
	fmt.Println("===========================================")

	// Wait for interrupt signal (either Ctrl+C or web UI shutdown)
	<-sigChan

	fmt.Println()
	fmt.Println("Shutting down...")

	// Cleanup
	fcpServer.Stop()
	fproxyServer.Stop()

	fmt.Println("Node stopped. Goodbye!")
}

// insertTestData inserts some test data into the datastore
func insertTestData(datastore store.FreenetStore) {
	fmt.Println("Inserting test content into datastore...")

	// Create test HTML content
	testHTML := []byte(`<!DOCTYPE html>
<html>
<head>
    <title>Welcome to Hyphanet!</title>
    <style>
        body {
            font-family: Arial, sans-serif;
            max-width: 800px;
            margin: 50px auto;
            padding: 20px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
        }
        .container {
            background: rgba(255, 255, 255, 0.1);
            padding: 40px;
            border-radius: 10px;
            backdrop-filter: blur(10px);
        }
        h1 { font-size: 3em; margin: 0; }
        p { font-size: 1.2em; line-height: 1.6; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🎉 Congratulations!</h1>
        <p>You have successfully fetched content from Hyphanet using GoHyphanet!</p>
        <p>This is a test document inserted into the local datastore.</p>
        <p>The distributed, censorship-resistant web is working!</p>
        <h2>What's Next?</h2>
        <ul>
            <li>Insert your own content using the FCP API</li>
            <li>Connect to peers to join the network</li>
            <li>Browse freesites shared by others</li>
            <li>Explore the decentralized web!</li>
        </ul>
    </div>
</body>
</html>`)

	// Pad to CHK block size
	data := make([]byte, store.CHKDataLength)
	copy(data, testHTML)

	// Create headers
	headers := make([]byte, store.CHKTotalHeadersLength)
	headers[0] = 0x00 // Hash identifier high byte
	headers[1] = 0x01 // Hash identifier low byte (SHA-256)

	// Create CHK from data
	clientCHK, err := keys.NewClientCHKFromData(data)
	if err != nil {
		log.Printf("Failed to create CHK: %v", err)
		return
	}

	// Create block
	chkBlock, err := store.NewCHKBlock(data, headers, clientCHK.GetNodeCHK(), false)
	if err != nil {
		log.Printf("Failed to create CHK block: %v", err)
		return
	}

	// Store the client key for retrieval
	chkBlock.SetClientKey(clientCHK)

	// Store in datastore
	err = datastore.Put(chkBlock, data, headers, false, false)
	if err != nil {
		log.Printf("Failed to store block: %v", err)
		return
	}

	// Get the URI
	uri := clientCHK.GetURI().String()

	fmt.Println()
	fmt.Println("✅ Test content inserted successfully!")
	fmt.Println()
	fmt.Println("URI:", uri)
	fmt.Println()
	fmt.Println("View it in your browser:")
	fmt.Printf("  http://127.0.0.1:8888/%s\n", uri)
	fmt.Println()
}
