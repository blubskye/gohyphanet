// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

// Package keepalive provides content reinsertion to maintain availability on Freenet.
package keepalive

import (
	"fmt"
	"sync"
	"time"
)

// Version info
const (
	Version    = "0.1.0"
	ClientName = "GoKeepalive"
)

// Default configuration values
const (
	DefaultPower             = 5   // Concurrent workers
	DefaultSplitfileTolerance = 70  // Skip reinsertion if availability >= this %
	DefaultSplitfileTestSize  = 50  // Sample size for availability testing (%)
	DefaultWebPort           = 3081
)

// BlockState represents the state of a block
type BlockState int

const (
	BlockUnknown BlockState = iota
	BlockFetching
	BlockFetched
	BlockFetchFailed
	BlockInserting
	BlockInserted
	BlockInsertFailed
)

// String returns the string representation of BlockState
func (s BlockState) String() string {
	switch s {
	case BlockUnknown:
		return "unknown"
	case BlockFetching:
		return "fetching"
	case BlockFetched:
		return "fetched"
	case BlockFetchFailed:
		return "fetch_failed"
	case BlockInserting:
		return "inserting"
	case BlockInserted:
		return "inserted"
	case BlockInsertFailed:
		return "insert_failed"
	default:
		return "unknown"
	}
}

// Block represents a single CHK block
type Block struct {
	mu sync.RWMutex

	// Identity
	URI       string // CHK key
	SegmentID int    // Segment this block belongs to
	BlockID   int    // Block index within segment

	// Type
	IsDataBlock bool // true = data block, false = check block

	// State
	State       BlockState
	FetchDone   bool
	FetchOK     bool
	InsertDone  bool
	InsertOK    bool
	Data        []byte // Fetched block data
	Error       string // Last error message

	// Compression info from URI extra bytes
	CryptoAlgo  int
	ControlFlag int
	CompressAlgo int
}

// NewBlock creates a new block
func NewBlock(uri string, segmentID, blockID int, isDataBlock bool) *Block {
	return &Block{
		URI:         uri,
		SegmentID:   segmentID,
		BlockID:     blockID,
		IsDataBlock: isDataBlock,
		State:       BlockUnknown,
	}
}

// SetFetchResult sets the fetch result
func (b *Block) SetFetchResult(ok bool, data []byte, err string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.FetchDone = true
	b.FetchOK = ok
	b.Data = data
	b.Error = err

	if ok {
		b.State = BlockFetched
	} else {
		b.State = BlockFetchFailed
	}
}

// SetInsertResult sets the insert result
func (b *Block) SetInsertResult(ok bool, err string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.InsertDone = true
	b.InsertOK = ok
	if err != "" {
		b.Error = err
	}

	if ok {
		b.State = BlockInserted
	} else {
		b.State = BlockInsertFailed
	}
}

// ClearData clears the block data to free memory
func (b *Block) ClearData() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Data = nil
}

// SegmentState represents the state of a segment
type SegmentState int

const (
	SegmentPending SegmentState = iota
	SegmentTesting
	SegmentSkipped // Availability OK
	SegmentHealing
	SegmentComplete
	SegmentFailed
)

// String returns the string representation of SegmentState
func (s SegmentState) String() string {
	switch s {
	case SegmentPending:
		return "pending"
	case SegmentTesting:
		return "testing"
	case SegmentSkipped:
		return "skipped"
	case SegmentHealing:
		return "healing"
	case SegmentComplete:
		return "complete"
	case SegmentFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Segment represents a collection of blocks (splitfile segment)
type Segment struct {
	mu sync.RWMutex

	// Identity
	ID int

	// Blocks
	Blocks     []*Block
	DataCount  int // Number of data blocks
	CheckCount int // Number of check blocks

	// State
	State         SegmentState
	Availability  float64 // Availability percentage (0-100)
	FetchSuccess  int
	FetchFailed   int
	InsertSuccess int
	InsertFailed  int
}

// NewSegment creates a new segment
func NewSegment(id int) *Segment {
	return &Segment{
		ID:     id,
		Blocks: make([]*Block, 0),
		State:  SegmentPending,
	}
}

// AddBlock adds a block to the segment
func (s *Segment) AddBlock(block *Block) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.Blocks = append(s.Blocks, block)
	if block.IsDataBlock {
		s.DataCount++
	} else {
		s.CheckCount++
	}
}

// GetBlock returns a block by index
func (s *Segment) GetBlock(idx int) *Block {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if idx < 0 || idx >= len(s.Blocks) {
		return nil
	}
	return s.Blocks[idx]
}

// Size returns the total number of blocks
func (s *Segment) Size() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Blocks)
}

// UpdateStats updates segment statistics
func (s *Segment) UpdateStats() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.FetchSuccess = 0
	s.FetchFailed = 0
	s.InsertSuccess = 0
	s.InsertFailed = 0

	for _, block := range s.Blocks {
		if block.FetchOK {
			s.FetchSuccess++
		} else if block.FetchDone {
			s.FetchFailed++
		}
		if block.InsertOK {
			s.InsertSuccess++
		} else if block.InsertDone && !block.InsertOK {
			s.InsertFailed++
		}
	}

	total := s.FetchSuccess + s.FetchFailed
	if total > 0 {
		s.Availability = float64(s.FetchSuccess) / float64(total) * 100
	}
}

