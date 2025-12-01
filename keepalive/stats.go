// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package keepalive

import (
	"fmt"
	"sync"
	"time"
)

// StatsCollector collects and reports statistics
type StatsCollector struct {
	mu sync.RWMutex

	// Session stats
	sessionStart time.Time
	sitesProcessed int
	segmentsProcessed int
	segmentsSkipped int
	segmentsHealed int
	segmentsFailed int

	// Block stats
	blocksFetched int
	blocksFetchFailed int
	blocksInserted int
	blocksInsertFailed int

	// Timing
	totalFetchTime time.Duration
	totalInsertTime time.Duration

	// Per-site history
	history []SessionRecord
}

// SessionRecord records stats for a single site session
type SessionRecord struct {
	SiteID       int
	SiteName     string
	StartTime    time.Time
	EndTime      time.Time
	Duration     time.Duration
	Success      bool
	Segments     int
	SegmentsHealed int
	Availability float64
}

// NewStatsCollector creates a new stats collector
func NewStatsCollector() *StatsCollector {
	return &StatsCollector{
		sessionStart: time.Now(),
		history:      make([]SessionRecord, 0),
	}
}

// Reset resets session statistics
func (s *StatsCollector) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sessionStart = time.Now()
	s.sitesProcessed = 0
	s.segmentsProcessed = 0
	s.segmentsSkipped = 0
	s.segmentsHealed = 0
	s.segmentsFailed = 0
	s.blocksFetched = 0
	s.blocksFetchFailed = 0
	s.blocksInserted = 0
	s.blocksInsertFailed = 0
	s.totalFetchTime = 0
	s.totalInsertTime = 0
}

// RecordSiteStart records the start of site processing
func (s *StatsCollector) RecordSiteStart(site *Site) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.sitesProcessed++
}

// RecordSiteComplete records the completion of site processing
func (s *StatsCollector) RecordSiteComplete(site *Site, success bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := SessionRecord{
		SiteID:    site.ID,
		SiteName:  site.Name,
		StartTime: site.StartedAt,
		EndTime:   site.CompletedAt,
		Duration:  site.CompletedAt.Sub(site.StartedAt),
		Success:   success,
		Segments:  site.SegmentCount(),
	}

	// Count healed segments
	for _, seg := range site.Segments {
		seg.mu.RLock()
		if seg.State == SegmentComplete {
			record.SegmentsHealed++
		}
		seg.mu.RUnlock()
	}

	site.mu.RLock()
	record.Availability = site.Availability
	site.mu.RUnlock()

	s.history = append(s.history, record)

	// Keep last 100 records
	if len(s.history) > 100 {
		s.history = s.history[len(s.history)-100:]
	}
}

// RecordSegmentResult records segment processing result
func (s *StatsCollector) RecordSegmentResult(state SegmentState) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.segmentsProcessed++

	switch state {
	case SegmentSkipped:
		s.segmentsSkipped++
	case SegmentComplete:
		s.segmentsHealed++
	case SegmentFailed:
		s.segmentsFailed++
	}
}

// RecordFetch records a fetch operation
func (s *StatsCollector) RecordFetch(success bool, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if success {
		s.blocksFetched++
	} else {
		s.blocksFetchFailed++
	}
	s.totalFetchTime += duration
}

// RecordInsert records an insert operation
func (s *StatsCollector) RecordInsert(success bool, duration time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if success {
		s.blocksInserted++
	} else {
		s.blocksInsertFailed++
	}
	s.totalInsertTime += duration
}

// GetSessionStats returns session statistics
func (s *StatsCollector) GetSessionStats() SessionStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return SessionStats{
		StartTime:         s.sessionStart,
		Duration:          time.Since(s.sessionStart),
		SitesProcessed:    s.sitesProcessed,
		SegmentsProcessed: s.segmentsProcessed,
		SegmentsSkipped:   s.segmentsSkipped,
		SegmentsHealed:    s.segmentsHealed,
		SegmentsFailed:    s.segmentsFailed,
		BlocksFetched:     s.blocksFetched,
		BlocksFetchFailed: s.blocksFetchFailed,
		BlocksInserted:    s.blocksInserted,
		BlocksInsertFailed: s.blocksInsertFailed,
		AvgFetchTime:      s.avgFetchTime(),
		AvgInsertTime:     s.avgInsertTime(),
	}
}

