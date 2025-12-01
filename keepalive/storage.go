// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package keepalive

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Storage handles persistence for keepalive data
type Storage struct {
	mu      sync.RWMutex
	dataDir string
}

// NewStorage creates a new storage instance
func NewStorage(dataDir string) *Storage {
	return &Storage{
		dataDir: dataDir,
	}
}

// Initialize creates the data directory structure
func (s *Storage) Initialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dirs := []string{
		s.dataDir,
		filepath.Join(s.dataDir, "sites"),
		filepath.Join(s.dataDir, "blocks"),
		filepath.Join(s.dataDir, "stats"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// SiteRecord represents a site record in storage
type SiteRecord struct {
	ID          int       `json:"id"`
	URI         string    `json:"uri"`
	Name        string    `json:"name"`
	AddedAt     time.Time `json:"added_at"`
	LastChecked time.Time `json:"last_checked,omitempty"`
	LastResult  string    `json:"last_result,omitempty"`
}

// SaveSite saves a site record
func (s *Storage) SaveSite(site *Site) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := &SiteRecord{
		ID:      site.ID,
		URI:     site.URI,
		Name:    site.Name,
		AddedAt: site.AddedAt,
	}

	filename := filepath.Join(s.dataDir, "sites", fmt.Sprintf("%d.json", site.ID))
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal site: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write site file: %w", err)
	}

	return nil
}

// LoadSite loads a site record by ID
func (s *Storage) LoadSite(id int) (*SiteRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filename := filepath.Join(s.dataDir, "sites", fmt.Sprintf("%d.json", id))
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read site file: %w", err)
	}

	var record SiteRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal site: %w", err)
	}

	return &record, nil
}

// LoadAllSites loads all site records
func (s *Storage) LoadAllSites() ([]*SiteRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sitesDir := filepath.Join(s.dataDir, "sites")
	entries, err := os.ReadDir(sitesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read sites directory: %w", err)
	}

	var sites []*SiteRecord
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(sitesDir, entry.Name()))
		if err != nil {
			continue
		}

		var record SiteRecord
		if err := json.Unmarshal(data, &record); err != nil {
			continue
		}

		sites = append(sites, &record)
	}

	return sites, nil
}

// DeleteSite deletes a site record and its blocks
func (s *Storage) DeleteSite(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Delete site record
	siteFile := filepath.Join(s.dataDir, "sites", fmt.Sprintf("%d.json", id))
	if err := os.Remove(siteFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete site file: %w", err)
	}

	// Delete blocks file
	blocksFile := filepath.Join(s.dataDir, "blocks", fmt.Sprintf("%d.txt", id))
	if err := os.Remove(blocksFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete blocks file: %w", err)
	}

	// Delete stats file
	statsFile := filepath.Join(s.dataDir, "stats", fmt.Sprintf("%d.json", id))
	if err := os.Remove(statsFile); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete stats file: %w", err)
	}

	return nil
}

// GetNextSiteID returns the next available site ID
func (s *Storage) GetNextSiteID() (int, error) {
	sites, err := s.LoadAllSites()
	if err != nil {
		return 1, err
	}

	maxID := 0
	for _, site := range sites {
		if site.ID > maxID {
			maxID = site.ID
		}
	}

	return maxID + 1, nil
}

// BlockRecord represents a block in storage
type BlockRecord struct {
	URI         string `json:"uri"`
	SegmentID   int    `json:"segment_id"`
	BlockID     int    `json:"block_id"`
	IsDataBlock bool   `json:"is_data_block"`
}

// SaveBlocks saves block keys for a site
func (s *Storage) SaveBlocks(siteID int, segments []*Segment) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := filepath.Join(s.dataDir, "blocks", fmt.Sprintf("%d.txt", siteID))
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create blocks file: %w", err)
	}
	defer file.Close()

	writer := bufio.NewWriter(file)

	for _, seg := range segments {
		for _, block := range seg.Blocks {
			blockType := "D"
			if !block.IsDataBlock {
				blockType = "C"
			}
			fmt.Fprintf(writer, "%d\t%d\t%s\t%s\n",
				block.SegmentID,
				block.BlockID,
				blockType,
				block.URI,
			)
		}
	}

	return writer.Flush()
}