// IsComplete checks if segment processing is complete
func (s *Segment) IsComplete() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.State == SegmentSkipped || s.State == SegmentComplete || s.State == SegmentFailed {
		return true
	}

	// Check if all blocks are done
	for _, block := range s.Blocks {
		if block.FetchOK {
			continue
		}
		if !block.InsertDone {
			return false
		}
	}
	return true
}

// SiteState represents the state of a site
type SiteState int

const (
	SiteIdle SiteState = iota
	SiteParsing
	SiteReinserting
	SitePaused
	SiteComplete
	SiteError
)

// String returns the string representation of SiteState
func (s SiteState) String() string {
	switch s {
	case SiteIdle:
		return "idle"
	case SiteParsing:
		return "parsing"
	case SiteReinserting:
		return "reinserting"
	case SitePaused:
		return "paused"
	case SiteComplete:
		return "complete"
	case SiteError:
		return "error"
	default:
		return "unknown"
	}
}

// Site represents a site/file to keep alive
type Site struct {
	mu sync.RWMutex

	// Identity
	ID   int
	Name string
	URI  string // Original URI (USK, SSK, CHK)

	// State
	State          SiteState
	CurrentSegment int // Current segment being processed
	Error          string

	// Segments and blocks
	Segments   []*Segment
	BlockCount int

	// Statistics
	TotalBlocks     int
	AvailableBlocks int
	Availability    float64

	// Timestamps
	AddedAt     time.Time
	StartedAt   time.Time
	CompletedAt time.Time
	LastUpdate  time.Time

	// Log
	Log []LogEntry
}

// LogEntry represents a log entry
type LogEntry struct {
	Time    time.Time
	Level   int // 0=info, 1=detail, 2=debug
	Message string
}

// NewSite creates a new site
func NewSite(id int, uri string) *Site {
	return &Site{
		ID:       id,
		URI:      uri,
		State:    SiteIdle,
		Segments: make([]*Segment, 0),
		Log:      make([]LogEntry, 0),
		AddedAt:  time.Now(),
	}
}

// AddSegment adds a segment to the site
func (s *Site) AddSegment(segment *Segment) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Segments = append(s.Segments, segment)
}

// GetSegment returns a segment by index
func (s *Site) GetSegment(idx int) *Segment {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if idx < 0 || idx >= len(s.Segments) {
		return nil
	}
	return s.Segments[idx]
}

// SegmentCount returns the number of segments
func (s *Site) SegmentCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.Segments)
}

// AddLog adds a log entry
func (s *Site) AddLog(level int, format string, args ...interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := LogEntry{
		Time:    time.Now(),
		Level:   level,
		Message: fmt.Sprintf(format, args...),
	}
	s.Log = append(s.Log, entry)

	// Limit log size
	if len(s.Log) > 1000 {
		s.Log = s.Log[len(s.Log)-1000:]
	}
}

// UpdateStats updates site statistics
func (s *Site) UpdateStats() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalBlocks = 0
	s.AvailableBlocks = 0

	for _, seg := range s.Segments {
		seg.UpdateStats()
		s.TotalBlocks += seg.Size()
		s.AvailableBlocks += seg.FetchSuccess
	}

	if s.TotalBlocks > 0 {
		s.Availability = float64(s.AvailableBlocks) / float64(s.TotalBlocks) * 100
	}

	s.LastUpdate = time.Now()
}

// GetRecentLogs returns the most recent log entries
func (s *Site) GetRecentLogs(count int) []LogEntry {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if count > len(s.Log) {
		count = len(s.Log)
	}
	return s.Log[len(s.Log)-count:]
}

// GetCurrentSegment returns the current segment being processed
func (s *Site) GetCurrentSegment() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.CurrentSegment
}

// GetSegmentsList returns a copy of the segments list
func (s *Site) GetSegmentsList() []*Segment {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*Segment, len(s.Segments))
	copy(result, s.Segments)
	return result
}

// Config holds plugin configuration
type Config struct {
	mu sync.RWMutex

	// Worker settings
	Power int // Concurrent workers

	// Reinsertion settings
	SplitfileTolerance int // Skip if availability >= this %
	SplitfileTestSize  int // Sample size for testing (%)

	// Active site
	ActiveSiteID int // -1 = none

	// Logging
	LogLevel int // 0=info, 1=detail, 2=debug
	LogUTC   bool

	// Server ports
	WebPort int
}

// NewConfig creates a new config with defaults
func NewConfig() *Config {
	return &Config{
		Power:              DefaultPower,
		SplitfileTolerance: DefaultSplitfileTolerance,
		SplitfileTestSize:  DefaultSplitfileTestSize,
		ActiveSiteID:       -1,
		LogLevel:           1,
		LogUTC:             true,
		WebPort:            DefaultWebPort,
	}
}

// GetPower returns the power setting
func (c *Config) GetPower() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Power
}

// SetPower sets the power setting
func (c *Config) SetPower(power int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Power = power
}

// GetTolerance returns the splitfile tolerance
func (c *Config) GetTolerance() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.SplitfileTolerance
}

// GetTestSize returns the splitfile test size
func (c *Config) GetTestSize() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.SplitfileTestSize
}

// GetActiveSiteID returns the active site ID
func (c *Config) GetActiveSiteID() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ActiveSiteID
}

// SetActiveSiteID sets the active site ID
func (c *Config) SetActiveSiteID(id int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ActiveSiteID = id
}