// avgFetchTime returns average fetch time (must hold lock)
func (s *StatsCollector) avgFetchTime() time.Duration {
	total := s.blocksFetched + s.blocksFetchFailed
	if total == 0 {
		return 0
	}
	return s.totalFetchTime / time.Duration(total)
}

// avgInsertTime returns average insert time (must hold lock)
func (s *StatsCollector) avgInsertTime() time.Duration {
	total := s.blocksInserted + s.blocksInsertFailed
	if total == 0 {
		return 0
	}
	return s.totalInsertTime / time.Duration(total)
}

// GetHistory returns session history
func (s *StatsCollector) GetHistory() []SessionRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]SessionRecord, len(s.history))
	copy(result, s.history)
	return result
}

// SessionStats contains session statistics
type SessionStats struct {
	StartTime         time.Time
	Duration          time.Duration
	SitesProcessed    int
	SegmentsProcessed int
	SegmentsSkipped   int
	SegmentsHealed    int
	SegmentsFailed    int
	BlocksFetched     int
	BlocksFetchFailed int
	BlocksInserted    int
	BlocksInsertFailed int
	AvgFetchTime      time.Duration
	AvgInsertTime     time.Duration
}

// String returns a formatted string of session stats
func (s SessionStats) String() string {
	return fmt.Sprintf(
		"Session Statistics:\n"+
		"  Duration: %s\n"+
		"  Sites processed: %d\n"+
		"  Segments: %d total, %d skipped, %d healed, %d failed\n"+
		"  Blocks fetched: %d (failed: %d)\n"+
		"  Blocks inserted: %d (failed: %d)\n"+
		"  Avg fetch time: %s\n"+
		"  Avg insert time: %s",
		s.Duration.Round(time.Second),
		s.SitesProcessed,
		s.SegmentsProcessed, s.SegmentsSkipped, s.SegmentsHealed, s.SegmentsFailed,
		s.BlocksFetched, s.BlocksFetchFailed,
		s.BlocksInserted, s.BlocksInsertFailed,
		s.AvgFetchTime.Round(time.Millisecond),
		s.AvgInsertTime.Round(time.Millisecond),
	)
}

// ProgressTracker tracks progress for UI updates
type ProgressTracker struct {
	mu sync.RWMutex

	site         *Site
	segment      *Segment
	message      string
	percent      float64
	lastUpdate   time.Time

	callbacks []func(ProgressUpdate)
}

// ProgressUpdate contains progress information
type ProgressUpdate struct {
	SiteID      int
	SiteName    string
	SegmentID   int
	SegmentTotal int
	Message     string
	Percent     float64
	UpdateTime  time.Time
}

// NewProgressTracker creates a new progress tracker
func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{
		callbacks: make([]func(ProgressUpdate), 0),
	}
}

// OnProgress registers a progress callback
func (p *ProgressTracker) OnProgress(cb func(ProgressUpdate)) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.callbacks = append(p.callbacks, cb)
}

// Update updates progress
func (p *ProgressTracker) Update(site *Site, segment *Segment, message string) {
	p.mu.Lock()
	p.site = site
	p.segment = segment
	p.message = message
	p.lastUpdate = time.Now()

	// Calculate percent
	if site != nil && segment != nil {
		segCount := site.SegmentCount()
		if segCount > 0 {
			p.percent = float64(segment.ID+1) / float64(segCount) * 100
		}
	}

	update := p.buildUpdate()
	callbacks := make([]func(ProgressUpdate), len(p.callbacks))
	copy(callbacks, p.callbacks)
	p.mu.Unlock()

	// Notify callbacks
	for _, cb := range callbacks {
		cb(update)
	}
}

// buildUpdate creates a progress update (must hold lock)
func (p *ProgressTracker) buildUpdate() ProgressUpdate {
	update := ProgressUpdate{
		Message:    p.message,
		Percent:    p.percent,
		UpdateTime: p.lastUpdate,
	}

	if p.site != nil {
		update.SiteID = p.site.ID
		update.SiteName = p.site.Name
		update.SegmentTotal = p.site.SegmentCount()
	}

	if p.segment != nil {
		update.SegmentID = p.segment.ID
	}

	return update
}

// Get returns current progress
func (p *ProgressTracker) Get() ProgressUpdate {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.buildUpdate()
}

// Clear clears progress
func (p *ProgressTracker) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.site = nil
	p.segment = nil
	p.message = ""
	p.percent = 0
}
