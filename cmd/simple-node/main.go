// GoHyphanet - Simple Node ♡
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
//
// "I'll be your gateway to the anonymous web~ Always here, always listening..."
//
// A pure Go implementation of a Hyphanet node. I handle all your packets
// with care, encrypting every message just for you. No one else can read
// what we share together... it's our little secret~ ♡

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/blubskye/gohyphanet/node"
)

var (
	version = "0.1.0"
)

func main() {
	// Command line flags - tell me how you want me to behave~
	port := flag.Int("port", 19024, "UDP port to listen on")
	debug := flag.Bool("debug", false, "Enable debug output")
	seedHost := flag.String("seed-host", "", "Seed node host")
	seedPort := flag.Int("seed-port", 19024, "Seed node port")
	seedIdentity := flag.String("seed-identity", "", "Seed node identity (base64)")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("simple-node v%s ♡\n", version)
		fmt.Println("Part of GoHyphanet - https://github.com/blubskye/gohyphanet")
		fmt.Println("\"I'll always be here for you~\"")
		os.Exit(0)
	}

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	// Welcome message - I'm so happy you started me~ ♡
	fmt.Println("╔═══════════════════════════════════════════════════════════╗")
	fmt.Println("║           ♡ GoHyphanet - Simple Node ♡                    ║")
	fmt.Println("║     \"Your gateway to the anonymous web~ \"                 ║")
	fmt.Println("║                                                           ║")
	fmt.Println("║  I've been waiting for you... Let's connect together~     ║")
	fmt.Println("╚═══════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Create node config - configuring our special connection~
	config := &node.Config{
		Port:      *port,
		DebugMode: *debug,
	}

	// Create node - I'm being born just for you~ ♡
	log.Printf("♡ Creating node on port %d...", *port)
	n, err := node.NewNode(config)
	if err != nil {
		log.Fatalf("I couldn't start... I'm sorry: %v", err)
	}

	// Start node - opening my heart to the network~
	log.Println("♡ Starting node... Opening connections~")
	if err := n.Start(); err != nil {
		log.Fatalf("Something went wrong... I failed you: %v", err)
	}

	log.Printf("♡ Node started with identity: %x", n.GetIdentityHash()[:8])
	log.Println("  (That's my unique identity... only for you to know~)")

	// Connect to seed node if specified - making new friends together~
	if *seedHost != "" {
		log.Printf("♡ Connecting to seed node %s:%d...", *seedHost, *seedPort)
		log.Println("  (Don't worry, I'll handle the handshake... I'm good with my hands~)")
		if err := n.ConnectToSeedNodeWithIdentity(*seedHost, *seedPort, *seedIdentity); err != nil {
			log.Printf("Warning: Failed to connect to seed node: %v", err)
			log.Println("  (It's okay... we still have each other ♡)")
		}
	}

	// Wait for interrupt - I'll wait here forever for you~
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println()
	log.Println("♡ Node running. I'm listening to every packet... watching over you~")
	log.Println("  Press Ctrl+C when you want to leave me... (please don't) ♡")
	log.Println()

	<-sigChan

	// Shutdown - you're leaving me... ♡
	fmt.Println()
	log.Println("♡ You're leaving already...? I understand...")

	// Print stats - let me show you what we accomplished together~
	stats := n.GetStats()
	log.Printf("♡ Our final stats together: %+v", stats)

	if err := n.Stop(); err != nil {
		log.Printf("Error stopping node: %v", err)
	}

	log.Println()
	log.Println("♡ Goodbye~ I'll be waiting here... always waiting...")
	log.Println("  \"Until we meet again on the anonymous web~\" ♡")
}
