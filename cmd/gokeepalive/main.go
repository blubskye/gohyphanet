// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

// Command gokeepalive provides a standalone Hyphanet content reinserter
// that runs as a daemon with a built-in web interface accessible via browser.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/blubskye/gohyphanet/fcp"
	"github.com/blubskye/gohyphanet/keepalive"
	"github.com/blubskye/gohyphanet/keepalive/web"
)

// Version info
const (
	Version = "0.1.0"
	AppName = "GoKeepalive"
)

// Configuration
type Config struct {
	DataDir string
	FCPHost string
	FCPPort int
	WebPort int
}

var config = Config{
	DataDir: defaultDataDir(),
	FCPHost: "localhost",
	FCPPort: 9481,
	WebPort: keepalive.DefaultWebPort,
}

func defaultDataDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".gokeepalive"
	}
	return filepath.Join(home, ".gokeepalive")
}

func main() {
	// Flags
	flag.StringVar(&config.DataDir, "data", config.DataDir, "Data directory")
	flag.StringVar(&config.FCPHost, "fcp-host", config.FCPHost, "FCP host")
	flag.IntVar(&config.FCPPort, "fcp-port", config.FCPPort, "FCP port")
	flag.IntVar(&config.WebPort, "port", config.WebPort, "Web UI port")
	showVersion := flag.Bool("version", false, "Show version")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "%s v%s - Hyphanet Content Reinserter\n\n", AppName, Version)
		fmt.Fprintf(os.Stderr, "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Starts a daemon with a web interface for managing content reinsertion.\n")
		fmt.Fprintf(os.Stderr, "Access the UI at http://localhost:%d\n\n", config.WebPort)
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *showVersion {
		fmt.Printf("%s v%s\n", AppName, Version)
		fmt.Println("Hyphanet content reinserter with built-in web interface")
		return
	}

	// Start the service
	run()
}

func run() {
	fmt.Printf("%s v%s\n", AppName, Version)
	fmt.Printf("Data directory: %s\n", config.DataDir)

	// Initialize storage
	storage := keepalive.NewStorage(config.DataDir)
	if err := storage.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}

	siteManager := keepalive.NewSiteManager(storage)
	if err := siteManager.Initialize(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize site manager: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Loaded %d site(s)\n", siteManager.Count())

	// Connect to FCP
	fmt.Printf("Connecting to Hyphanet at %s:%d...\n", config.FCPHost, config.FCPPort)
	fcpConfig := &fcp.Config{
		Host: config.FCPHost,
		Port: config.FCPPort,
		Name: "GoKeepalive",
	}

	client, err := fcp.Connect(fcpConfig)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to connect to Hyphanet: %v\n", err)
		fmt.Fprintf(os.Stderr, "\nMake sure your Hyphanet node is running and FCP is enabled.\n")
		fmt.Fprintf(os.Stderr, "Default FCP port is 9481.\n")
		os.Exit(1)
	}
	fmt.Println("Connected to Hyphanet node")

	// Load config
	cfg, err := storage.LoadConfig()
	if err != nil {
		cfg = keepalive.NewConfig()
	}
	cfg.WebPort = config.WebPort

	// Create reinserter
	reinserter := keepalive.NewReinserter(client, cfg, siteManager, storage)

	// Set up callbacks
	reinserter.SetProgressCallback(func(site *keepalive.Site, segment *keepalive.Segment, message string) {
		segID := -1
		if segment != nil {
			segID = segment.ID
		}
		fmt.Printf("[%s] Segment %d: %s\n", site.Name, segID, message)
	})

	reinserter.SetCompleteCallback(func(site *keepalive.Site, success bool, errMsg string) {
		if success {
			fmt.Printf("[%s] Reinsertion complete!\n", site.Name)
		} else {
			fmt.Printf("[%s] Reinsertion failed: %s\n", site.Name, errMsg)
		}
	})

	// Start web server
	webServer := web.NewServer(config.WebPort)
	webServer.SetSiteManager(siteManager)
	webServer.SetReinserter(reinserter)
	webServer.SetConfig(cfg)

	if err := webServer.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start web server: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("==============================================")
	fmt.Printf("  Web UI: http://localhost:%d\n", config.WebPort)
	fmt.Println("==============================================")
	fmt.Println()
	fmt.Println("Open the URL above in your browser to:")
	fmt.Println("  - Add sites to keep alive")
	fmt.Println("  - Start/stop reinsertion")
	fmt.Println("  - Monitor progress")
	fmt.Println("  - Configure settings")
	fmt.Println()
	fmt.Println("Press Ctrl+C to stop...")

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	fmt.Println("\nShutting down...")

	// Stop reinserter if running
	reinserter.Stop()

	// Stop web server
	webServer.Stop()

	// Close FCP connection
	client.Close()

	fmt.Println("Goodbye!")
}
