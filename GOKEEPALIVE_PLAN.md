# GoKeepalive Implementation Plan

A Go port of the Keepalive plugin for Hyphanet/Freenet - automatic content reinsertion to maintain availability.

## Project Status

**Current Progress:** ALL PHASES COMPLETE (~2,500 LOC)

| Phase | Status | Description |
|-------|--------|-------------|
| 1 | ✅ Complete | Data model (Site, Block, Segment) |
| 2 | ✅ Complete | Block fetcher/inserter |
| 3 | ✅ Complete | Reinserter engine |
| 4 | ✅ Complete | Web UI |
| 5 | ✅ Complete | CLI/Daemon |

---

## Overview

Keepalive monitors and maintains the availability of Freenet content by:
- Periodically checking block availability
- Reinserting missing blocks
- Using FEC (Forward Error Correction) to heal segments
- Tracking persistence statistics

---

## Phase 1: Data Model

### 1.1 Core Types
- `Site` - A site/file to keep alive (URI, blocks, segments)
- `Block` - Individual CHK block with fetch/insert status
- `Segment` - Collection of blocks (data blocks + check blocks)
- `Config` - Plugin configuration

### 1.2 Storage
- `SiteStore` - Persistent site configuration
- `BlockStore` - Block key lists per site
- `StatsStore` - Availability statistics

**Files to create:**
- `keepalive/model.go`
- `keepalive/storage.go`

---

## Phase 2: Block Operations

### 2.1 Block Fetching
- Fetch individual blocks via FCP
- Check availability (ignoreStore mode)
- Handle compression types

### 2.2 Block Insertion
- Insert blocks via FCP
- Track insertion status
- Handle errors

### 2.3 Metadata Parsing
- Parse Freenet metadata
- Extract block keys from manifests
- Handle splitfiles, archives

**Files to create:**
- `keepalive/fetcher.go`
- `keepalive/inserter.go`
- `keepalive/metadata.go`

---

## Phase 3: Reinserter Engine

### 3.1 Site Management
- Add/remove sites
- Update USK editions
- Track site state

### 3.2 Reinsertion Logic
- Sample-based availability testing
- Full segment healing
- Concurrent fetch/insert workers

### 3.3 Statistics
- Block availability percentage
- Segment health tracking
- Activity logging

**Files to create:**
- `keepalive/reinserter.go`
- `keepalive/worker.go`
- `keepalive/stats.go`

---

## Phase 4: Web UI

### 4.1 Pages
- Site list/overview
- Add new site
- Site details/log
- Settings

### 4.2 Features
- Start/stop reinsertion
- Real-time progress
- Log viewer

**Files to create:**
- `keepalive/web/server.go`
- `keepalive/web/handlers.go`
- `keepalive/web/templates/`

---

## Phase 5: CLI Tool

### 5.1 Commands
- `gokeepalive list` - List sites
- `gokeepalive add <uri>` - Add site
- `gokeepalive remove <id>` - Remove site
- `gokeepalive start [id]` - Start reinsertion
- `gokeepalive stop` - Stop reinsertion
- `gokeepalive status` - Show status
- `gokeepalive serve` - Start web UI

**Files to create:**
- `cmd/gokeepalive/main.go`

---

## Configuration

Default settings:
- `power`: 5 (concurrent workers)
- `splitfile_tolerance`: 70% (skip if availability above)
- `splitfile_test_size`: 50% (sample size for testing)

---

## Key Concepts

### Availability Testing
1. For each segment, sample ~50% of blocks
2. Fetch with ignoreStore=true (bypass cache)
3. Calculate availability percentage
4. If >= 70%, skip reinsertion
5. Otherwise, fetch all blocks and heal

### Segment Healing
- Freenet uses FEC (Forward Error Correction)
- Data blocks + check blocks
- Can recover missing blocks if enough remain
- Heal = reconstruct + reinsert missing

### Block Types
- **Data blocks**: Original content
- **Check blocks**: FEC parity blocks
- Both stored as CHK keys

---

## File Structure

```
gohyphanet/
├── keepalive/
│   ├── model.go           # Core data types
│   ├── storage.go         # Persistence
│   ├── fetcher.go         # Block fetching
│   ├── inserter.go        # Block insertion
│   ├── metadata.go        # Metadata parsing
│   ├── reinserter.go      # Main reinsertion engine
│   ├── worker.go          # Concurrent workers
│   ├── stats.go           # Statistics
│   └── web/               # Web interface
│       ├── server.go
│       ├── handlers.go
│       └── templates/
├── cmd/
│   └── gokeepalive/       # CLI tool
│       └── main.go
└── GOKEEPALIVE_PLAN.md    # This file
```

---

## Dependencies

- `gohyphanet/fcp` - FCP client library

---

## References

- Java Keepalive: `/home/blubskye/Downloads/keepalive-15-keepalive_source_0.3.2/`
- Freenet FCP: https://github.com/freenet/wiki/wiki/FCPv2
