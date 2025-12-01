// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package keepalive

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
)

// ReinserterState represents the state of the reinserter
type ReinserterState int

const (
	ReinserterIdle ReinserterState = iota
	ReinserterRunning
	ReinserterPaused
	ReinserterStopping
)

// String returns the string representation of ReinserterState
func (s ReinserterState) String() string {
	switch s {
	case ReinserterIdle:
		return "idle"
	case ReinserterRunning:
		return "running"
	case ReinserterPaused:
		return "paused"
	case ReinserterStopping:
		return "stopping"
	default:
		return "unknown"
	}
}

// Reinserter manages the reinsertion process for sites
type Reinserter struct {
	mu sync.RWMutex

	client      *fcp.Client
	config      *Config
	siteManager *SiteManager
	storage     *Storage

	fetcher  *Fetcher
	inserter *Inserter

	state     ReinserterState
	activeSite *Site

	// Control channels
	stopCh   chan struct{}
	pauseCh  chan struct{}
	resumeCh chan struct{}

	// Progress callback
	onProgress func(*Site, *Segment, string)
	onComplete func(*Site, bool, string)
}

// NewReinserter creates a new reinserter
func NewReinserter(client *fcp.Client, config *Config, siteManager *SiteManager, storage *Storage) *Reinserter {
	return &Reinserter{
		client:      client,
		config:      config,
		siteManager: siteManager,
		storage:     storage,
		fetcher:     NewFetcher(client),
		inserter:    NewInserter(client),
		state:       ReinserterIdle,
	}
}

