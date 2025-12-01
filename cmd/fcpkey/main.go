// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
)

const version = "0.1.0"

func main() {
	// Global flags
	host := flag.String("host", "localhost", "Freenet node hostname")
	port := flag.Int("port", 9481, "Freenet node port")
	keystore := flag.String("keystore", "", "Path to keystore file (default: ~/.gohyphanet/keys.json)")
	showVersion := flag.Bool("version", false, "Show version and license information")
	showLicense := flag.Bool("license", false, "Show license information")
	showSource := flag.Bool("source", false, "Show source code URL")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "fcpkey - Freenet Key Management v%s\n\n", version)
		fmt.Fprintf(os.Stderr, "Usage: fcpkey [global options] <command> [arguments]\n\n")
		fmt.Fprintf(os.Stderr, "Global Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nCommands:\n")
		fmt.Fprintf(os.Stderr, "  generate <name>           Generate a new SSK keypair\n")
		fmt.Fprintf(os.Stderr, "  add <name> <insert-uri>   Add an existing key\n")
		fmt.Fprintf(os.Stderr, "  get <name>                Get a key by name\n")
		fmt.Fprintf(os.Stderr, "  list                      List all keys\n")
		fmt.Fprintf(os.Stderr, "  delete <name>             Delete a key\n")
		fmt.Fprintf(os.Stderr, "  export <name>             Export a key (shows private key)\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  fcpkey generate mysite\n")
		fmt.Fprintf(os.Stderr, "  fcpkey list\n")
		fmt.Fprintf(os.Stderr, "  fcpkey get mysite\n")
		fmt.Fprintf(os.Stderr, "  fcpkey export mysite\n")
	}

	flag.Parse()

	if *showLicense {
		fmt.Println(fcp.PrintLicenseNotice())
		os.Exit(0)
	}

	if *showVersion {
		fmt.Println(fcp.GetFullVersionString())
		os.Exit(0)
	}

	if *showSource {
		fmt.Println(fcp.SourceURL)
		os.Exit(0)
	}

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	command := flag.Arg(0)

	// Load keystore - try SQLite first, fall back to JSON
	var ks fcp.KeyStoreInterface
	
	sqliteKS, err := fcp.NewSQLiteKeyStore(*keystore)
	if err == nil {
		ks = sqliteKS
		defer sqliteKS.Close()
	} else {
		// Fall back to JSON keystore
		jsonKS, err := fcp.NewKeyStore(*keystore)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to load keystore: %v\n", err)
			os.Exit(1)
		}
		ks = jsonKS
	}

	// Execute command
	switch command {
	case "generate":
		if flag.NArg() < 2 {
			fmt.Fprintf(os.Stderr, "Error: generate requires a key name\n")
			os.Exit(1)
		}
		handleGenerate(ks, flag.Arg(1), *host, *port)

	case "add":
		if flag.NArg() < 3 {
			fmt.Fprintf(os.Stderr, "Error: add requires a name and insert URI\n")
			os.Exit(1)
		}
		handleAdd(ks, flag.Arg(1), flag.Arg(2))

	case "get":
		if flag.NArg() < 2 {
			fmt.Fprintf(os.Stderr, "Error: get requires a key name\n")
			os.Exit(1)
		}
		handleGet(ks, flag.Arg(1), false)

	case "export":
		if flag.NArg() < 2 {
			fmt.Fprintf(os.Stderr, "Error: export requires a key name\n")
			os.Exit(1)
		}
		handleGet(ks, flag.Arg(1), true)

	case "list":
		handleList(ks)

	case "delete":
		if flag.NArg() < 2 {
			fmt.Fprintf(os.Stderr, "Error: delete requires a key name\n")
			os.Exit(1)
		}
		handleDelete(ks, flag.Arg(1))

	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown command: %s\n", command)
		flag.Usage()
		os.Exit(1)
	}
}

