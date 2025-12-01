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
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
)

var debugMode bool

func debugLog(format string, args ...interface{}) {
	if debugMode {
		log.Printf("[FCPGET] "+format, args...)
	}
}

type DownloadJob struct {
	URI        string
	OutputPath string
	Index      int
	Total      int
}

type DownloadResult struct {
	Job     DownloadJob
	Success bool
	Error   error
	Size    int64
}

func main() {
	// Command line flags
	output := flag.String("o", "", "Output file (default: stdout for single file)")
	outputDir := flag.String("d", ".", "Output directory (for multiple files)")
	host := flag.String("host", "localhost", "Hyphanet node hostname")
	port := flag.Int("port", 9481, "Hyphanet node port")
	timeout := flag.Duration("timeout", 30*time.Minute, "Operation timeout per file")
	progress := flag.Bool("progress", true, "Show download progress")
	quiet := flag.Bool("q", false, "Quiet mode (no progress to stderr)")
	verbose := flag.Bool("v", false, "Verbose output")
	debug := flag.Bool("debug", false, "Enable debug logging")
	maxRetries := flag.Int("retries", 3, "Number of retries on failure")
	showVersion := flag.Bool("version", false, "Show version and license information")
	showLicense := flag.Bool("license", false, "Show license information")
	showSource := flag.Bool("source", false, "Show source code URL")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "fcpget - Retrieve data from Hyphanet v%s\n\n", fcp.Version)
		fmt.Fprintf(os.Stderr, "Usage: fcpget [options] <URI> [URI2 URI3 ...]\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  # Download to stdout\n")
		fmt.Fprintf(os.Stderr, "  fcpget KSK@mykey\n\n")
		fmt.Fprintf(os.Stderr, "  # Download to file\n")
		fmt.Fprintf(os.Stderr, "  fcpget KSK@mykey -o output.txt\n\n")
		fmt.Fprintf(os.Stderr, "  # Download with progress\n")
		fmt.Fprintf(os.Stderr, "  fcpget USK@site/index.html -o index.html --progress\n\n")
		fmt.Fprintf(os.Stderr, "  # Download multiple files\n")
		fmt.Fprintf(os.Stderr, "  fcpget CHK@.../file1.txt CHK@.../file2.jpg -d downloads/\n\n")
		fmt.Fprintf(os.Stderr, "  # Pipe to tar\n")
		fmt.Fprintf(os.Stderr, "  fcpget CHK@... | tar xzf -\n\n")
		fmt.Fprintf(os.Stderr, "  # Debug stuck downloads\n")
		fmt.Fprintf(os.Stderr, "  fcpget CHK@... --debug --progress\n\n")
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

	if flag.NArg() < 1 {
		fmt.Fprintf(os.Stderr, "Error: URI required\n\n")
		flag.Usage()
		os.Exit(1)
	}

	uris := flag.Args()
	debugLog("URIs to download: %d", len(uris))

	// Connect to Hyphanet node
	config := &fcp.Config{
		Host:    *host,
		Port:    *port,
		Name:    "fcpget",
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

	// Handle single file to stdout
	if len(uris) == 1 && (*output == "" || *output == "-") {
		err := downloadToStdout(client, uris[0], *timeout, *maxRetries, showProgress, *quiet)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
		return
	}

	// Create output directory if needed
	if len(uris) > 1 || (*output == "" && len(uris) == 1) {
		if err := os.MkdirAll(*outputDir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error: Failed to create output directory: %v\n", err)
			os.Exit(1)
		}
	}

	// Prepare download jobs
	var jobs []DownloadJob
	for i, uri := range uris {
		var outPath string
		if len(uris) == 1 && *output != "" && *output != "-" {
			outPath = *output
		} else {
			filename := extractFilename(uri)
			if filename == "" {
				filename = fmt.Sprintf("download-%d", i+1)
			}
			outPath = filepath.Join(*outputDir, filename)
		}

		jobs = append(jobs, DownloadJob{
			URI:        uri,
			OutputPath: outPath,
			Index:      i + 1,
			Total:      len(uris),
		})
	}

	// Download files
	results := make(chan DownloadResult, len(jobs))
	var wg sync.WaitGroup

	for _, job := range jobs {
		wg.Add(1)
		go func(j DownloadJob) {
			defer wg.Done()

			result := DownloadResult{Job: j}

			for attempt := 1; attempt <= *maxRetries; attempt++ {
				if attempt > 1 && !*quiet {
					fmt.Printf("\n[%d/%d] Retrying %s (attempt %d/%d)...\n",
						j.Index, j.Total, j.URI, attempt, *maxRetries)
					time.Sleep(time.Duration(attempt) * 2 * time.Second)
				}

				err := downloadFile(client, j, *timeout, showProgress, *quiet)
				if err == nil {
					result.Success = true
					break
				}

				result.Error = err
				debugLog("Download attempt %d failed: %v", attempt, err)

				if attempt == *maxRetries {
					break
				}
			}

			results <- result
		}(job)
	}

	// Wait for all downloads
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	successCount := 0
	failCount := 0

	for result := range results {
		if result.Success {
			successCount++
			if !*quiet {
				fmt.Printf("\n✓ Downloaded: %s → %s\n", result.Job.URI, result.Job.OutputPath)
			}
		} else {
			failCount++
			fmt.Fprintf(os.Stderr, "\n✗ Failed: %s - %v\n", result.Job.URI, result.Error)
		}
	}

	// Summary
	if !*quiet && len(jobs) > 1 {
		fmt.Printf("\n")
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Printf("Download Summary: %d succeeded, %d failed\n", successCount, failCount)
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	}

	if failCount > 0 {
		os.Exit(1)
	}
}

