# GoSone Implementation Plan

A Go port of the Sone social networking plugin for Hyphanet/Freenet.

## Project Status

**Current Progress:** ALL PHASES COMPLETE (~7,000 LOC)

| Phase | Status | Description |
|-------|--------|-------------|
| 1 | ✅ Complete | Web of Trust (WoT) client |
| 2 | ✅ Complete | Data model (Sone, Post, Reply, Album, Image) |
| 3 | ✅ Complete | Database layer with persistence |
| 4 | ✅ Complete | XML parser (downloading Sones) |
| 5 | ✅ Complete | XML serializer (uploading Sones) |
| 6 | ✅ Complete | Event system (Go channels) |
| 7 | ✅ Complete | Core component (inserters, downloaders) |
| 8 | ✅ Complete | Text parser (links, mentions) |
| 9 | ✅ Complete | Web UI |
| 10 | ✅ Complete | CLI tool |
| 11 | ✅ Complete | USK subscriptions |
| 12 | ✅ Complete | Image handling |
| 13 | ✅ Complete | Testing |

---

## Phase 8: Text Parser

**Goal:** Parse Sone text for links, mentions, and special elements.

### 8.1 Link Types to Detect
- [ ] Freenet URIs (`USK@`, `SSK@`, `CHK@`, `KSK@`)
- [ ] Sone links (`sone://SONE_ID`)
- [ ] Post links (`post://POST_ID`)
- [ ] HTTP/HTTPS URLs
- [ ] Freemail addresses (`identity@freemail`)

### 8.2 Mention Detection
- [ ] Detect `@nickname` mentions
- [ ] Resolve mentions to Sone IDs
- [ ] Trigger mention notifications

### 8.3 Output Format
- [ ] Return parsed elements as structured data
- [ ] Support rendering to HTML
- [ ] Support rendering to plain text

**Files to create:**
- `sone/textparser.go`

---

## Phase 9: Web UI

**Goal:** HTTP-based web interface for Sone.

### 9.1 Core Infrastructure
- [ ] HTTP server setup
- [ ] Template engine (html/template)
- [ ] Static file serving (CSS, JS)
- [ ] Session management
- [ ] Form handling with CSRF protection

### 9.2 Pages
- [ ] `/` - Index/feed page
- [ ] `/sone/{id}` - View Sone profile
- [ ] `/post/{id}` - View single post
- [ ] `/known-sones` - List all known Sones
- [ ] `/create-post` - Create new post form
- [ ] `/create-reply` - Create reply form
- [ ] `/options` - Settings page
- [ ] `/search` - Search page
- [ ] `/bookmarks` - Bookmarked posts
- [ ] `/login` - Select local Sone

### 9.3 AJAX Endpoints
- [ ] `POST /ajax/create-post` - Create post
- [ ] `POST /ajax/create-reply` - Create reply
- [ ] `POST /ajax/delete-post` - Delete post
- [ ] `POST /ajax/delete-reply` - Delete reply
- [ ] `POST /ajax/like` - Like post/reply
- [ ] `POST /ajax/unlike` - Unlike post/reply
- [ ] `POST /ajax/follow` - Follow Sone
- [ ] `POST /ajax/unfollow` - Unfollow Sone
- [ ] `POST /ajax/bookmark` - Bookmark post
- [ ] `POST /ajax/unbookmark` - Remove bookmark
- [ ] `GET /ajax/status` - Get current status
- [ ] `GET /ajax/notifications` - Get notifications
- [ ] `POST /ajax/dismiss-notification` - Dismiss notification

### 9.4 Templates
- [ ] Base layout template
- [ ] Post template (reusable)
- [ ] Reply template (reusable)
- [ ] Sone profile template
- [ ] Navigation template
- [ ] Notification template

**Files to create:**
- `sone/web/server.go`
- `sone/web/handlers.go`
- `sone/web/ajax.go`
- `sone/web/templates/` (directory)
- `sone/web/static/` (directory)

---

## Phase 10: CLI Tool

**Goal:** Command-line interface for testing and basic operations.

### 10.1 Commands
- [ ] `gosone status` - Show connection status
- [ ] `gosone identities` - List local identities
- [ ] `gosone sones` - List known Sones
- [ ] `gosone feed [sone-id]` - Show post feed
- [ ] `gosone post <sone-id> <text>` - Create a post
- [ ] `gosone reply <sone-id> <post-id> <text>` - Create a reply
- [ ] `gosone follow <sone-id> <target-id>` - Follow a Sone
- [ ] `gosone unfollow <sone-id> <target-id>` - Unfollow a Sone
- [ ] `gosone profile <sone-id>` - View Sone profile