// SetProgressCallback sets the progress callback
func (r *Reinserter) SetProgressCallback(cb func(*Site, *Segment, string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onProgress = cb
}

// SetCompleteCallback sets the completion callback
func (r *Reinserter) SetCompleteCallback(cb func(*Site, bool, string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onComplete = cb
}

// Start begins reinsertion for a site
func (r *Reinserter) Start(site *Site) error {
	r.mu.Lock()
	if r.state == ReinserterRunning {
		r.mu.Unlock()
		return fmt.Errorf("reinserter is already running")
	}

	r.state = ReinserterRunning
	r.activeSite = site
	r.stopCh = make(chan struct{})
	r.pauseCh = make(chan struct{})
	r.resumeCh = make(chan struct{})
	r.mu.Unlock()

	// Update config
	r.config.SetActiveSiteID(site.ID)

	// Start reinsertion in background
	go r.run(site)

	return nil
}

// Stop stops the current reinsertion
func (r *Reinserter) Stop() {
	r.mu.Lock()
	if r.state != ReinserterRunning && r.state != ReinserterPaused {
		r.mu.Unlock()
		return
	}
	r.state = ReinserterStopping
	close(r.stopCh)
	r.mu.Unlock()
}

// Pause pauses the current reinsertion
func (r *Reinserter) Pause() {
	r.mu.Lock()
	if r.state != ReinserterRunning {
		r.mu.Unlock()
		return
	}
	r.state = ReinserterPaused
	r.pauseCh <- struct{}{}
	r.mu.Unlock()
}

// Resume resumes a paused reinsertion
func (r *Reinserter) Resume() {
	r.mu.Lock()
	if r.state != ReinserterPaused {
		r.mu.Unlock()
		return
	}
	r.state = ReinserterRunning
	r.resumeCh <- struct{}{}
	r.mu.Unlock()
}

// GetState returns the current state
func (r *Reinserter) GetState() ReinserterState {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.state
}

// GetActiveSite returns the active site
func (r *Reinserter) GetActiveSite() *Site {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.activeSite
}

// run is the main reinsertion loop
func (r *Reinserter) run(site *Site) {
	site.mu.Lock()
	site.State = SiteReinserting
	site.StartedAt = time.Now()
	site.mu.Unlock()

	site.AddLog(0, "Starting reinsertion for %s", site.URI)
	r.notifyProgress(site, nil, "Starting reinsertion")

	success := true
	errMsg := ""

	// Process each segment
	for i := 0; i < site.SegmentCount(); i++ {
		select {
		case <-r.stopCh:
			site.AddLog(0, "Reinsertion stopped by user")
			success = false
			errMsg = "stopped by user"
			goto done

		case <-r.pauseCh:
			site.AddLog(0, "Reinsertion paused")
			r.notifyProgress(site, nil, "Paused")
			// Wait for resume or stop
			select {
			case <-r.resumeCh:
				site.AddLog(0, "Reinsertion resumed")
			case <-r.stopCh:
				success = false
				errMsg = "stopped while paused"
				goto done
			}

		default:
		}

		segment := site.GetSegment(i)
		if segment == nil {
			continue
		}

		site.mu.Lock()
		site.CurrentSegment = i
		site.mu.Unlock()

		if err := r.processSegment(site, segment); err != nil {
			site.AddLog(0, "Segment %d failed: %s", i, err)
			segment.mu.Lock()
			segment.State = SegmentFailed
			segment.mu.Unlock()
			success = false
			errMsg = err.Error()
		}
	}

done:
	// Update final state
	site.mu.Lock()
	if success {
		site.State = SiteComplete
	} else {
		site.State = SiteError
		site.Error = errMsg
	}
	site.CompletedAt = time.Now()
	site.mu.Unlock()

	// Save stats
	r.siteManager.SaveSiteStats(site)

	// Update reinserter state
	r.mu.Lock()
	r.state = ReinserterIdle
	r.activeSite = nil
	r.mu.Unlock()

	r.config.SetActiveSiteID(-1)

	site.AddLog(0, "Reinsertion complete (success=%v)", success)
	r.notifyComplete(site, success, errMsg)
}

// processSegment handles a single segment
func (r *Reinserter) processSegment(site *Site, segment *Segment) error {
	segment.mu.Lock()
	segment.State = SegmentTesting
	segment.mu.Unlock()

	site.AddLog(1, "Processing segment %d (%d blocks)", segment.ID, segment.Size())
	r.notifyProgress(site, segment, "Testing availability")

	// Step 1: Sample availability test
	testSize := r.config.GetTestSize()
	tolerance := r.config.GetTolerance()

	availability, err := r.testSegmentAvailability(site, segment, testSize)
	if err != nil {
		return fmt.Errorf("availability test failed: %w", err)
	}

	segment.mu.Lock()
	segment.Availability = availability
	segment.mu.Unlock()

	site.AddLog(1, "Segment %d availability: %.1f%%", segment.ID, availability)

	// Step 2: Skip if availability is above tolerance
	if availability >= float64(tolerance) {
		site.AddLog(1, "Segment %d skipped (%.1f%% >= %d%%)", segment.ID, availability, tolerance)
		segment.mu.Lock()
		segment.State = SegmentSkipped
		segment.mu.Unlock()
		r.notifyProgress(site, segment, "Skipped - good availability")
		return nil
	}

	// Step 3: Heal the segment - fetch all remaining blocks and reinsert missing ones
	segment.mu.Lock()
	segment.State = SegmentHealing
	segment.mu.Unlock()

	site.AddLog(1, "Healing segment %d", segment.ID)
	r.notifyProgress(site, segment, "Healing segment")

	if err := r.healSegment(site, segment); err != nil {
		return fmt.Errorf("segment healing failed: %w", err)
	}

	segment.mu.Lock()
	segment.State = SegmentComplete
	segment.mu.Unlock()

	site.AddLog(1, "Segment %d complete", segment.ID)
	r.notifyProgress(site, segment, "Complete")

	return nil
}

// testSegmentAvailability tests availability by sampling blocks
func (r *Reinserter) testSegmentAvailability(site *Site, segment *Segment, testPercent int) (float64, error) {
	segment.mu.RLock()
	blocks := segment.Blocks
	segment.mu.RUnlock()

	if len(blocks) == 0 {
		return 100.0, nil
	}

	// Calculate sample size
	sampleSize := len(blocks) * testPercent / 100
	if sampleSize < 1 {
		sampleSize = 1
	}
	if sampleSize > len(blocks) {
		sampleSize = len(blocks)
	}

	// Random sample selection
	indices := rand.Perm(len(blocks))[:sampleSize]
	sampled := make([]*Block, sampleSize)
	for i, idx := range indices {
		sampled[i] = blocks[idx]
	}

	site.AddLog(2, "Testing %d of %d blocks in segment %d", sampleSize, len(blocks), segment.ID)

	// Fetch sampled blocks
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	workers := r.config.GetPower()
	resultChan := make(chan *FetchResult, sampleSize)

	go r.fetcher.FetchBlocks(ctx, sampled, workers, resultChan)

	// Collect results
	successCount := 0
	totalCount := 0

	for result := range resultChan {
		totalCount++
		if result.Success {
			successCount++
		}
	}

	if totalCount == 0 {
		return 0, nil
	}

	return float64(successCount) / float64(totalCount) * 100, nil
}

// healSegment fetches all missing blocks and reinserts them
func (r *Reinserter) healSegment(site *Site, segment *Segment) error {
	segment.mu.RLock()
	blocks := segment.Blocks
	segment.mu.RUnlock()

	// Find blocks that need fetching (not yet successfully fetched)
	var toFetch []*Block
	for _, block := range blocks {
		block.mu.RLock()
		needsFetch := !block.FetchOK
		block.mu.RUnlock()
		if needsFetch {
			toFetch = append(toFetch, block)
		}
	}

	if len(toFetch) > 0 {
		site.AddLog(2, "Fetching %d blocks", len(toFetch))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		workers := r.config.GetPower()
		resultChan := make(chan *FetchResult, len(toFetch))

		go r.fetcher.FetchBlocks(ctx, toFetch, workers, resultChan)

		// Collect results
		for result := range resultChan {
			if result.Success {
				site.AddLog(2, "Fetched block %d:%d", result.Block.SegmentID, result.Block.BlockID)
			}
		}
	}

	// Update segment stats
	segment.UpdateStats()

	// Find blocks that failed fetching (need reinsertion)
	var toInsert []*Block
	for _, block := range blocks {
		block.mu.RLock()
		failed := block.FetchDone && !block.FetchOK
		hasData := len(block.Data) > 0
		block.mu.RUnlock()

		// We can only insert blocks we have data for
		// In a real implementation, we'd use FEC to recover missing blocks
		if failed && hasData {
			toInsert = append(toInsert, block)
		}
	}

	if len(toInsert) > 0 {
		site.AddLog(2, "Reinserting %d blocks", len(toInsert))

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		workers := r.config.GetPower()
		resultChan := make(chan *InsertResult, len(toInsert))

		go r.inserter.InsertBlocks(ctx, toInsert, workers, resultChan)

		// Collect results
		for result := range resultChan {
			if result.Success {
				site.AddLog(2, "Reinserted block %d:%d", result.Block.SegmentID, result.Block.BlockID)
			}
		}
	}

	// Final stats update
	segment.UpdateStats()

	return nil
}

// notifyProgress sends a progress update
func (r *Reinserter) notifyProgress(site *Site, segment *Segment, message string) {
	r.mu.RLock()
	cb := r.onProgress
	r.mu.RUnlock()

	if cb != nil {
		cb(site, segment, message)
	}
}

// notifyComplete sends a completion notification
func (r *Reinserter) notifyComplete(site *Site, success bool, errMsg string) {
	r.mu.RLock()
	cb := r.onComplete
	r.mu.RUnlock()

	if cb != nil {
		cb(site, success, errMsg)
	}
}

// GetStats returns reinserter statistics
func (r *Reinserter) GetStats() (fetchTotal, fetchSuccess, fetchFail, insertTotal, insertSuccess, insertFail int) {
	ft, fs, ff := r.fetcher.GetStats()
	it, is, if_ := r.inserter.GetStats()
	return ft, fs, ff, it, is, if_
}

// ResetStats resets all statistics
func (r *Reinserter) ResetStats() {
	r.fetcher.ResetStats()
	r.inserter.ResetStats()
}
