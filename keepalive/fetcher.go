// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package keepalive

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
)

// FetchResult contains the result of a block fetch
type FetchResult struct {
	Block   *Block
	Success bool
	Data    []byte
	Error   string
}

// Fetcher handles block fetching via FCP
type Fetcher struct {
	mu sync.RWMutex

	client      *fcp.Client
	maxRetries  int
	timeout     time.Duration
	ignoreStore bool // Bypass local datastore

	// Stats
	fetchCount   int
	successCount int
	failCount    int
}

// NewFetcher creates a new block fetcher
func NewFetcher(client *fcp.Client) *Fetcher {
	return &Fetcher{
		client:      client,
		maxRetries:  3,
		timeout:     60 * time.Second,
		ignoreStore: true, // For availability testing
	}
}

// SetTimeout sets the fetch timeout
func (f *Fetcher) SetTimeout(timeout time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.timeout = timeout
}

// SetIgnoreStore sets whether to bypass the local datastore
func (f *Fetcher) SetIgnoreStore(ignore bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ignoreStore = ignore
}

// SetMaxRetries sets the maximum retry count
func (f *Fetcher) SetMaxRetries(retries int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.maxRetries = retries
}

// FetchBlock fetches a single block
func (f *Fetcher) FetchBlock(ctx context.Context, block *Block) *FetchResult {
	f.mu.Lock()
	f.fetchCount++
	ignoreStore := f.ignoreStore
	maxRetries := f.maxRetries
	timeout := f.timeout
	f.mu.Unlock()

	block.mu.Lock()
	block.State = BlockFetching
	block.mu.Unlock()

	result := &FetchResult{
		Block: block,
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx.Err() != nil {
			result.Error = "context cancelled"
			break
		}

		data, err := f.doFetch(ctx, block.URI, ignoreStore, timeout)
		if err == nil {
			result.Success = true
			result.Data = data

			f.mu.Lock()
			f.successCount++
			f.mu.Unlock()

			block.SetFetchResult(true, data, "")
			return result
		}

		lastErr = err

		// Don't retry on permanent failures
		if isPermanentFetchError(err) {
			break
		}

		// Brief pause before retry
		select {
		case <-ctx.Done():
			result.Error = "context cancelled"
			block.SetFetchResult(false, nil, result.Error)
			return result
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}

	f.mu.Lock()
	f.failCount++
	f.mu.Unlock()

	if lastErr != nil {
		result.Error = lastErr.Error()
	} else {
		result.Error = "fetch failed"
	}

	block.SetFetchResult(false, nil, result.Error)
	return result
}

// doFetch performs the actual FCP fetch
func (f *Fetcher) doFetch(ctx context.Context, uri string, ignoreStore bool, timeout time.Duration) ([]byte, error) {
	// Create a unique identifier for this fetch
	identifier := fmt.Sprintf("gokeepalive-fetch-%d", time.Now().UnixNano())

	// Build ClientGet message
	fields := map[string]string{
		"URI":           uri,
		"Identifier":    identifier,
		"Verbosity":     "0",
		"ReturnType":    "direct",
		"MaxSize":       "32768", // CHK block size
		"MaxRetries":    "0",     // We handle retries ourselves
		"PriorityClass": "3",     // Low priority
	}

	if ignoreStore {
		fields["IgnoreDS"] = "true"
	}

	msg := &fcp.Message{
		Name:   "ClientGet",
		Fields: fields,
	}

	// Send the request
	if err := f.client.SendMessage(msg); err != nil {
		return nil, fmt.Errorf("failed to send ClientGet: %w", err)
	}

	// Wait for response with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			// Try to cancel the fetch
			cancelMsg := &fcp.Message{
				Name: "RemoveRequest",
				Fields: map[string]string{
					"Identifier": identifier,
					"Global":     "false",
				},
			}
			f.client.SendMessage(cancelMsg)
			return nil, fmt.Errorf("fetch timeout")

		default:
			resp, err := f.client.ReceiveMessage()
			if err != nil {
				return nil, fmt.Errorf("failed to read response: %w", err)
			}

			// Check if this is our response
			if resp.Fields["Identifier"] != identifier {
				continue
			}

			switch resp.Name {
			case "AllData":
				// Success - data is in the message
				return resp.Data, nil

			case "DataFound":
				// Wait for AllData to follow
				continue

			case "GetFailed":
				code := resp.Fields["Code"]
				desc := resp.Fields["CodeDescription"]
				if desc == "" {
					desc = resp.Fields["ShortCodeDescription"]
				}
				return nil, fmt.Errorf("fetch failed: %s (%s)", desc, code)

			case "IdentifierCollision":
				return nil, fmt.Errorf("identifier collision")

			case "ProtocolError":
				return nil, fmt.Errorf("protocol error: %s", resp.Fields["CodeDescription"])
			}
		}
	}
}

// FetchBlocks fetches multiple blocks concurrently
func (f *Fetcher) FetchBlocks(ctx context.Context, blocks []*Block, workers int, resultChan chan<- *FetchResult) {
	if workers <= 0 {
		workers = 1
	}

	// Create work channel
	workChan := make(chan *Block, len(blocks))
	for _, block := range blocks {
		workChan <- block
	}
	close(workChan)

	// Start workers
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for block := range workChan {
				if ctx.Err() != nil {
					return
				}
				result := f.FetchBlock(ctx, block)
				if resultChan != nil {
					resultChan <- result
				}
			}
		}()
	}

	wg.Wait()
	if resultChan != nil {
		close(resultChan)
	}
}

// GetStats returns fetch statistics
func (f *Fetcher) GetStats() (total, success, fail int) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.fetchCount, f.successCount, f.failCount
}

// ResetStats resets fetch statistics
func (f *Fetcher) ResetStats() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetchCount = 0
	f.successCount = 0
	f.failCount = 0
}

// isPermanentFetchError checks if the error is permanent (no retry)
func isPermanentFetchError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// These error codes indicate the data doesn't exist
	permanentCodes := []string{
		"DataNotFound",
		"RouteNotFound",
		"RejectedOverload",
		"FatalError",
	}
	for _, code := range permanentCodes {
		if contains(errStr, code) {
			return true
		}
	}
	return false
}

// contains checks if s contains substr
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestFetcher is a mock fetcher for testing
type TestFetcher struct {
	Results map[string]bool // URI -> success
}

// NewTestFetcher creates a test fetcher
func NewTestFetcher() *TestFetcher {
	return &TestFetcher{
		Results: make(map[string]bool),
	}
}

// FetchBlock simulates a block fetch
func (f *TestFetcher) FetchBlock(ctx context.Context, block *Block) *FetchResult {
	result := &FetchResult{
		Block: block,
	}

	if success, ok := f.Results[block.URI]; ok && success {
		result.Success = true
		result.Data = make([]byte, 32768) // Simulated block data
		block.SetFetchResult(true, result.Data, "")
	} else {
		result.Error = "test: not found"
		block.SetFetchResult(false, nil, result.Error)
	}

	return result
}
