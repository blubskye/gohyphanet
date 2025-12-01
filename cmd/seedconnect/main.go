// GoHyphanet - Hyphanet Node Implementation
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/blubskye/gohyphanet/node"
)

func main() {
	// Command line flags
	seedHost := flag.String("seed", "", "Seed node hostname/IP")
	seedPort := flag.Int("seed-port", 12345, "Seed node port")
	seedIdentity := flag.String("seed-identity", "", "Seed node identity (base64)")
	localPort := flag.Int("port", 12346, "Local UDP port")
	debug := flag.Bool("debug", false, "Enable debug logging")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "seedconnect - Hyphanet Seed Node Connection Test\n\n")
		fmt.Fprintf(os.Stderr, "Usage: seedconnect [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  seedconnect -seed 198.50.223.20 -seed-port 59747 \\\n")
		fmt.Fprintf(os.Stderr, "    -seed-identity \"9KMO9Hrd7Jc4r8DCKCu2ZqlAZjAWCB5mhLi~A5n7wSM\" -debug\n\n")
	}

	flag.Parse()

	if *seedHost == "" {
		fmt.Fprintf(os.Stderr, "Error: -seed flag is required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("Hyphanet Seed Node Connection Test")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println()

	// Create node
	config := &node.Config{
		Port:      *localPort,
		DebugMode: *debug,
	}

	n, err := node.NewNode(config)
	if err != nil {
		log.Fatalf("Failed to create node: %v", err)
	}

	log.Printf("Local node identity hash: %x", n.GetIdentityHash()[:16])
	log.Printf("Listening on UDP port: %d", *localPort)
	log.Println()

	// Start node
	if err := n.Start(); err != nil {
		log.Fatalf("Failed to start node: %v", err)
	}

	log.Printf("Connecting to seed node: %s:%d", *seedHost, *seedPort)
	if *seedIdentity != "" {
		log.Printf("Using seed identity: %s", *seedIdentity)
	}
	log.Println()

	// Connect to seed node
	var connectErr error
	if *seedIdentity != "" {
		connectErr = n.ConnectToSeedNodeWithIdentity(*seedHost, *seedPort, *seedIdentity)
	} else {
		connectErr = n.ConnectToSeedNode(*seedHost, *seedPort)
	}
	if connectErr != nil {
		log.Fatalf("Failed to connect to seed node: %v", connectErr)
	}

	log.Println("Connection attempt sent!")
	log.Println("Waiting for handshake completion (press Ctrl+C to exit)...")
	log.Println()

	// Wait for interrupt
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Print statistics periodically
	statsTicker := time.NewTicker(5 * time.Second)
	defer statsTicker.Stop()

	// Keep running for a while to receive response
	timeout := time.After(60 * time.Second)

	for {
		select {
		case <-sigChan:
			log.Println("\nShutting down...")
			goto shutdown
		case <-timeout:
			log.Println("\nTimeout waiting for response")
			goto shutdown
		case <-statsTicker.C:
			// Print stats
			stats := n.GetStats()
			log.Printf("Stats: peers=%v sessions=%v handshakes=%d",
				stats["peers"], stats["sessions"], stats["active_handshakes"])

			// Check if connected
			if peerStats, ok := stats["peers"].(map[string]interface{}); ok {
				if connected, ok := peerStats["connected"].(int); ok && connected > 0 {
					log.Println("\n✓✓✓ SUCCESSFULLY CONNECTED TO SEED NODE ✓✓✓")
					log.Println("Handshake complete, session established!")
					log.Println()
					// Keep running to allow further communication
				}
			}
		}
	}

shutdown:

	// Stop node
	if err := n.Stop(); err != nil {
		log.Printf("Error stopping node: %v", err)
	}

	log.Println("Goodbye!")
}
