# Hyphanet Go Implementation - Complete Roadmap

## Vision
Build a complete, production-ready Hyphanet node implementation in Go that can:
- Connect to the Hyphanet network
- Route requests using location-based routing
- Store and retrieve data blocks
- Handle CHK/SSK/USK requests
- Provide FCP server for clients

## Timeline Estimate: 6-12 months

---

## Phase 1: Foundation (Weeks 1-4) ✅ STARTED

### Week 1-2: Network Layer ✅ PARTIALLY COMPLETE
- [x] Node identity (ECDSA P-256)
- [x] ECDH key exchange
- [x] UDP transport
- [x] Packet encryption (PCFB mode)
- [x] JFK Message 1 (send)
- [ ] JFK Message 2 (receive/verify)
- [ ] JFK Message 3 (send)
- [ ] JFK Message 4 (receive/complete)
- [ ] Session key derivation

**Files:**
- `node/crypto/*.go` ✅
- `node/transport/*.go` ✅
- `node/protocol/jfk.go` (needs completion)

### Week 3: Peer Management
- [ ] Peer node structure
- [ ] Peer state machine (disconnected, handshaking, connected)
- [ ] Peer list management
- [ ] Seed node loading (parse .fref files)
- [ ] Connection lifecycle

**Files:**
- `node/peer/peer.go`
- `node/peer/manager.go`
- `node/peer/seednode.go`

### Week 4: Session Management
- [ ] Session key tracking
- [ ] Key rekeying
- [ ] Packet sequence numbers
- [ ] Message authentication
- [ ] Session timeout handling

**Files:**
- `node/session/session.go`
- `node/session/tracker.go`

---

## Phase 2: Message Protocol (Weeks 5-8)

### Week 5-6: NewPacketFormat (NPF)
- [ ] NPF packet structure
- [ ] Message types (data, ack, probe, etc.)
- [ ] Packet fragmentation
- [ ] Packet reassembly
- [ ] Loss detection
- [ ] Retransmission

**Reference:** `fred-next/src/freenet/node/NewPacketFormat.java`

**Files:**
- `node/protocol/npf.go`
- `node/protocol/fragment.go`
- `node/protocol/ack.go`

### Week 7: Message Routing
- [ ] Message dispatcher
- [ ] Message handlers (by type)
- [ ] Request ID tracking
- [ ] Timeout handling
- [ ] Error responses

**Files:**
- `node/message/dispatcher.go`
- `node/message/handler.go`
- `node/message/types.go`

### Week 8: Node-to-Node Messages
- [ ] FNPPing/FNPPong
- [ ] FNPVoid
- [ ] FNPDisconnect
- [ ] Load messages
- [ ] Announcement protocol

**Files:**
- `node/message/ping.go`
- `node/message/announce.go`

---

## Phase 3: Data Layer (Weeks 9-12)

### Week 9-10: Block Storage
- [ ] Block interface
- [ ] CHK blocks
- [ ] SSK blocks
- [ ] Datastore interface
- [ ] SQLite datastore implementation
- [ ] File-based datastore
- [ ] LRU cache
- [ ] Salt generation

**Files:**
- `node/storage/block.go`
- `node/storage/datastore.go`
- `node/storage/sqlite.go`
- `node/storage/cache.go`

### Week 11: Key System
- [ ] CHK generation/verification
- [ ] SSK generation/verification
- [ ] USK handling
- [ ] KSK handling
- [ ] Crypto verification
- [ ] Key parsing

**Files:**
- `node/keys/chk.go`
- `node/keys/ssk.go`
- `node/keys/usk.go`

### Week 12: Data Verification
- [ ] Hash verification
- [ ] Signature verification
- [ ] Block validation
- [ ] Metadata parsing

**Files:**
- `node/verify/verify.go`

---

## Phase 4: Routing (Weeks 13-16)

### Week 13-14: Location System
- [ ] Node location (double 0.0-1.0)
- [ ] Distance calculation
- [ ] Routing table
- [ ] Closest peer finding
- [ ] Location management
- [ ] Swapping algorithm

**Reference:** `fred-next/src/freenet/node/Location.java`

**Files:**
- `node/routing/location.go`
- `node/routing/table.go`
- `node/routing/swap.go`

### Week 15: Request Routing
- [ ] HTL (Hops To Live)
- [ ] Route selection
- [ ] Backoff
- [ ] Loop detection
- [ ] Recently failed tracking

**Files:**
- `node/routing/route.go`
- `node/routing/htl.go`

### Week 16: Load Management
- [ ] Load tracking
- [ ] Bandwidth management
- [ ] Request throttling
- [ ] Priority handling

**Files:**
- `node/load/manager.go`
- `node/load/bandwidth.go`

---

## Phase 5: Request Handling (Weeks 17-20)

### Week 17-18: CHK Requests
- [ ] CHK insert sender
- [ ] CHK request sender
- [ ] CHK insert handler
- [ ] CHK request handler
- [ ] Store forwarding
- [ ] Data verification

**Files:**
- `node/request/chk_insert.go`
- `node/request/chk_fetch.go`
- `node/request/handler.go`

### Week 19: SSK Requests
- [ ] SSK insert sender
- [ ] SSK request sender
- [ ] SSK insert handler
- [ ] SSK request handler
- [ ] Signature verification
- [ ] Pubkey caching