func handleGenerate(ks fcp.KeyStoreInterface, name, host string, port int) {
	fmt.Fprintf(os.Stderr, "Generating new SSK keypair...\n")

	// Connect to Freenet node
	config := &fcp.Config{
		Host:    host,
		Port:    port,
		Name:    "fcpkey",
		Version: "2.0",
	}

	client, err := fcp.Connect(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect to Freenet node: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	// Start listening
	go func() {
		if err := client.Listen(); err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "Warning: Listen error: %v\n", err)
		}
	}()

	// Generate key
	keyPair, err := client.GenerateSSK()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to generate key: %v\n", err)
		os.Exit(1)
	}

	// Save to keystore
	if err := ks.Add(name, keyPair); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to save key: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Successfully generated and saved key: %s\n", name)
	fmt.Fprintf(os.Stderr, "Type: %s\n", keyPair.Type)
	fmt.Fprintf(os.Stderr, "Request URI: %s\n", keyPair.PublicKey)
	fmt.Fprintf(os.Stderr, "\nUse 'fcpkey export %s' to see the private insert URI\n", name)
}

func handleAdd(ks fcp.KeyStoreInterface, name, insertURI string) {
	keyType := fcp.ParseKeyType(insertURI)
	if keyType == "UNKNOWN" {
		fmt.Fprintf(os.Stderr, "Error: Invalid key URI format\n")
		os.Exit(1)
	}

	requestURI := fcp.GetRequestURI(insertURI)

	keyPair := &fcp.KeyPair{
		Type:       keyType,
		PrivateKey: insertURI,
		PublicKey:  requestURI,
		Created:    time.Now(),
		Modified:   time.Now(),
	}

	if err := ks.Add(name, keyPair); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to add key: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Successfully added key: %s\n", name)
	fmt.Fprintf(os.Stderr, "Type: %s\n", keyType)
	fmt.Fprintf(os.Stderr, "Request URI: %s\n", requestURI)
}

func handleGet(ks fcp.KeyStoreInterface, name string, showPrivate bool) {
	keyPair, err := ks.Get(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Name: %s\n", keyPair.Name)
	fmt.Printf("Type: %s\n", keyPair.Type)
	fmt.Printf("Request URI: %s\n", keyPair.PublicKey)
	
	if showPrivate && keyPair.PrivateKey != "" {
		fmt.Printf("Insert URI: %s\n", keyPair.PrivateKey)
	}
	
	fmt.Printf("Created: %s\n", keyPair.Created.Format(time.RFC3339))
	fmt.Printf("Modified: %s\n", keyPair.Modified.Format(time.RFC3339))

	if len(keyPair.Metadata) > 0 {
		fmt.Println("\nMetadata:")
		for k, v := range keyPair.Metadata {
			fmt.Printf("  %s: %s\n", k, v)
		}
	}
}

func handleList(ks fcp.KeyStoreInterface) {
	var keys []*fcp.KeyPair
	var err error
	
	// Use ListAll if available
	if lister, ok := ks.(interface{ ListAll() ([]*fcp.KeyPair, error) }); ok {
		keys, err = lister.ListAll()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to list keys: %v\n", err)
			os.Exit(1)
		}
	} else {
		// Fall back to List + Get for each key
		names, err := ks.List()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to list keys: %v\n", err)
			os.Exit(1)
		}
		
		for _, name := range names {
			key, err := ks.Get(name)
			if err == nil {
				keys = append(keys, key)
			}
		}
	}
	
	if len(keys) == 0 {
		fmt.Println("No keys found")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tTYPE\tCREATED\tREQUEST URI")
	fmt.Fprintln(w, "----\t----\t-------\t-----------")

	for _, key := range keys {
		created := key.Created.Format("2006-01-02")
		// Truncate long URIs for display
		requestURI := key.PublicKey
		if len(requestURI) > 60 {
			requestURI = requestURI[:57] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", key.Name, key.Type, created, requestURI)
	}
	w.Flush()

	fmt.Fprintf(os.Stderr, "\nTotal: %d keys\n", len(keys))
	fmt.Fprintf(os.Stderr, "Use 'fcpkey get <name>' for details\n")
	fmt.Fprintf(os.Stderr, "Use 'fcpkey export <name>' to see private keys\n")
}

func handleDelete(ks fcp.KeyStoreInterface, name string) {
	// Confirm deletion
	fmt.Fprintf(os.Stderr, "Are you sure you want to delete key '%s'? (y/N): ", name)
	var response string
	fmt.Scanln(&response)

	if response != "y" && response != "Y" && response != "yes" && response != "Yes" {
		fmt.Fprintf(os.Stderr, "Deletion cancelled\n")
		return
	}

	if err := ks.Delete(name); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "Successfully deleted key: %s\n", name)
}