func downloadToStdout(client *fcp.Client, uri string, timeout time.Duration, maxRetries int, showProgress, quiet bool) error {
	ops := fcp.NewOperations(client)

	var result *fcp.OperationResult
	var err error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 && !quiet {
			fmt.Fprintf(os.Stderr, "Retry %d/%d...\n", attempt, maxRetries)
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)

		if showProgress {
			result, err = ops.GetWithProgress(ctx, uri, func(succeeded, total int) {
				percent := 0.0
				if total > 0 {
					percent = float64(succeeded) / float64(total) * 100
				}
				fmt.Fprintf(os.Stderr, "\rProgress: %d/%d blocks (%.1f%%)    ",
					succeeded, total, percent)
			})
			if err == nil {
				fmt.Fprintf(os.Stderr, "\n")
			}
		} else {
			result, err = ops.Get(ctx, uri)
		}

		cancel()

		if err == nil && result != nil && result.Success {
			break
		}
	}

	if err != nil {
		return fmt.Errorf("get operation failed: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("get failed: %s", result.Error)
	}

	// Write to stdout
	if _, err := os.Stdout.Write(result.Data); err != nil {
		return fmt.Errorf("failed to write output: %w", err)
	}

	if !quiet {
		fmt.Fprintf(os.Stderr, "Successfully retrieved %d bytes\n", len(result.Data))
	}

	return nil
}

