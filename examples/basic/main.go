// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
)

func main() {
	// Create a configuration
	config := fcp.DefaultConfig()
	config.Name = "GoHyphanetBasicExample"

	// Connect to Freenet node
	fmt.Println("Connecting to Freenet node...")
	client, err := fcp.Connect(config)
	if err != nil {
		log.Fatalf("Failed to connect: %v", err)
	}
	defer client.Close()

	fmt.Println("✓ Connected to Freenet node!")

	// Start listening for messages in a goroutine
	go func() {
		if err := client.Listen(); err != nil {
			log.Printf("Listen error: %v", err)
		}
	}()

	// Create high-level operations API
	ops := fcp.NewOperations(client)

	// Example 1: Simple Put operation
	fmt.Println("\n=== Example 1: Inserting Data ===")
	testData := []byte("Hello from GoHyphanet! This is a test message.")
	testURI := fmt.Sprintf("KSK@test-%d", time.Now().Unix())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Printf("Inserting data to: %s\n", testURI)
	putResult, err := ops.Put(ctx, testURI, testData)
	if err != nil {
		log.Printf("Put failed: %v", err)
	} else if putResult.Success {
		fmt.Printf("✓ Successfully inserted!\n")
		fmt.Printf("  URI: %s\n", putResult.URI)
	} else {
		fmt.Printf("✗ Put failed: %s\n", putResult.Error)
	}

	// Example 2: Simple Get operation
	fmt.Println("\n=== Example 2: Retrieving Data ===")
	fmt.Printf("Retrieving data from: %s\n", testURI)

	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel2()

	getResult, err := ops.Get(ctx2, testURI)
	if err != nil {
		log.Printf("Get failed: %v", err)
	} else if getResult.Success {
		fmt.Printf("✓ Successfully retrieved!\n")
		fmt.Printf("  Data: %s\n", string(getResult.Data))
		fmt.Printf("  Size: %d bytes\n", len(getResult.Data))
	} else {
		fmt.Printf("✗ Get failed: %s\n", getResult.Error)
	}

	// Example 3: Generate a key
	fmt.Println("\n=== Example 3: Generating SSK Key ===")
	fmt.Println("Generating new SSK keypair...")

	keyPair, err := client.GenerateSSK()
	if err != nil {
		log.Printf("Key generation failed: %v", err)
	} else {
		fmt.Println("✓ Key generated successfully!")
		fmt.Printf("  Type: %s\n", keyPair.Type)
		fmt.Printf("  Public Key: %s\n", keyPair.PublicKey)
		fmt.Printf("  Private Key: %s\n", keyPair.PrivateKey)
	}

	// Example 4: Using key store
	fmt.Println("\n=== Example 4: Key Storage ===")
	
	// Try SQLite first
	keyStore, err := fcp.NewSQLiteKeyStore("")
	if err != nil {
		// Fall back to JSON
		fmt.Println("SQLite not available, using JSON keystore")
		jsonStore, err := fcp.NewKeyStore("")
		if err != nil {
			log.Printf("Failed to create keystore: %v", err)
			return
		}
		
		// Save the key
		if keyPair != nil {
			testKeyName := fmt.Sprintf("test-key-%d", time.Now().Unix())
			if err := jsonStore.Add(testKeyName, keyPair); err != nil {
				log.Printf("Failed to save key: %v", err)
			} else {
				fmt.Printf("✓ Key saved as: %s\n", testKeyName)
				
				// List all keys
				names, _ := jsonStore.List()
				fmt.Printf("  Total keys in store: %d\n", len(names))
			}
		}
	} else {
		defer keyStore.Close()
		fmt.Println("Using SQLite keystore")
		
		// Save the key
		if keyPair != nil {
			testKeyName := fmt.Sprintf("test-key-%d", time.Now().Unix())
			if err := keyStore.Add(testKeyName, keyPair); err != nil {
				log.Printf("Failed to save key: %v", err)
			} else {
				fmt.Printf("✓ Key saved as: %s\n", testKeyName)
				
				// List all keys
				names, _ := keyStore.List()
				fmt.Printf("  Total keys in store: %d\n", len(names))
				
				// Get stats (SQLite only)
				if stats, err := keyStore.GetStats(); err == nil {
					fmt.Printf("  Database size: %d bytes\n", stats["db_size_bytes"])
				}
			}
		}
	}

	fmt.Println("\n=== Examples Complete! ===")
	fmt.Println("\nNext steps:")
	fmt.Println("  - Try the concurrent examples: go run examples/concurrent/main.go")
	fmt.Println("  - Use the CLI tools: ./fcpget, ./fcpput, ./fcpkey")
	fmt.Println("  - Create a site: ./fcpsitemgr init mysite ./website")
}
