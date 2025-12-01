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

// InsertResult contains the result of a block insert
type InsertResult struct {
	Block   *Block
	Success bool
	URI     string // Returned URI (should match original)
	Error   string
}

// Inserter handles block insertion via FCP
type Inserter struct {
	mu sync.RWMutex

	client     *fcp.Client
	maxRetries int
	timeout    time.Duration

	// Stats
	insertCount  int
	successCount int
	failCount    int
}

// NewInserter creates a new block inserter
func NewInserter(client *fcp.Client) *Inserter {
	return &Inserter{
		client:     client,
		maxRetries: 3,
		timeout:    120 * time.Second,
	}
}

// SetTimeout sets the insert timeout
func (i *Inserter) SetTimeout(timeout time.Duration) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.timeout = timeout
}

// SetMaxRetries sets the maximum retry count
func (i *Inserter) SetMaxRetries(retries int) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.maxRetries = retries
}

// InsertBlock inserts a single block
func (i *Inserter) InsertBlock(ctx context.Context, block *Block) *InsertResult {
	i.mu.Lock()
	i.insertCount++
	maxRetries := i.maxRetries
	timeout := i.timeout
	i.mu.Unlock()

	// Check if block has data
	block.mu.RLock()
	data := block.Data
	uri := block.URI
	block.mu.RUnlock()

	if len(data) == 0 {
		result := &InsertResult{
			Block: block,
			Error: "no data to insert",
		}
		block.SetInsertResult(false, result.Error)
		return result
	}

	block.mu.Lock()
	block.State = BlockInserting
	block.mu.Unlock()

	result := &InsertResult{
		Block: block,
	}

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx.Err() != nil {
			result.Error = "context cancelled"
			break
		}

		insertedURI, err := i.doInsert(ctx, uri, data, timeout)
		if err == nil {
			result.Success = true
			result.URI = insertedURI

			i.mu.Lock()
			i.successCount++
			i.mu.Unlock()

			block.SetInsertResult(true, "")
			return result
		}

		lastErr = err

		// Don't retry on permanent failures
		if isPermanentInsertError(err) {
			break
		}

		// Brief pause before retry
		select {
		case <-ctx.Done():
			result.Error = "context cancelled"
			block.SetInsertResult(false, result.Error)
			return result
		case <-time.After(time.Duration(attempt+1) * time.Second):
		}
	}

	i.mu.Lock()
	i.failCount++
	i.mu.Unlock()

	if lastErr != nil {
		result.Error = lastErr.Error()
	} else {
		result.Error = "insert failed"
	}

	block.SetInsertResult(false, result.Error)
	return result
}

// doInsert performs the actual FCP insert
func (i *Inserter) doInsert(ctx context.Context, uri string, data []byte, timeout time.Duration) (string, error) {
	// Create a unique identifier for this insert
	identifier := fmt.Sprintf("gokeepalive-insert-%d", time.Now().UnixNano())

	// Build ClientPut message
	msg := &fcp.Message{
		Name: "ClientPut",
		Fields: map[string]string{
			"URI":           uri,
			"Identifier":    identifier,
			"Verbosity":     "0",
			"MaxRetries":    "0",     // We handle retries ourselves
			"PriorityClass": "3",     // Low priority
			"GetCHKOnly":    "false",
			"DontCompress":  "true",  // Block is already compressed
			"UploadFrom":    "direct",
		},
		Data: data,
	}

	// Send the request
	if err := i.client.SendMessage(msg); err != nil {
		return "", fmt.Errorf("failed to send ClientPut: %w", err)
	}

	// Wait for response with timeout
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			// Try to cancel the insert
			cancelMsg := &fcp.Message{
				Name: "RemoveRequest",
				Fields: map[string]string{
					"Identifier": identifier,
					"Global":     "false",
				},
			}
			i.client.SendMessage(cancelMsg)
			return "", fmt.Errorf("insert timeout")

		default:
			resp, err := i.client.ReceiveMessage()
			if err != nil {
				return "", fmt.Errorf("failed to read response: %w", err)
			}

			// Check if this is our response
			if resp.Fields["Identifier"] != identifier {
				continue
			}

			switch resp.Name {
			case "PutSuccessful":
				return resp.Fields["URI"], nil

			case "URIGenerated":
				// For CHK inserts, the final URI is generated
				// Continue waiting for PutSuccessful
				continue

			case "SimpleProgress":
				// Progress update, continue waiting
				continue

			case "PutFailed":
				code := resp.Fields["Code"]
				desc := resp.Fields["CodeDescription"]
				if desc == "" {
					desc = resp.Fields["ShortCodeDescription"]
				}
				return "", fmt.Errorf("insert failed: %s (%s)", desc, code)

			case "IdentifierCollision":
				return "", fmt.Errorf("identifier collision")

			case "ProtocolError":
				return "", fmt.Errorf("protocol error: %s", resp.Fields["CodeDescription"])
			}
		}
	}
}

// InsertBlocks inserts multiple blocks concurrently
func (i *Inserter) InsertBlocks(ctx context.Context, blocks []*Block, workers int, resultChan chan<- *InsertResult) {
	if workers <= 0 {
		workers = 1
	}

	// Filter blocks that have data
	toInsert := make([]*Block, 0, len(blocks))
	for _, block := range blocks {
		block.mu.RLock()
		hasData := len(block.Data) > 0
		block.mu.RUnlock()
		if hasData {
			toInsert = append(toInsert, block)
		}
	}

	if len(toInsert) == 0 {
		if resultChan != nil {
			close(resultChan)
		}
		return
	}

	// Create work channel
	workChan := make(chan *Block, len(toInsert))
	for _, block := range toInsert {
		workChan <- block
	}
	close(workChan)

	// Start workers
	var wg sync.WaitGroup
	for j := 0; j < workers; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for block := range workChan {
				if ctx.Err() != nil {
					return
				}
				result := i.InsertBlock(ctx, block)
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

// GetStats returns insert statistics
func (i *Inserter) GetStats() (total, success, fail int) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	return i.insertCount, i.successCount, i.failCount
}

// ResetStats resets insert statistics
func (i *Inserter) ResetStats() {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.insertCount = 0
	i.successCount = 0
	i.failCount = 0
}

// isPermanentInsertError checks if the error is permanent (no retry)
func isPermanentInsertError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// These error codes indicate permanent failure
	permanentCodes := []string{
		"CollisionNotFound",
		"FatalError",
		"InternalError",
	}
	for _, code := range permanentCodes {
		if contains(errStr, code) {
			return true
		}
	}
	return false
}

// TestInserter is a mock inserter for testing
type TestInserter struct {
	Results map[string]bool // URI -> success
}

// NewTestInserter creates a test inserter
func NewTestInserter() *TestInserter {
	return &TestInserter{
		Results: make(map[string]bool),
	}
}

// InsertBlock simulates a block insert
func (i *TestInserter) InsertBlock(ctx context.Context, block *Block) *InsertResult {
	result := &InsertResult{
		Block: block,
	}

	block.mu.RLock()
	uri := block.URI
	hasData := len(block.Data) > 0
	block.mu.RUnlock()

	if !hasData {
		result.Error = "no data to insert"
		block.SetInsertResult(false, result.Error)
		return result
	}

	if success, ok := i.Results[uri]; ok && success {
		result.Success = true
		result.URI = uri
		block.SetInsertResult(true, "")
	} else {
		result.Error = "test: insert failed"
		block.SetInsertResult(false, result.Error)
	}

	return result
}
