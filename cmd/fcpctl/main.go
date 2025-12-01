// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
)

const version = "0.1.0"

type SiteConfig struct {
	Name       string
	Path       string
	KeyName    string
	Version    int
	IndexFile  string
	DefaultMIME string
}

func main() {
	host := flag.String("host", "localhost", "Freenet node hostname")
	port := flag.Int("port", 9481, "Freenet node port")
	keystore := flag.String("keystore", "", "Path to keystore")
	showVersion := flag.Bool("version", false, "Show version and license information")
	showLicense := flag.Bool("license", false, "Show license information")
	showSource := flag.Bool("source", false, "Show source code URL")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "fcpctl - Freenet Control Tool v%s\n\n", fcp.Version)
		fmt.Fprintf(os.Stderr, "Usage: fcpctl [global options] <command> [arguments]\n\n")
		fmt.Fprintf(os.Stderr, "Global Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nCommands:\n")
		fmt.Fprintf(os.Stderr, "  getnode                Get node information\n")
		fmt.Fprintf(os.Stderr, "  shutdown               Shutdown the node\n")
		fmt.Fprintf(os.Stderr, "\nSource: %s\n", fcp.SourceURL)
		fmt.Fprintf(os.Stderr, "License: %s\n", fcp.LicenseName)
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

	// Load keystore
	var ks interface {
		Get(string) (*fcp.KeyPair, error)
		Add(string, *fcp.KeyPair) error
		List() ([]string, error)
	}

	// Try SQLite first, fall back to JSON
	sqliteKS, err := fcp.NewSQLiteKeyStore(*keystore)
	if err == nil {
		ks = sqliteKS
		defer sqliteKS.Close()
	} else {
		jsonKS, err := fcp.NewKeyStore(*keystore)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to load keystore: %v\n", err)
			os.Exit(1)
		}
		ks = jsonKS
	}

	switch command {
	case "init":
		if flag.NArg() < 3 {
			fmt.Fprintf(os.Stderr, "Error: init requires site name and path\n")
			os.Exit(1)
		}
		handleInit(flag.Arg(1), flag.Arg(2))

	case "upload":
		if flag.NArg() < 2 {
			fmt.Fprintf(os.Stderr, "Error: upload requires site name\n")
			os.Exit(1)
		}
		handleUpload(flag.Arg(1), *host, *port, ks)

	case "list":
		handleListSites()

	case "info":
		if flag.NArg() < 2 {
			fmt.Fprintf(os.Stderr, "Error: info requires site name\n")
			os.Exit(1)
		}
		handleInfo(flag.Arg(1), ks)

	case "genkey":
		if flag.NArg() < 2 {
			fmt.Fprintf(os.Stderr, "Error: genkey requires site name\n")
			os.Exit(1)
		}
		handleGenKey(flag.Arg(1), *host, *port, ks)

	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown command: %s\n", command)
		flag.Usage()
		os.Exit(1)
	}
}

