// GoHyphanet - Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
)

var debugMode bool

func debugLog(format string, args ...interface{}) {
	if debugMode {
		log.Printf("[FCPPUT] "+format, args...)
	}
}

func main() {
	// Command line flags
	input := flag.String("i", "", "Input file (default: stdin)")
	host := flag.String("host", "localhost", "Hyphanet node hostname")
	port := flag.Int("port", 9481, "Hyphanet node port")
	timeout := flag.Duration("timeout", 30*time.Minute, "Operation timeout")
	progress := flag.Bool("progress", true, "Show upload progress")
	quiet := flag.Bool("q", false, "Quiet mode (no progress to stderr)")
	verbose := flag.Bool("v", false, "Verbose output")
	debug := flag.Bool("debug", false, "Enable debug logging")
	maxRetries := flag.Int("retries", 3, "Number of retries on failure")
	chk := flag.Bool("chk", false, "Generate CHK (if no URI specified)")
	showVersion := flag.Bool("version", false, "Show version and license information")
	showLicense := flag.Bool("license", false, "Show license information")
	showSource := flag.Bool("source", false, "Show source code URL")
	priority := flag.Int("priority", 3, "Priority class (0-6, lower is higher priority)")
	compress := flag.Bool("compress", true, "Compress data before insertion")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "fcpput - Insert data into Hyphanet v%s\n\n", fcp.Version)
		fmt.Fprintf(os.Stderr, "Usage: fcpput [options] [URI]\n\n")
		fmt.Fprintf(os.Stderr, "If no URI is provided, generates a CHK.\n")
		fmt.Fprintf(os.Stderr, "URI can be: KSK@name, SSK@..., USK@..., or CHK@\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Insert file to KSK\n")
		fmt.Fprintf(os.Stderr, "  fcpput KSK@mykey -i file.txt\n\n")
		fmt.Fprintf(os.Stderr, "  # Insert from stdin\n")
		fmt.Fprintf(os.Stderr, "  echo 'Hello' | fcpput KSK@greeting\n\n")
		fmt.Fprintf(os.Stderr, "  # Generate CHK with progress\n")
		fmt.Fprintf(os.Stderr, "  fcpput -i file.pdf --progress\n\n")
		fmt.Fprintf(os.Stderr, "  # Insert tarball\n")
		fmt.Fprintf(os.Stderr, "  tar czf - mydir/ | fcpput --progress\n\n")
		fmt.Fprintf(os.Stderr, "  # Debug stuck upload\n")
		fmt.Fprintf(os.Stderr, "  fcpput KSK@test -i large.zip --debug --progress\n\n")
		fmt.Fprintf(os.Stderr, "Source: %s\n", fcp.SourceURL)
		fmt.Fprintf(os.Stderr, "License: %s\n", fcp.LicenseName)
	}

	flag.Parse()

	debugMode = *debug || *verbose
	showProgress := *progress && !*quiet

	if debugMode {
		log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds)
		debugLog("Debug mode enabled")
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

	if *showSource {
		fmt.Println(fcp.SourceURL)
		os.Exit(0)
	}

	// Get URI if provided
	var uri string
	if flag.NArg() > 0 {
		uri = flag.Arg(0)
	} else if *chk {
		uri = "CHK@"
	} else {
		// Generate CHK by default if no URI specified
		uri = "CHK@"
	}

	debugLog("Target URI: %s", uri)

	// Read input data
	var data []byte
	var err error

	if *input == "" || *input == "-" {
		if !*quiet {
			fmt.Fprintf(os.Stderr, "Reading from stdin...\n")
		}
		data, err = io.ReadAll(os.Stdin)
	} else {
		if !*quiet {
			fmt.Fprintf(os.Stderr, "Reading from: %s\n", *input)
		}
		data, err = os.ReadFile(*input)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to read input: %v\n", err)
		os.Exit(1)
	}

	if len(data) == 0 {
		fmt.Fprintf(os.Stderr, "Error: No data to insert\n")
		os.Exit(1)
	}

	debugLog("Data size: %d bytes", len(data))

	if !*quiet {
		fmt.Fprintf(os.Stderr, "Inserting %d bytes to %s...\n", len(data), uri)
	}

	// Connect to Hyphanet node
	config := &fcp.Config{
		Host:    *host,
		Port:    *port,
		Name:    "fcpput",
		Version: "2.0",
	}

	debugLog("Connecting to %s:%d", *host, *port)
	client, err := fcp.Connect(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to connect to Hyphanet node at %s:%d: %v\n", *host, *port, err)
		os.Exit(1)
	}
	defer client.Close()
	debugLog("Connected to Hyphanet node")

	// Start listening for messages
	go func() {
		debugLog("Starting FCP listener...")
		err := client.Listen()
		if err != nil && err != io.EOF && !strings.Contains(err.Error(), "use of closed network connection") {
			if !*quiet {
				debugLog("Listener error: %v", err)
			}
		}
		debugLog("Listener stopped")
	}()

	time.Sleep(200 * time.Millisecond)

	// Attempt insertion with retries
	var result *fcp.OperationResult
	for attempt := 1; attempt <= *maxRetries; attempt++ {
		if attempt > 1 && !*quiet {
			fmt.Fprintf(os.Stderr, "\nRetry %d/%d...\n", attempt, *maxRetries)
			time.Sleep(time.Duration(attempt) * 2 * time.Second)
		}

		if showProgress {
			result, err = uploadWithProgress(client, uri, data, *timeout, *priority, *compress)
		} else {
			result, err = uploadSimple(client, uri, data, *timeout, *priority, *compress)
		}

		if err == nil && result != nil && result.Success {
			break
		}

		debugLog("Upload attempt %d failed: %v", attempt, err)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "\nError: Put operation failed: %v\n", err)
		os.Exit(1)
	}

	if !result.Success {
		fmt.Fprintf(os.Stderr, "\nError: Put failed: %s\n", result.Error)
		os.Exit(1)
	}

	// Output the resulting URI
	fmt.Println(result.URI)

	if !*quiet {
		keyType := fcp.ParseKeyType(result.URI)
		fmt.Fprintf(os.Stderr, "\n✓ Successfully inserted as %s\n", keyType)

		// If it's an insert URI, also show the request URI
		if fcp.IsInsertURI(result.URI) {
			requestURI := fcp.GetRequestURI(result.URI)
			fmt.Fprintf(os.Stderr, "Request URI: %s\n", requestURI)
		}
	}
}