// LoadBlocks loads block keys for a site
func (s *Storage) LoadBlocks(siteID int) ([]*BlockRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filename := filepath.Join(s.dataDir, "blocks", fmt.Sprintf("%d.txt", siteID))
	file, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open blocks file: %w", err)
	}
	defer file.Close()

	var blocks []*BlockRecord
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, "\t")
		if len(parts) < 4 {
			continue
		}

		segID, _ := strconv.Atoi(parts[0])
		blockID, _ := strconv.Atoi(parts[1])
		isData := parts[2] == "D"
		uri := parts[3]

		blocks = append(blocks, &BlockRecord{
			SegmentID:   segID,
			BlockID:     blockID,
			IsDataBlock: isData,
			URI:         uri,
		})
	}

	return blocks, scanner.Err()
}

// SiteStats represents statistics for a site
type SiteStats struct {
	SiteID        int       `json:"site_id"`
	LastCheck     time.Time `json:"last_check"`
	TotalBlocks   int       `json:"total_blocks"`
	AvailBlocks   int       `json:"avail_blocks"`
	Availability  float64   `json:"availability"`
	SegmentStats  []SegmentStats `json:"segments,omitempty"`
}

// SegmentStats represents statistics for a segment
type SegmentStats struct {
	SegmentID    int     `json:"segment_id"`
	DataBlocks   int     `json:"data_blocks"`
	CheckBlocks  int     `json:"check_blocks"`
	FetchSuccess int     `json:"fetch_success"`
	FetchFailed  int     `json:"fetch_failed"`
	Availability float64 `json:"availability"`
}

// SaveStats saves statistics for a site
func (s *Storage) SaveStats(stats *SiteStats) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filename := filepath.Join(s.dataDir, "stats", fmt.Sprintf("%d.json", stats.SiteID))
	data, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal stats: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write stats file: %w", err)
	}

	return nil
}

// LoadStats loads statistics for a site
func (s *Storage) LoadStats(siteID int) (*SiteStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filename := filepath.Join(s.dataDir, "stats", fmt.Sprintf("%d.json", siteID))
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read stats file: %w", err)
	}

	var stats SiteStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return nil, fmt.Errorf("failed to unmarshal stats: %w", err)
	}

	return &stats, nil
}

// ConfigRecord represents configuration in storage
type ConfigRecord struct {
	Power              int  `json:"power"`
	SplitfileTolerance int  `json:"splitfile_tolerance"`
	SplitfileTestSize  int  `json:"splitfile_test_size"`
	ActiveSiteID       int  `json:"active_site_id"`
	LogLevel           int  `json:"log_level"`
	LogUTC             bool `json:"log_utc"`
	WebPort            int  `json:"web_port"`
}

// SaveConfig saves configuration
func (s *Storage) SaveConfig(cfg *Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record := &ConfigRecord{
		Power:              cfg.Power,
		SplitfileTolerance: cfg.SplitfileTolerance,
		SplitfileTestSize:  cfg.SplitfileTestSize,
		ActiveSiteID:       cfg.ActiveSiteID,
		LogLevel:           cfg.LogLevel,
		LogUTC:             cfg.LogUTC,
		WebPort:            cfg.WebPort,
	}

	filename := filepath.Join(s.dataDir, "config.json")
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// LoadConfig loads configuration
func (s *Storage) LoadConfig() (*Config, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filename := filepath.Join(s.dataDir, "config.json")
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return NewConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var record ConfigRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	cfg := &Config{
		Power:              record.Power,
		SplitfileTolerance: record.SplitfileTolerance,
		SplitfileTestSize:  record.SplitfileTestSize,
		ActiveSiteID:       record.ActiveSiteID,
		LogLevel:           record.LogLevel,
		LogUTC:             record.LogUTC,
		WebPort:            record.WebPort,
	}

	// Apply defaults for zero values
	if cfg.Power == 0 {
		cfg.Power = DefaultPower
	}
	if cfg.SplitfileTolerance == 0 {
		cfg.SplitfileTolerance = DefaultSplitfileTolerance
	}
	if cfg.SplitfileTestSize == 0 {
		cfg.SplitfileTestSize = DefaultSplitfileTestSize
	}
	if cfg.WebPort == 0 {
		cfg.WebPort = DefaultWebPort
	}

	return cfg, nil
}

// SiteManager manages sites in memory with persistence
type SiteManager struct {
	mu      sync.RWMutex
	storage *Storage
	sites   map[int]*Site
	nextID  int
}

// NewSiteManager creates a new site manager
func NewSiteManager(storage *Storage) *SiteManager {
	return &SiteManager{
		storage: storage,
		sites:   make(map[int]*Site),
		nextID:  1,
	}
}

// Initialize loads sites from storage
func (m *SiteManager) Initialize() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	records, err := m.storage.LoadAllSites()
	if err != nil {
		return err
	}

	for _, record := range records {
		site := NewSite(record.ID, record.URI)
		site.Name = record.Name
		site.AddedAt = record.AddedAt
		m.sites[site.ID] = site

		if site.ID >= m.nextID {
			m.nextID = site.ID + 1
		}

		// Load blocks if available
		blocks, err := m.storage.LoadBlocks(site.ID)
		if err == nil && blocks != nil {
			m.loadBlocksIntoSite(site, blocks)
		}

		// Load stats if available
		stats, err := m.storage.LoadStats(site.ID)
		if err == nil && stats != nil {
			site.TotalBlocks = stats.TotalBlocks
			site.AvailableBlocks = stats.AvailBlocks
			site.Availability = stats.Availability
		}
	}

	return nil
}