func handleInit(siteName, sitePath string) {
	absPath, err := filepath.Abs(sitePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid path: %v\n", err)
		os.Exit(1)
	}

	// Create site directory if it doesn't exist
	if err := os.MkdirAll(absPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create directory: %v\n", err)
		os.Exit(1)
	}

	// Create a simple site config file
	configPath := filepath.Join(absPath, ".siteconfig")
	config := fmt.Sprintf("name=%s\npath=%s\nversion=0\nindex=index.html\n", siteName, absPath)
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write config: %v\n", err)
		os.Exit(1)
	}

	// Create a sample index.html if it doesn't exist
	indexPath := filepath.Join(absPath, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		sampleHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>%s</title>
</head>
<body>
    <h1>Welcome to %s</h1>
    <p>This is a sample Freesite. Edit this file to customize your site.</p>
</body>
</html>
`, siteName, siteName)
		if err := os.WriteFile(indexPath, []byte(sampleHTML), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to create sample index.html: %v\n", err)
		}
	}

	fmt.Printf("Site '%s' initialized at: %s\n", siteName, absPath)
	fmt.Println("\nNext steps:")
	fmt.Printf("  1. Generate a key: fcpsitemgr genkey %s\n", siteName)
	fmt.Printf("  2. Edit your site files in: %s\n", absPath)
	fmt.Printf("  3. Upload your site: fcpsitemgr upload %s\n", siteName)
}

func handleGenKey(siteName, host string, port int, ks interface {
	Get(string) (*fcp.KeyPair, error)
	Add(string, *fcp.KeyPair) error
}) {
	fmt.Fprintf(os.Stderr, "Generating key for site '%s'...\n", siteName)

	config := &fcp.Config{
		Host:    host,
		Port:    port,
		Name:    "fcpsitemgr",
		Version: "2.0",
	}

	client, err := fcp.Connect(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	go func() {
		if err := client.Listen(); err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}()

	keyPair, err := client.GenerateSSK()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to generate key: %v\n", err)
		os.Exit(1)
	}

	// Store with site name
	keyName := "site-" + siteName
	if err := ks.Add(keyName, keyPair); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to save key: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Key generated and saved as: %s\n", keyName)
	fmt.Printf("Public URI: %s\n", keyPair.PublicKey)
	fmt.Println("\nShare the public URI with others to let them access your site.")
}

func handleUpload(siteName, host string, port int, ks interface {
	Get(string) (*fcp.KeyPair, error)
}) {
	// Load site config
	siteConfig, err := loadSiteConfig(siteName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Hint: Run 'fcpsitemgr init %s <path>' first\n", siteName)
		os.Exit(1)
	}

	// Get the key
	keyName := "site-" + siteName
	keyPair, err := ks.Get(keyName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Key not found for site '%s'\n", siteName)
		fmt.Fprintf(os.Stderr, "Hint: Run 'fcpsitemgr genkey %s' first\n", siteName)
		os.Exit(1)
	}

	fmt.Printf("Uploading site '%s' from: %s\n", siteName, siteConfig.Path)

	// Connect to Freenet
	config := &fcp.Config{
		Host:    host,
		Port:    port,
		Name:    "fcpsitemgr",
		Version: "2.0",
	}

	client, err := fcp.Connect(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	go func() {
		if err := client.Listen(); err != nil && err != io.EOF {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	}()

	ops := fcp.NewOperations(client)

	// Collect all files
	files, err := collectFiles(siteConfig.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to collect files: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d files to upload\n", len(files))

	// Upload files concurrently
	var wg sync.WaitGroup
	semaphore := make(chan struct{}, 3) // Max 3 concurrent uploads
	errors := make(chan error, len(files))
	success := 0
	var mu sync.Mutex

	for _, file := range files {
		wg.Add(1)
		go func(file FileInfo) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			// Construct URI
			relPath := strings.TrimPrefix(file.RelPath, "/")
			uri := fmt.Sprintf("%s%s/%d/%s", 
				keyPair.PrivateKey, siteName, siteConfig.Version, relPath)

			fmt.Printf("Uploading: %s\n", relPath)

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
			defer cancel()

			result, err := ops.Put(ctx, uri, file.Data)
			if err != nil {
				errors <- fmt.Errorf("failed to upload %s: %w", relPath, err)
				return
			}

			if !result.Success {
				errors <- fmt.Errorf("failed to upload %s: %s", relPath, result.Error)
				return
			}

			mu.Lock()
			success++
			mu.Unlock()

			fmt.Printf("✓ Uploaded: %s\n", relPath)
		}(file)
	}

	wg.Wait()
	close(errors)

	// Report errors
	hasErrors := false
	for err := range errors {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		hasErrors = true
	}

	fmt.Printf("\nUpload complete: %d/%d files successful\n", success, len(files))

	if !hasErrors {
		// Increment version for next upload
		siteConfig.Version++
		if err := saveSiteConfig(siteName, siteConfig); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to update version: %v\n", err)
		}

		publicURI := fmt.Sprintf("%s%s/%d/", 
			keyPair.PublicKey, siteName, siteConfig.Version-1)
		fmt.Printf("\nYour site is available at:\n%s\n", publicURI)
	}
}

type FileInfo struct {
	RelPath string
	Data    []byte
}

func collectFiles(root string) ([]FileInfo, error) {
	var files []FileInfo

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		// Skip hidden files and config
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}

		// Convert to forward slashes for URIs
		relPath = filepath.ToSlash(relPath)

		files = append(files, FileInfo{
			RelPath: relPath,
			Data:    data,
		})

		return nil
	})

	return files, err
}

func loadSiteConfig(siteName string) (*SiteConfig, error) {
	// Try to find site config in common locations
	locations := []string{
		fmt.Sprintf("./%s/.siteconfig", siteName),
		fmt.Sprintf("./sites/%s/.siteconfig", siteName),
		fmt.Sprintf("%s/.siteconfig", siteName),
	}

	for _, loc := range locations {
		data, err := os.ReadFile(loc)
		if err == nil {
			return parseSiteConfig(data, loc)
		}
	}

	return nil, fmt.Errorf("site '%s' not found", siteName)
}

func parseSiteConfig(data []byte, configPath string) (*SiteConfig, error) {
	config := &SiteConfig{
		Version:   0,
		IndexFile: "index.html",
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		switch key {
		case "name":
			config.Name = value
		case "path":
			config.Path = value
		case "version":
			fmt.Sscanf(value, "%d", &config.Version)
		case "index":
			config.IndexFile = value
		}
	}

	// Use path from config file location if not specified
	if config.Path == "" {
		config.Path = filepath.Dir(configPath)
	}

	return config, nil
}

func saveSiteConfig(siteName string, config *SiteConfig) error {
	configPath := filepath.Join(config.Path, ".siteconfig")
	content := fmt.Sprintf("name=%s\npath=%s\nversion=%d\nindex=%s\n",
		config.Name, config.Path, config.Version, config.IndexFile)
	return os.WriteFile(configPath, []byte(content), 0644)
}

func handleListSites() {
	// Look for sites in current directory and ./sites
	locations := []string{".", "./sites"}

	found := false
	for _, loc := range locations {
		entries, err := os.ReadDir(loc)
		if err != nil {
			continue
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			configPath := filepath.Join(loc, entry.Name(), ".siteconfig")
			if _, err := os.Stat(configPath); err == nil {
				data, _ := os.ReadFile(configPath)
				config, _ := parseSiteConfig(data, configPath)
				if config != nil {
					fmt.Printf("%s (v%d) - %s\n", config.Name, config.Version, config.Path)
					found = true
				}
			}
		}
	}

	if !found {
		fmt.Println("No sites found")
		fmt.Println("Create a site with: fcpsitemgr init <name> <path>")
	}
}

func handleInfo(siteName string, ks interface{ Get(string) (*fcp.KeyPair, error) }) {
	config, err := loadSiteConfig(siteName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Site: %s\n", config.Name)
	fmt.Printf("Path: %s\n", config.Path)
	fmt.Printf("Version: %d\n", config.Version)
	fmt.Printf("Index: %s\n", config.IndexFile)

	keyName := "site-" + siteName
	keyPair, err := ks.Get(keyName)
	if err == nil {
		fmt.Printf("Public URI: %s%s/%d/\n", keyPair.PublicKey, siteName, config.Version)
	} else {
		fmt.Println("Key: Not generated (run 'fcpsitemgr genkey')")
	}

	// Count files
	files, err := collectFiles(config.Path)
	if err == nil {
		fmt.Printf("Files: %d\n", len(files))

		totalSize := 0
		for _, f := range files {
			totalSize += len(f.Data)
		}
		fmt.Printf("Total size: %d bytes\n", totalSize)
	}
}