func uploadWithProgress(client *fcp.Client, uri string, data []byte, timeout time.Duration, priority int, compress bool) (*fcp.OperationResult, error) {
	identifier := fmt.Sprintf("put-%d", time.Now().UnixNano())
	debugLog("Upload identifier: %s", identifier)

	resultChan := make(chan *fcp.OperationResult, 1)
	errChan := make(chan error, 1)

	// Progress tracking
	var progressMu sync.Mutex
	var lastTime = time.Now()
	var lastSucceeded int
	var speed float64
	progressStarted := false

	// Register progress handler
	debugLog("Registering progress handler")
	client.RegisterHandler("SimpleProgress", func(m *fcp.Message) error {
		if m.Fields["Identifier"] != identifier {
			return nil
		}

		if !progressStarted {
			debugLog("First progress message received")
			progressStarted = true
		}

		progressMu.Lock()
		defer progressMu.Unlock()

		var succeeded, total int
		fmt.Sscanf(m.Fields["Succeeded"], "%d", &succeeded)
		fmt.Sscanf(m.Fields["Total"], "%d", &total)

		debugLog("Progress: %d/%d blocks", succeeded, total)

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
		if filled > barWidth {
			filled = barWidth
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

		fmt.Fprintf(os.Stderr, "\r[%s] %.1f%% (%d/%d blocks) %.2f KB/s    ",
			bar, percent, succeeded, total, speed)

		return nil
	})

	// Register result handlers
	debugLog("Registering result handlers")
	client.RegisterHandler("PutSuccessful", func(m *fcp.Message) error {
		if m.Fields["Identifier"] != identifier {
			return nil
		}

		debugLog("PutSuccessful received")
		resultChan <- &fcp.OperationResult{
			Success: true,
			URI:     m.Fields["URI"],
		}
		return nil
	})

	client.RegisterHandler("PutFailed", func(m *fcp.Message) error {
		if m.Fields["Identifier"] != identifier {
			return nil
		}

		debugLog("PutFailed: %s - %s", m.Fields["Code"], m.Fields["CodeDescription"])

		code := m.Fields["Code"]
		desc := m.Fields["CodeDescription"]
		errChan <- fmt.Errorf("put failed [%s]: %s", code, desc)
		return nil
	})

	// Send ClientPut message
	msg := &fcp.Message{
		Name: "ClientPut",
		Fields: map[string]string{
			"URI":           uri,
			"Identifier":    identifier,
			"UploadFrom":    "direct",
			"PriorityClass": fmt.Sprintf("%d", priority),
			"Verbosity":     "511", // Maximum progress messages
			"MaxRetries":    "-1",
			"DontCompress":  fmt.Sprintf("%t", !compress),
			"GetCHKOnly":    "false",
			"Global":        "false",
		},
		Data: data,
	}

	debugLog("Sending ClientPut message")
	if err := client.SendMessage(msg); err != nil {
		return nil, fmt.Errorf("failed to send put message: %w", err)
	}

	// Wait for result
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case result := <-resultChan:
			fmt.Fprintf(os.Stderr, "\n")
			if !result.Success {
				return nil, fmt.Errorf("upload failed")
			}
			if !progressStarted {
				debugLog("WARNING: No progress messages were received")
			}
			return result, nil

		case err := <-errChan:
			fmt.Fprintf(os.Stderr, "\n")
			return nil, err

		case <-ticker.C:
			debugLog("Still waiting for upload... (progress started: %v)", progressStarted)

		case <-ctx.Done():
			fmt.Fprintf(os.Stderr, "\n")
			return nil, fmt.Errorf("upload timeout after %v", timeout)
		}
	}
}

func uploadSimple(client *fcp.Client, uri string, data []byte, timeout time.Duration, priority int, compress bool) (*fcp.OperationResult, error) {
	ops := fcp.NewOperations(client)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	return ops.Put(ctx, uri, data)
}
