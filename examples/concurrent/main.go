// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
)

// Example 1: Batch Get Operations
func batchGetExample() {
	client, err := fcp.Connect(fcp.DefaultConfig())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	go client.Listen()
	ops := fcp.NewOperations(client)

	// List of URIs to retrieve concurrently
	uris := []string{
		"KSK@file1",
		"KSK@file2",
		"KSK@file3",
		"CHK@abc123...",
		"USK@site/index.html",
	}

	var wg sync.WaitGroup
	results := make(chan struct {
		uri    string
		result *fcp.OperationResult
		err    error
	}, len(uris))

	// Launch concurrent gets
	for _, uri := range uris {
		wg.Add(1)
		go func(uri string) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			result, err := ops.Get(ctx, uri)
			results <- struct {
				uri    string
				result *fcp.OperationResult
				err    error
			}{uri, result, err}
		}(uri)
	}

	// Wait for all operations to complete
	go func() {
		wg.Wait()
		close(results)
	}()

	// Process results
	successCount := 0
	for res := range results {
		if res.err != nil {
			fmt.Printf("Failed to get %s: %v\n", res.uri, res.err)
		} else if !res.result.Success {
			fmt.Printf("Failed to get %s: %s\n", res.uri, res.result.Error)
		} else {
			fmt.Printf("Successfully retrieved %s (%d bytes)\n", res.uri, len(res.result.Data))
			successCount++
		}
	}

	fmt.Printf("\nCompleted: %d/%d successful\n", successCount, len(uris))
}

// Example 2: Batch Put Operations with Rate Limiting
func batchPutWithRateLimiting() {
	client, err := fcp.Connect(fcp.DefaultConfig())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	go client.Listen()
	ops := fcp.NewOperations(client)

	// Files to upload
	files := map[string][]byte{
		"KSK@doc1": []byte("Document 1 content"),
		"KSK@doc2": []byte("Document 2 content"),
		"KSK@doc3": []byte("Document 3 content"),
		"KSK@doc4": []byte("Document 4 content"),
		"KSK@doc5": []byte("Document 5 content"),
	}

	// Rate limiter: max 2 concurrent uploads
	semaphore := make(chan struct{}, 2)
	var wg sync.WaitGroup
	results := make(chan struct {
		uri    string
		result *fcp.OperationResult
		err    error
	}, len(files))

	for uri, data := range files {
		wg.Add(1)
		go func(uri string, data []byte) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			fmt.Printf("Starting upload: %s\n", uri)
			result, err := ops.Put(ctx, uri, data)

			results <- struct {
				uri    string
				result *fcp.OperationResult
				err    error
			}{uri, result, err}
		}(uri, data)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	for res := range results {
		if res.err != nil {
			fmt.Printf("Failed to put %s: %v\n", res.uri, res.err)
		} else if !res.result.Success {
			fmt.Printf("Failed to put %s: %s\n", res.uri, res.result.Error)
		} else {
			fmt.Printf("Successfully inserted %s -> %s\n", res.uri, res.result.URI)
		}
	}
}

// Example 3: Worker Pool Pattern
type Job struct {
	URI  string
	Data []byte
}

type Result struct {
	Job    Job
	URI    string
	Error  error
}

func workerPoolExample() {
	client, err := fcp.Connect(fcp.DefaultConfig())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	go client.Listen()
	ops := fcp.NewOperations(client)

	// Create job and result channels
	jobs := make(chan Job, 100)
	results := make(chan Result, 100)

	// Start worker pool
	numWorkers := 3
	var wg sync.WaitGroup

	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go worker(i, ops, jobs, results, &wg)
	}

	// Send jobs
	go func() {
		for i := 0; i < 10; i++ {
			jobs <- Job{
				URI:  fmt.Sprintf("KSK@job-%d", i),
				Data: []byte(fmt.Sprintf("Job %d data", i)),
			}
		}
		close(jobs)
	}()

	// Wait for workers to finish
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	for result := range results {
		if result.Error != nil {
			fmt.Printf("Job %s failed: %v\n", result.Job.URI, result.Error)
		} else {
			fmt.Printf("Job %s completed: %s\n", result.Job.URI, result.URI)
		}
	}
}