func downloadFile(client *fcp.Client, job DownloadJob, timeout time.Duration, showProgress, quiet bool) error {
	identifier := fmt.Sprintf("get-%d", time.Now().UnixNano())
	debugLog("Download identifier: %s for URI: %s", identifier, job.URI)

	resultChan := make(chan *fcp.OperationResult, 1)
	errChan := make(chan error, 1)

	// Progress tracking
	var progressMu sync.Mutex
	var lastTime = time.Now()
	var lastSucceeded int
	var speed float64
	progressStarted := false

	if !quiet {
		if job.Total == 1 {
			fmt.Printf("Downloading: %s\n", job.URI)
			fmt.Printf("Output: %s\n", job.OutputPath)
		} else {
			fmt.Printf("\n[%d/%d] Downloading: %s\n", job.Index, job.Total, job.URI)
			fmt.Printf("Output: %s\n", job.OutputPath)
		}
	}

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

		if !showProgress {
			return nil
		}

		progressMu.Lock()
		defer progressMu.Unlock()

		var succeeded, total, required int
		fmt.Sscanf(m.Fields["Succeeded"], "%d", &succeeded)
		fmt.Sscanf(m.Fields["Total"], "%d", &total)
		fmt.Sscanf(m.Fields["Required"], "%d", &required)

		debugLog("Progress: %d/%d blocks (required: %d)", succeeded, total, required)

		// Use required if available, otherwise total
		denominator := required
		if denominator == 0 {
			denominator = total
		}

		percent := 0.0
		if denominator > 0 {
			percent = float64(succeeded) / float64(denominator) * 100
		}

		// Cap at 100%
		if percent > 100 {
			percent = 100
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

		// Show which denominator we're using
		displayTotal := denominator
		if displayTotal == 0 {
			displayTotal = total
		}

		fmt.Fprintf(os.Stderr, "\r[%s] %.1f%% (%d/%d blocks) %.2f KB/s    ",
			bar, percent, succeeded, displayTotal, speed)

		return nil
	})

	// Register data received handler
	debugLog("Registering data handlers")
	client.RegisterHandler("AllData", func(m *fcp.Message) error {
		if m.Fields["Identifier"] != identifier {
			return nil
		}

		debugLog("AllData received, size: %d bytes", len(m.Data))

		// Write data to file
		if err := os.WriteFile(job.OutputPath, m.Data, 0644); err != nil {
			errChan <- fmt.Errorf("failed to write file: %w", err)
			return nil
		}

		resultChan <- &fcp.OperationResult{
			Success: true,
			Data:    m.Data,
		}
		return nil
	})

	client.RegisterHandler("GetFailed", func(m *fcp.Message) error {
		if m.Fields["Identifier"] != identifier {
			return nil
		}

		debugLog("GetFailed: %s - %s", m.Fields["Code"], m.Fields["CodeDescription"])

		code := m.Fields["Code"]
		desc := m.Fields["CodeDescription"]
		errChan <- fmt.Errorf("get failed [%s]: %s", code, desc)
		return nil
	})

	client.RegisterHandler("DataFound", func(m *fcp.Message) error {
		if m.Fields["Identifier"] != identifier {
			return nil
		}

		debugLog("DataFound - download starting")
		if !quiet && showProgress {
			fmt.Fprintf(os.Stderr, "Data found, starting download...\n")
		}
		return nil
	})

	// Send ClientGet request
	msg := &fcp.Message{
		Name: "ClientGet",
		Fields: map[string]string{
			"URI":           job.URI,
			"Identifier":    identifier,
			"ReturnType":    "direct",
			"Verbosity":     "511", // Maximum progress messages
			"MaxRetries":    "-1",
			"PriorityClass": "2", // Higher priority for interactive downloads
		},
	}

	debugLog("Sending ClientGet request")
	if err := client.SendMessage(msg); err != nil {
		return fmt.Errorf("failed to send get request: %w", err)
	}

	// Wait for result
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case result := <-resultChan:
			if showProgress {
				fmt.Fprintf(os.Stderr, "\n")
			}
			if !result.Success {
				return fmt.Errorf("download failed")
			}
			if !progressStarted {
				debugLog("WARNING: No progress messages were received")
			}
			return nil

		case err := <-errChan:
			if showProgress {
				fmt.Fprintf(os.Stderr, "\n")
			}
			return err

		case <-ticker.C:
			debugLog("Still waiting for download... (progress started: %v)", progressStarted)

		case <-ctx.Done():
			if showProgress {
				fmt.Fprintf(os.Stderr, "\n")
			}
			return fmt.Errorf("download timeout after %v", timeout)
		}
	}
}

func extractFilename(uri string) string {
	// Try to extract filename from URI
	// Format: CHK@.../filename or USK@.../site/version/filename

	parts := strings.Split(uri, "/")
	if len(parts) > 0 {
		lastPart := parts[len(parts)-1]
		// Remove trailing slashes
		lastPart = strings.TrimSuffix(lastPart, "/")
		if lastPart != "" && !strings.Contains(lastPart, "@") {
			return lastPart
		}
	}

	// If we can't extract a filename, return empty
	return ""
}
