// GoHyphanet - Java Wrapper Test
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/blubskye/gohyphanet/node/javashim"
)

func main() {
	seedHost := flag.String("seed", "198.50.223.20", "Seed node hostname/IP")
	seedPort := flag.Int("seed-port", 59747, "Seed node port")
	seedIdentity := flag.String("seed-identity", "9KMO9Hrd7Jc4r8DCKCu2ZqlAZjAWCB5mhLi~A5n7wSM", "Seed node identity (base64)")
	jarPath := flag.String("jar", "java/hyphanet-shim.jar", "Path to Java shim JAR")
	debug := flag.Bool("debug", false, "Enable debug logging")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "testwrapper - Test Java Handshake Wrapper\n\n")
		fmt.Fprintf(os.Stderr, "Usage: testwrapper [options]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  testwrapper -seed 198.50.223.20 -seed-port 59747 -debug\n\n")
	}

	flag.Parse()

	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)

	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("Java Handshake Wrapper Test")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println()

	// Resolve JAR path
	absJarPath, err := filepath.Abs(*jarPath)
	if err != nil {
		log.Fatalf("Failed to resolve JAR path: %v", err)
	}

	// Check if JAR exists
	if _, err := os.Stat(absJarPath); os.IsNotExist(err) {
		log.Fatalf("JAR file not found: %s", absJarPath)
	}

	log.Printf("Using JAR: %s", absJarPath)
	log.Println()

	// Create shim
	log.Println("Starting Java shim...")
	shim, err := javashim.NewShim(absJarPath, *debug)
	if err != nil {
		log.Fatalf("Failed to start shim: %v", err)
	}
	defer shim.Close()

	log.Println("✓ Java shim started and responding to ping")
	log.Println()

	// Perform handshake
	log.Printf("Attempting handshake with seed node: %s:%d", *seedHost, *seedPort)
	if *seedIdentity != "" {
		log.Printf("Using seed identity: %s...", (*seedIdentity)[:16])
	}
	log.Println()

	result, err := shim.HandshakeWithIdentity(*seedHost, *seedPort, *seedIdentity)
	if err != nil {
		log.Fatalf("Handshake failed: %v", err)
	}

	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println("Handshake Result")
	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Printf("Success: %v", result.Success)
	log.Printf("Message: %s", result.Message)

	if result.Success {
		log.Printf("Response Length: %d bytes", result.ResponseLength)
		log.Printf("Remote Address: %s:%d", result.RemoteAddress, result.RemotePort)
		log.Println()
		log.Println("✓ Successfully received response from seed node!")
	} else {
		log.Println()
		log.Println("✗ No response received (this is expected with simplified packet format)")
	}

	log.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	log.Println()

	log.Println("Shutting down Java shim...")
	if err := shim.Close(); err != nil {
		log.Printf("Warning: shim shutdown error: %v", err)
	}

	log.Println("Done!")
}
