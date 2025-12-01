// GoHyphanet - Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
)

type SiteConfig struct {
	Name        string
	Path        string
	KeyName     string
	Version     int
	IndexFile   string
	DefaultMIME string
}

var debugMode bool

func debugLog(format string, args ...interface{}) {
	if debugMode {
		log.Printf("[DEBUG] "+format, args...)
	}
}

func main() {
	host := flag.String("host", "localhost", "Hyphanet node hostname")
	port := flag.Int("port", 9481, "Hyphanet node port")
	keystore := flag.String("keystore", "", "Path to keystore")
	showVersion := flag.Bool("version", false, "Show version and license information")
	showLicense := flag.Bool("license", false, "Show license information")
	indexFile := flag.String("index", "index.html", "Default index file for site")
	progress := flag.Bool("progress", false, "Show upload progress")
	debug := flag.Bool("debug", false, "Enable debug logging")
	verbose := flag.Bool("v", false, "Verbose output")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "fcpsitemgr - Freesite Manager v%s\n\n", fcp.Version)
		fmt.Fprintf(os.Stderr, "Usage: fcpsitemgr [global options] <command> [arguments]\n\n")
		fmt.Fprintf(os.Stderr, "Global Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nCommands:\n")
		fmt.Fprintf(os.Stderr, "  init <site> [<path>]   Initialize a new site (prompts for path if not provided)\n")
		fmt.Fprintf(os.Stderr, "  upload <site>          Upload site to Hyphanet\n")
		fmt.Fprintf(os.Stderr, "  list                   List all configured sites\n")
		fmt.Fprintf(os.Stderr, "  info <site>            Show site information\n")
		fmt.Fprintf(os.Stderr, "  genkey <site>          Generate a new key for site\n")
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  fcpsitemgr init mysite ./website --index main.html\n")
		fmt.Fprintf(os.Stderr, "  fcpsitemgr genkey mysite --debug\n")
		fmt.Fprintf(os.Stderr, "  fcpsitemgr upload mysite --progress --debug\n")
		fmt.Fprintf(os.Stderr, "\nSource: %s\n", fcp.SourceURL)
		fmt.Fprintf(os.Stderr, "License: %s\n", fcp.LicenseName)
	}

	flag.Parse()

	debugMode = *debug || *verbose

	if debugMode {
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
		debugLog("Debug mode enabled")
		// Enable FCP debug mode
		fcp.DebugMode = true
	}

	if *showLicense {
		fmt.Println(fcp.PrintLicenseNotice())
		os.Exit(0)
	}

	if *showVersion {
		fmt.Println(fcp.GetFullVersionString())
		os.Exit(0)
	}

	if flag.NArg() < 1 {
		flag.Usage()
		os.Exit(1)
	}

	command := flag.Arg(0)
	debugLog("Command: %s", command)

	// Load keystore
	var ks interface {
		Get(string) (*fcp.KeyPair, error)
		Add(string, *fcp.KeyPair) error
		List() ([]string, error)
	}

	// Try SQLite first, fall back to JSON
	debugLog("Loading keystore from: %s", *keystore)
	sqliteKS, err := fcp.NewSQLiteKeyStore(*keystore)
	if err == nil {
		debugLog("Using SQLite keystore")
		ks = sqliteKS
		defer sqliteKS.Close()
	} else {
		debugLog("SQLite keystore failed: %v, falling back to JSON", err)
		jsonKS, err := fcp.NewKeyStore(*keystore)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to load keystore: %v\n", err)
			os.Exit(1)
		}
		debugLog("Using JSON keystore")
		ks = jsonKS
	}

	switch command {
	case "init":
		if flag.NArg() < 2 {
			fmt.Fprintf(os.Stderr, "Error: init requires site name\n")
			os.Exit(1)
		}
		sitePath := ""
		if flag.NArg() >= 3 {
			sitePath = flag.Arg(2)
		}
		handleInit(flag.Arg(1), sitePath, *indexFile)

	case "upload":
		if flag.NArg() < 2 {
			fmt.Fprintf(os.Stderr, "Error: upload requires site name\n")
			os.Exit(1)
		}
		handleUpload(flag.Arg(1), *host, *port, ks, *progress)

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

func handleInit(siteName, sitePath, defaultIndex string) {
	debugLog("Initializing site: %s at path: %s", siteName, sitePath)

	if sitePath == "" {
		// Prompt for path
		reader := bufio.NewReader(os.Stdin)
		fmt.Printf("Enter site path (default: ./%s): ", siteName)
		input, _ := reader.ReadString('\n')
		sitePath = strings.TrimSpace(input)
		if sitePath == "" {
			sitePath = fmt.Sprintf("./%s", siteName)
		}
	}

	absPath, err := filepath.Abs(sitePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid path: %v\n", err)
		os.Exit(1)
	}
	debugLog("Absolute path: %s", absPath)

	// Create site directory if it doesn't exist
	if err := os.MkdirAll(absPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to create directory: %v\n", err)
		os.Exit(1)
	}

	// Create a simple site config file
	configPath := filepath.Join(absPath, ".siteconfig")
	config := fmt.Sprintf("name=%s\npath=%s\nversion=0\nindex=%s\n", siteName, absPath, defaultIndex)
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to write config: %v\n", err)
		os.Exit(1)
	}
	debugLog("Config written to: %s", configPath)

	// Create a sample index.html if it doesn't exist
	indexPath := filepath.Join(absPath, defaultIndex)
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		sampleHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>%s</title>
</head>
<body>
    <h1>Welcome to %s</h1>
    <p>This is a sample Freesite. Edit this file to customize your site.</p>
    <footer>
        <p>Powered by <a href="%s">GoHyphanet</a> (AGPLv3)</p>
    </footer>
</body>
</html>
`, siteName, siteName, fcp.SourceURL)
		if err := os.WriteFile(indexPath, []byte(sampleHTML), 0644); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: Failed to create sample %s: %v\n", defaultIndex, err)
		}
		debugLog("Sample index created: %s", indexPath)
	}

	fmt.Printf("Site '%s' initialized at: %s with index: %s\n", siteName, absPath, defaultIndex)
	fmt.Println("\nNext steps:")
	fmt.Printf("  1. Generate a key: fcpsitemgr genkey %s\n", siteName)
	fmt.Printf("  2. Edit your site files in: %s\n", absPath)
	fmt.Printf("  3. Upload your site: fcpsitemgr upload %s --progress\n", siteName)
}

func handleGenKey(siteName, host string, port int, ks interface {
	Get(string) (*fcp.KeyPair, error)
	Add(string, *fcp.KeyPair) error
}) {
	fmt.Fprintf(os.Stderr, "Generating key for site '%s'...\n", siteName)
	debugLog("Connecting to %s:%d", host, port)

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
	debugLog("Connected to Hyphanet node")

	// Start listener
	listenerDone := make(chan error, 1)
	go func() {
		debugLog("Starting FCP listener...")
		err := client.Listen()
		debugLog("Listener stopped: %v", err)
		listenerDone <- err
	}()

	// Give listener time to start
	time.Sleep(200 * time.Millisecond)
	debugLog("Requesting SSK key generation...")

	keyPair, err := client.GenerateSSK()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to generate key: %v\n", err)
		os.Exit(1)
	}
	debugLog("Key generated successfully")

	// Store with site name
	keyName := "site-" + siteName
	if err := ks.Add(keyName, keyPair); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to save key: %v\n", err)
		os.Exit(1)
	}
	debugLog("Key saved to keystore as: %s", keyName)

	fmt.Printf("Key generated and saved as: %s\n", keyName)
	fmt.Printf("Public URI: %s\n", keyPair.PublicKey)
	fmt.Println("\nShare the public URI with others to let them access your site.")
}

func handleUpload(siteName, host string, port int, ks interface {
	Get(string) (*fcp.KeyPair, error)
}, showProgress bool) {
	debugLog("Starting upload for site: %s", siteName)

	// Load site config
	siteConfig, err := loadSiteConfig(siteName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Hint: Run 'fcpsitemgr init %s <path>' first\n", siteName)
		os.Exit(1)
	}
	debugLog("Site config loaded: %+v", siteConfig)

	// Get the key
	keyName := "site-" + siteName
	keyPair, err := ks.Get(keyName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Key not found for site '%s'\n", siteName)
		fmt.Fprintf(os.Stderr, "Hint: Run 'fcpsitemgr genkey %s' first\n", siteName)
		os.Exit(1)
	}
	debugLog("Key loaded: %s", keyName)

	fmt.Printf("Uploading site '%s' from: %s\n", siteName, siteConfig.Path)

	// Connect to Hyphanet
	config := &fcp.Config{
		Host:    host,
		Port:    port,
		Name:    "fcpsitemgr",
		Version: "2.0",
	}

	debugLog("Connecting to %s:%d", host, port)
	client, err := fcp.Connect(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()
	debugLog("Connected to Hyphanet node")

	// Start listener in background
	go func() {
		debugLog("Starting FCP listener for uploads...")
		err := client.Listen()
		if err != nil && err != io.EOF && !strings.Contains(err.Error(), "use of closed network connection") {
			debugLog("Listener error: %v", err)
		}
		debugLog("Listener stopped")
	}()

	time.Sleep(200 * time.Millisecond)

	// Collect all files
	files, err := collectFiles(siteConfig.Path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to collect files: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d files to upload\n\n", len(files))

	// Use USK format for versioned site
	uskURI := strings.Replace(keyPair.PrivateKey, "SSK@", "USK@", 1)
	uskURI = strings.TrimSuffix(uskURI, "/")
	siteURI := fmt.Sprintf("%s/%s/%d/", uskURI, siteName, siteConfig.Version)

	debugLog("Site URI: %s", siteURI)
	fmt.Printf("Uploading to: %s\n\n", siteURI)

	// Upload using ClientPutComplexDir
	err = uploadFreesite(client, siteURI, files, siteConfig.IndexFile, showProgress)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: Upload failed: %v\n", err)
		os.Exit(1)
	}

	// Increment version
	siteConfig.Version++
	if err := saveSiteConfig(siteName, siteConfig); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: Failed to update version: %v\n", err)
	}

	publicURI := strings.Replace(keyPair.PublicKey, "SSK@", "USK@", 1)
	publicURI = strings.TrimSuffix(publicURI, "/")
	publicURI = fmt.Sprintf("%s/%s/%d/", publicURI, siteName, siteConfig.Version-1)

	fmt.Printf("\n✓ Upload complete!\n\n")
	fmt.Printf("Your site is available at:\n%s\n", publicURI)
}

func uploadFreesite(client *fcp.Client, siteURI string, files []FileInfo, defaultDoc string, showProgress bool) error {
	identifier := fmt.Sprintf("site-%d", time.Now().UnixNano())
	debugLog("Upload identifier: %s", identifier)

	resultChan := make(chan *fcp.OperationResult, 1)
	errChan := make(chan error, 1)

	// Progress tracking
	var progressMu sync.Mutex
	var lastTime = time.Now()
	var lastSucceeded int
	var speed float64

	// Register ALL handlers BEFORE sending the message
	debugLog("Registering progress handler")
	client.RegisterHandler("SimpleProgress", func(m *fcp.Message) error {
		if m.Fields["Identifier"] != identifier {
			return nil
		}

		progressMu.Lock()
		defer progressMu.Unlock()

		var succeeded, total int
		fmt.Sscanf(m.Fields["Succeeded"], "%d", &succeeded)
		fmt.Sscanf(m.Fields["Total"], "%d", &total)

		percent := 0.0
		if total > 0 {
			percent = float64(succeeded) / float64(total) * 100
		}

		currentTime := time.Now()
		deltaTime := currentTime.Sub(lastTime).Seconds()
		if deltaTime > 0.5 {
			deltaBlocks := succeeded - lastSucceeded
			const blockSize = 32768
			speed = float64(deltaBlocks*blockSize) / deltaTime / 1024
			lastTime = currentTime
			lastSucceeded = succeeded
		}

		barWidth := 50
		filled := int(float64(barWidth) * percent / 100)
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

		fmt.Fprintf(os.Stderr, "\r[%s] %.1f%% (%d/%d blocks) %.2f KB/s    ",
			bar, percent, succeeded, total, speed)

		return nil
	})

	debugLog("Registering result handlers")
	client.RegisterHandler("PutSuccessful", func(m *fcp.Message) error {
		if m.Fields["Identifier"] == identifier {
			debugLog("PutSuccessful received")
			resultChan <- &fcp.OperationResult{Success: true, URI: m.Fields["URI"]}
		}
		return nil
	})

	client.RegisterHandler("PutFailed", func(m *fcp.Message) error {
		if m.Fields["Identifier"] == identifier {
			errChan <- fmt.Errorf("%s: %s", m.Fields["Code"], m.Fields["CodeDescription"])
		}
		return nil
	})

	// Build message
	msg := &fcp.Message{
		Name: "ClientPutComplexDir",
		Fields: map[string]string{
			"URI":           siteURI,
			"Identifier":    identifier,
			"Verbosity":     "511",
			"MaxRetries":    "-1",
			"PriorityClass": "3",
			"Global":        "false",
			"DefaultName":   defaultDoc,
		},
	}

	// Add files
	for i, file := range files {
		msg.Fields[fmt.Sprintf("Files.%d.Name", i)] = file.RelPath
		msg.Fields[fmt.Sprintf("Files.%d.UploadFrom", i)] = "direct"
		msg.Fields[fmt.Sprintf("Files.%d.DataLength", i)] = fmt.Sprintf("%d", len(file.Data))
		msg.Fields[fmt.Sprintf("Files.%d.Metadata.ContentType", i)] = guessMimeType(file.RelPath)
	}

	debugLog("Sending ClientPutComplexDir")
	if err := client.SendMessage(msg); err != nil {
		return err
	}

	// Send file data
	for _, file := range files {
		debugLog("Sending data for: %s", file.RelPath)
		if err := client.SendRawData(file.Data); err != nil {
			return fmt.Errorf("failed to send %s: %w", file.RelPath, err)
		}
	}

	// Wait for result
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	select {
	case <-resultChan:
		if showProgress {
			fmt.Fprintf(os.Stderr, "\n")
		}
		return nil
	case err := <-errChan:
		if showProgress {
			fmt.Fprintf(os.Stderr, "\n")
		}
		return err
	case <-ctx.Done():
		return fmt.Errorf("timeout")
	}
}

func guessMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	mimeTypes := map[string]string{
		".html": "text/html", ".htm": "text/html", ".css": "text/css",
		".js": "application/javascript", ".json": "application/json",
		".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
		".gif": "image/gif", ".svg": "image/svg+xml", ".ico": "image/x-icon",
		".pdf": "application/pdf", ".txt": "text/plain",
	}
	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

type FileInfo struct {
	RelPath string
	Data    []byte
}

func collectFiles(root string) ([]FileInfo, error) {
	var files []FileInfo
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(root, path)
		relPath = filepath.ToSlash(relPath)
		files = append(files, FileInfo{RelPath: relPath, Data: data})
		return nil
	})
	return files, err
}

func loadSiteConfig(siteName string) (*SiteConfig, error) {
	locations := []string{
		fmt.Sprintf("./%s/.siteconfig", siteName),
		fmt.Sprintf("./sites/%s/.siteconfig", siteName),
		fmt.Sprintf("%s/.siteconfig", siteName),
	}
	for _, loc := range locations {
		if data, err := os.ReadFile(loc); err == nil {
			return parseSiteConfig(data, loc)
		}
	}
	return nil, fmt.Errorf("site '%s' not found", siteName)
}

func parseSiteConfig(data []byte, configPath string) (*SiteConfig, error) {
	config := &SiteConfig{Version: 0, IndexFile: "index.html"}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "name":
			config.Name = parts[1]
		case "path":
			config.Path = parts[1]
		case "version":
			fmt.Sscanf(parts[1], "%d", &config.Version)
		case "index":
			config.IndexFile = parts[1]
		}
	}
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
	locations := []string{".", "./sites"}
	found := false
	for _, loc := range locations {
		entries, _ := os.ReadDir(loc)
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			configPath := filepath.Join(loc, entry.Name(), ".siteconfig")
			if data, err := os.ReadFile(configPath); err == nil {
				if config, _ := parseSiteConfig(data, configPath); config != nil {
					fmt.Printf("%s (v%d) - %s\n", config.Name, config.Version, config.Path)
					found = true
				}
			}
		}
	}
	if !found {
		fmt.Println("No sites found. Create one with: fcpsitemgr init <site> <path>")
	}
}

func handleInfo(siteName string, ks interface{ Get(string) (*fcp.KeyPair, error) }) {
	config, err := loadSiteConfig(siteName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Site: %s\nPath: %s\nVersion: %d\nIndex: %s\n",
		config.Name, config.Path, config.Version, config.IndexFile)
	keyPair, _ := ks.Get("site-" + siteName)
	if keyPair != nil {
		publicURI := strings.Replace(keyPair.PublicKey, "SSK@", "USK@", 1)
		publicURI = strings.TrimSuffix(publicURI, "/")
		fmt.Printf("Public URI: %s/%s/%d/\n", publicURI, siteName, config.Version)
	} else {
		fmt.Println("Key: Not generated")
	}
	if files, err := collectFiles(config.Path); err == nil {
		totalSize := 0
		for _, f := range files {
			totalSize += len(f.Data)
		}
		fmt.Printf("Files: %d\nTotal size: %d bytes\n", len(files), totalSize)
	}
}