**Files:**
- `node/request/ssk_insert.go`
- `node/request/ssk_fetch.go`

### Week 20: Advanced Features
- [ ] USK polling
- [ ] Splitfile handling
- [ ] Metadata parsing
- [ ] Redirect handling

**Files:**
- `node/request/usk.go`
- `node/request/splitfile.go`

---

## Phase 6: FCP Server (Weeks 21-24)

### Week 21-22: FCP Protocol
- [ ] FCP connection handling
- [ ] FCP message parsing
- [ ] ClientHello/NodeHello
- [ ] Get/Put operations
- [ ] Progress tracking
- [ ] Multiple clients

**Files:**
- `node/fcp/server.go`
- `node/fcp/client.go`
- `node/fcp/messages.go`

### Week 23: FCP Operations
- [ ] ClientGet
- [ ] ClientPut
- [ ] GenerateSSK
- [ ] ListPeers
- [ ] GetConfig/ModifyConfig

**Files:**
- `node/fcp/get.go`
- `node/fcp/put.go`
- `node/fcp/admin.go`

### Week 24: FCP Advanced
- [ ] Persistent requests
- [ ] Request queue
- [ ] Watchdog
- [ ] Plugin support (basic)

---

## Phase 7: Network Management (Weeks 25-28)

### Week 25-26: Opennet
- [ ] Announcement sending
- [ ] Announcement receiving
- [ ] Peer discovery
- [ ] Seed node connection
- [ ] Strangers management

**Files:**
- `node/opennet/announcer.go`
- `node/opennet/manager.go`

### Week 27: Darknet
- [ ] Node reference parsing
- [ ] Manual peer adding
- [ ] Friend connections
- [ ] Persistent peers

**Files:**
- `node/darknet/manager.go`
- `node/darknet/noderef.go`

### Week 28: Network Statistics
- [ ] Stats tracking
- [ ] Bandwidth stats
- [ ] Success rates
- [ ] Load stats
- [ ] Peer stats

**Files:**
- `node/stats/stats.go`
- `node/stats/collector.go`

---

## Phase 8: Testing & Hardening (Weeks 29-32)

### Week 29: Unit Tests
- [ ] Crypto tests
- [ ] Protocol tests
- [ ] Routing tests
- [ ] Storage tests

### Week 30: Integration Tests
- [ ] Full handshake test
- [ ] Data insert/fetch test
- [ ] Multi-node test
- [ ] Load test

### Week 31: Interop Testing
- [ ] Connect to real Fred nodes
- [ ] Exchange data with Fred
- [ ] FCP compatibility
- [ ] Protocol compatibility

### Week 32: Bug Fixes & Optimization
- [ ] Performance profiling
- [ ] Memory optimization
- [ ] Connection stability
- [ ] Error handling

---

## Phase 9: Features & Polish (Weeks 33+)

### Additional Features
- [ ] Web interface
- [ ] Configuration management
- [ ] Logging system
- [ ] Monitoring/metrics
- [ ] Auto-update
- [ ] Platform packages (deb, rpm, etc.)

---

## Architecture Overview

```
hyphanet-go/
├── node/
│   ├── crypto/         # Cryptographic primitives ✅
│   ├── transport/      # UDP transport ✅
│   ├── protocol/       # JFK, NPF protocols
│   ├── peer/           # Peer management
│   ├── session/        # Session tracking
│   ├── message/        # Message handling
│   ├── storage/        # Block storage
│   ├── keys/           # Key system
│   ├── routing/        # Location & routing
│   ├── request/        # Request handling
│   ├── fcp/            # FCP server
│   ├── opennet/        # Opennet management
│   ├── darknet/        # Darknet management
│   ├── stats/          # Statistics
│   └── node.go         # Main node ✅
│
├── cmd/
│   ├── hyphanet/       # Main node binary
│   ├── hyphanetctl/    # Control utility
│   └── tools/          # Debugging tools
│
├── pkg/
│   └── util/           # Shared utilities
│
└── test/               # Integration tests
```

## Success Metrics

### Phase 1-2 Complete
- ✅ Can connect to seed nodes
- ✅ Can complete handshake
- ✅ Can send/receive messages

### Phase 3-4 Complete
- ✅ Can store blocks
- ✅ Can route requests
- ✅ Can find closest peers

### Phase 5-6 Complete
- ✅ Can fetch CHK blocks
- ✅ Can insert CHK blocks
- ✅ Can serve FCP clients

### Phase 7-8 Complete
- ✅ Can join opennet
- ✅ Can route to other nodes
- ✅ Passes compatibility tests

### Phase 9 Complete
- ✅ Production ready
- ✅ Packaged for distribution
- ✅ Documented

---

## Current Status

- **Completed:** Phase 1 (50%) - Network layer foundation
- **Next:** Complete JFK handshake, then peer management
- **ETA:** Production-ready node in 6-12 months with dedicated effort

## Resources Needed

- **Time:** 20-40 hours/week
- **Testing:** Multiple machines/VMs for multi-node testing
- **Reference:** Fred codebase for protocol details
- **Community:** Hyphanet developers for protocol questions

---

Let's build this! 🚀
