// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details

// wottest - Test program for Web of Trust client
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/blubskye/gohyphanet/fcp"
	"github.com/blubskye/gohyphanet/wot"
)

func main() {
	host := flag.String("host", "localhost", "Freenet node host")
	port := flag.Int("port", 9481, "Freenet FCP port")
	flag.Parse()

	fmt.Println("Web of Trust Client Test")
	fmt.Println("========================")
	fmt.Printf("Connecting to %s:%d...\n", *host, *port)

	// Connect to Freenet
	config := &fcp.Config{
		Host:    *host,
		Port:    *port,
		Name:    "WoTTest",
		Version: "2.0",
	}

	client, err := wot.Connect(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	fmt.Println("Connected to Freenet node")

	// Start background listener
	client.StartListening()

	// Test Ping
	fmt.Println("\nTesting Ping...")
	if err := client.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "Ping failed: %v\n", err)
		fmt.Println("(WoT plugin may not be loaded)")
	} else {
		fmt.Println("Pong received - WoT is responding!")
	}

	// Get own identities
	fmt.Println("\nGetting own identities...")
	ownIdentities, err := client.GetOwnIdentities()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get own identities: %v\n", err)
	} else {
		fmt.Printf("Found %d own identities:\n", len(ownIdentities))
		for i, id := range ownIdentities {
			fmt.Printf("\n  [%d] %s\n", i+1, id.Nickname)
			fmt.Printf("      ID: %s\n", id.ID)
			fmt.Printf("      Contexts: %v\n", id.Contexts)
			if len(id.Properties) > 0 {
				fmt.Printf("      Properties: %v\n", id.Properties)
			}
		}
	}

	// Get identities with positive score (trusted)
	fmt.Println("\nGetting trusted identities with 'Sone' context...")
	trustedIdentities, err := client.GetIdentitiesByScore("", "+", "Sone")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get trusted identities: %v\n", err)
	} else {
		fmt.Printf("Found %d trusted Sone identities:\n", len(trustedIdentities))
		for i, id := range trustedIdentities {
			if i >= 10 {
				fmt.Printf("  ... and %d more\n", len(trustedIdentities)-10)
				break
			}
			fmt.Printf("  [%d] %s (%s)\n", i+1, id.Nickname, id.ID[:8]+"...")
		}
	}

	// Generate a random name
	fmt.Println("\nGenerating random name...")
	name, err := client.RandomName()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to generate random name: %v\n", err)
	} else {
		fmt.Printf("Random name: %s\n", name)
	}

	fmt.Println("\nWoT client test complete!")
}