// loadBlocksIntoSite loads block records into a site structure
func (m *SiteManager) loadBlocksIntoSite(site *Site, blocks []*BlockRecord) {
	// Group blocks by segment
	segmentBlocks := make(map[int][]*BlockRecord)
	for _, block := range blocks {
		segmentBlocks[block.SegmentID] = append(segmentBlocks[block.SegmentID], block)
	}

	// Create segments
	for segID, segBlocks := range segmentBlocks {
		segment := NewSegment(segID)

		for _, br := range segBlocks {
			block := NewBlock(br.URI, br.SegmentID, br.BlockID, br.IsDataBlock)
			segment.AddBlock(block)
		}

		site.AddSegment(segment)
	}
}

// AddSite adds a new site
func (m *SiteManager) AddSite(uri string, name string) (*Site, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	site := NewSite(m.nextID, uri)
	site.Name = name
	m.nextID++

	m.sites[site.ID] = site

	// Save to storage
	if err := m.storage.SaveSite(site); err != nil {
		delete(m.sites, site.ID)
		return nil, err
	}

	return site, nil
}

// GetSite returns a site by ID
func (m *SiteManager) GetSite(id int) *Site {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sites[id]
}

// GetAllSites returns all sites
func (m *SiteManager) GetAllSites() []*Site {
	m.mu.RLock()
	defer m.mu.RUnlock()

	sites := make([]*Site, 0, len(m.sites))
	for _, site := range m.sites {
		sites = append(sites, site)
	}
	return sites
}

// RemoveSite removes a site
func (m *SiteManager) RemoveSite(id int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.sites[id]; !exists {
		return fmt.Errorf("site not found: %d", id)
	}

	delete(m.sites, id)
	return m.storage.DeleteSite(id)
}

// SaveSiteBlocks saves blocks for a site
func (m *SiteManager) SaveSiteBlocks(site *Site) error {
	return m.storage.SaveBlocks(site.ID, site.Segments)
}

// SaveSiteStats saves statistics for a site
func (m *SiteManager) SaveSiteStats(site *Site) error {
	site.UpdateStats()

	stats := &SiteStats{
		SiteID:       site.ID,
		LastCheck:    time.Now(),
		TotalBlocks:  site.TotalBlocks,
		AvailBlocks:  site.AvailableBlocks,
		Availability: site.Availability,
	}

	for _, seg := range site.Segments {
		segStats := SegmentStats{
			SegmentID:    seg.ID,
			DataBlocks:   seg.DataCount,
			CheckBlocks:  seg.CheckCount,
			FetchSuccess: seg.FetchSuccess,
			FetchFailed:  seg.FetchFailed,
			Availability: seg.Availability,
		}
		stats.SegmentStats = append(stats.SegmentStats, segStats)
	}

	return m.storage.SaveStats(stats)
}

// Count returns the number of sites
func (m *SiteManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.sites)
}