### 10.2 Configuration
- [ ] Config file support (`~/.gosone/config.yaml`)
- [ ] Command-line flags for FCP host/port

**Files to create:**
- `cmd/gosone/main.go`
- `cmd/gosone/commands.go`

---

## Phase 11: USK Subscriptions

**Goal:** Real-time notifications when Sones update.

### 11.1 FCP Integration
- [ ] `SubscribeUSK` message support
- [ ] `UnsubscribeUSK` message support
- [ ] `SubscribedUSKUpdate` handler

### 11.2 Sone Update Detection
- [ ] Subscribe to followed Sones' USKs
- [ ] Automatic re-fetch on edition change
- [ ] Unsubscribe when unfollowing

**Files to modify:**
- `fcp/client.go` - Add USK subscription methods
- `sone/core.go` - Integrate USK subscriptions

---

## Phase 12: Image Handling

**Goal:** Support image uploads and display.

### 12.1 Image Upload
- [ ] Accept image file upload via web UI
- [ ] Generate image metadata (dimensions, MIME type)
- [ ] Insert image to Hyphanet via FCP
- [ ] Track insertion progress
- [ ] Update album with image key on completion

### 12.2 Image Display
- [ ] Proxy image requests through fproxy
- [ ] Thumbnail generation (optional)
- [ ] Lazy loading in web UI

### 12.3 Album Management
- [ ] Create album
- [ ] Delete album
- [ ] Move images between albums
- [ ] Set album cover image

**Files to create:**
- `sone/images.go`
- `sone/web/image_handlers.go`

---

## Phase 13: Testing

**Goal:** Comprehensive test coverage.

### 13.1 Unit Tests
- [ ] `wot/wot_test.go` - WoT client tests
- [ ] `sone/model_test.go` - Data model tests
- [ ] `sone/xml_test.go` - XML parsing tests
- [ ] `sone/database_test.go` - Database tests
- [ ] `sone/textparser_test.go` - Text parser tests

### 13.2 Integration Tests
- [ ] FCP connection tests (requires running Hyphanet node)
- [ ] WoT integration tests
- [ ] Full Sone insertion/fetch cycle

### 13.3 Test Data
- [ ] Sample Sone XML files
- [ ] Mock FCP responses

---

## File Structure (Target)

```
gohyphanet/
├── fcp/                     # FCP client library (existing)
├── wot/                     # Web of Trust client (complete)
│   └── wot.go
├── sone/                    # Sone implementation
│   ├── model.go             # Data model (complete)
│   ├── xml.go               # XML parser/serializer (complete)
│   ├── database.go          # Database layer (complete)
│   ├── events.go            # Event system (complete)
│   ├── core.go              # Core logic (complete)
│   ├── textparser.go        # Text parsing (Phase 8)
│   ├── images.go            # Image handling (Phase 12)
│   └── web/                 # Web interface (Phase 9)
│       ├── server.go
│       ├── handlers.go
│       ├── ajax.go
│       ├── templates/
│       └── static/
├── cmd/
│   ├── wottest/             # WoT test tool (complete)
│   └── gosone/              # CLI tool (Phase 10)
└── GOSONE_PLAN.md           # This file
```

---

## Dependencies

**Required:**
- Go 1.21+
- Running Hyphanet node with FCP enabled
- Web of Trust plugin installed

**Go Packages (standard library only):**
- `encoding/xml`
- `encoding/json`
- `net/http`
- `html/template`
- `crypto/sha256`

---

## Getting Started

### Build
```bash
cd /home/blubskye/Downloads/gohyphanet
go build ./wot ./sone
```

### Test WoT Connection
```bash
go build -o wottest ./cmd/wottest
./wottest -host localhost -port 9481
```

---

## Notes

- Protocol version: 0 (compatible with Sone v82)
- Client name: "GoSone"
- Client version: "0.1.0"
- Default FCP port: 9481
- Default data directory: `~/.gosone`

---

## References

- Java Sone source: `/home/blubskye/Downloads/Sone-main(1)/Sone-main/`
- Web of Trust source: `/home/blubskye/Downloads/plugin-WebOfTrust-next/`
- Freenet reference: `/home/blubskye/Downloads/fred-next/`
- GoHyphanet FCP: `/home/blubskye/Downloads/gohyphanet/fcp/`