func worker(id int, ops *fcp.Operations, jobs <-chan Job, results chan<- Result, wg *sync.WaitGroup) {
	defer wg.Done()

	for job := range jobs {
		fmt.Printf("Worker %d processing: %s\n", id, job.URI)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		result, err := ops.Put(ctx, job.URI, job.Data)
		cancel()

		var finalURI string
		var finalErr error

		if err != nil {
			finalErr = err
		} else if !result.Success {
			finalErr = fmt.Errorf(result.Error)
		} else {
			finalURI = result.URI
		}

		results <- Result{
			Job:   job,
			URI:   finalURI,
			Error: finalErr,
		}
	}
}

// Example 4: Concurrent Get with Progress Aggregation
func concurrentGetWithProgress() {
	client, err := fcp.Connect(fcp.DefaultConfig())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	go client.Listen()
	ops := fcp.NewOperations(client)

	uris := []string{
		"KSK@large1",
		"KSK@large2",
		"KSK@large3",
	}

	type Progress struct {
		uri       string
		succeeded int
		total     int
	}

	progressChan := make(chan Progress, 100)
	var wg sync.WaitGroup

	// Start progress display goroutine
	go func() {
		progressMap := make(map[string]Progress)
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case p, ok := <-progressChan:
				if !ok {
					return
				}
				progressMap[p.uri] = p
			case <-ticker.C:
				fmt.Print("\033[2J\033[H") // Clear screen
				fmt.Println("Download Progress:")
				fmt.Println("=================")
				for uri, p := range progressMap {
					if p.total > 0 {
						percent := float64(p.succeeded) / float64(p.total) * 100
						fmt.Printf("%s: %d/%d blocks (%.1f%%)\n", 
							uri, p.succeeded, p.total, percent)
					}
				}
			}
		}
	}()

	// Launch concurrent gets with progress tracking
	for _, uri := range uris {
		wg.Add(1)
		go func(uri string) {
			defer wg.Done()

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			_, err := ops.GetWithProgress(ctx, uri, func(succeeded, total int) {
				progressChan <- Progress{uri, succeeded, total}
			})

			if err != nil {
				fmt.Printf("\nError getting %s: %v\n", uri, err)
			}
		}(uri)
	}

	wg.Wait()
	close(progressChan)
	time.Sleep(time.Second) // Let progress display finish
}

// Example 5: Pipeline Pattern - Get, Process, Put
func pipelineExample() {
	client, err := fcp.Connect(fcp.DefaultConfig())
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	go client.Listen()
	ops := fcp.NewOperations(client)

	// Stage 1: Get data
	getJobs := make(chan string, 10)
	getData := make(chan struct {
		uri  string
		data []byte
	}, 10)

	// Stage 2: Process data
	processedData := make(chan struct {
		originalURI string
		data        []byte
	}, 10)

	// Stage 3: Put processed data
	results := make(chan string, 10)

	var wg sync.WaitGroup

	// Stage 1 workers: Get
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for uri := range getJobs {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				result, err := ops.Get(ctx, uri)
				cancel()

				if err == nil && result.Success {
					getData <- struct {
						uri  string
						data []byte
					}{uri, result.Data}
				}
			}
		}()
	}

	// Stage 2 workers: Process
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range getData {
				// Simulate processing (e.g., compression, encryption, etc.)
				processed := append([]byte("PROCESSED: "), item.data...)

				processedData <- struct {
					originalURI string
					data        []byte
				}{item.uri, processed}
			}
		}()
	}

	// Stage 3 workers: Put
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range processedData {
				newURI := item.originalURI + "-processed"
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
				result, err := ops.Put(ctx, newURI, item.data)
				cancel()

				if err == nil && result.Success {
					results <- result.URI
				}
			}
		}()
	}

	// Send jobs
	go func() {
		for i := 0; i < 5; i++ {
			getJobs <- fmt.Sprintf("KSK@input-%d", i)
		}
		close(getJobs)
	}()

	// Close channels in sequence
	go func() {
		wg.Wait()
		close(getData)
	}()

	go func() {
		wg.Wait()
		close(processedData)
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect final results
	for uri := range results {
		fmt.Printf("Successfully processed and stored: %s\n", uri)
	}
}

func main() {
	fmt.Println("=== Example 1: Batch Get ===")
	batchGetExample()

	fmt.Println("\n=== Example 2: Batch Put with Rate Limiting ===")
	batchPutWithRateLimiting()

	fmt.Println("\n=== Example 3: Worker Pool ===")
	workerPoolExample()

	fmt.Println("\n=== Example 4: Concurrent Get with Progress ===")
	concurrentGetWithProgress()

	fmt.Println("\n=== Example 5: Pipeline Pattern ===")
	pipelineExample()
}
