# GoFreemail Implementation Plan

A Go port of the Freemail plugin for Hyphanet/Freenet - anonymous email over Freenet.

## Project Status

**Current Progress:** ALL PHASES COMPLETE (~8,000 LOC)

| Phase | Status | Description |
|-------|--------|-------------|
| 1 | ✅ Complete | Data model (Account, Message, Channel) |
| 2 | ✅ Complete | Channel/transport layer (RTS, slots, encryption) |
| 3 | ✅ Complete | SMTP server |
| 4 | ✅ Complete | IMAP server |
| 5 | ✅ Complete | Web UI |
| 6 | ✅ Complete | CLI tool |
| 7 | 🔲 Pending | Testing |

---

## Architecture Overview

Freemail provides anonymous email over Freenet using:
- **Web of Trust** for identity management
- **RTS (Ready-To-Send)** protocol for establishing channels
- **Slot-based messaging** for async communication
- **AES-256 + RSA** encryption for message security
- **SMTP/IMAP** servers for standard email client compatibility

---

## Phase 1: Data Model

### 1.1 Core Types
- [ ] `Account` - User account with identity, keys, message bank
- [ ] `Message` - Email message with headers, body, flags
- [ ] `MessageBank` - Hierarchical folder storage (inbox, sent, etc.)
- [ ] `EmailAddress` - Freemail address parsing (user@identity.freemail)
- [ ] `Channel` - Encrypted communication channel between accounts

### 1.2 Storage
- [ ] `AccountStore` - Persistent account storage
- [ ] `MessageStore` - File-based message storage with IMAP UIDs
- [ ] `ChannelStore` - Channel state persistence

**Files to create:**
- `freemail/model.go`
- `freemail/account.go`
- `freemail/message.go`
- `freemail/channel.go`
- `freemail/storage.go`

---

## Phase 2: Channel/Transport Layer

### 2.1 RTS Protocol
- [ ] `RTSFetcher` - Poll for incoming RTS messages
- [ ] `RTSSender` - Send RTS to establish channels
- [ ] RTS message format (RSA-encrypted AES key + encrypted payload)
- [ ] Signature verification using sender's public key

### 2.2 Slot Management
- [ ] `SlotManager` - Track used/unused/expired slots
- [ ] Poll-ahead strategy (check slots beyond last used)
- [ ] 7-day slot expiration

### 2.3 Encryption
- [ ] AES-256 symmetric encryption for messages
- [ ] RSA key wrapping for AES keys
- [ ] SHA-256 signatures

### 2.4 MailSite
- [ ] Publish account metadata to Freenet
- [ ] Include RTS key, public key, and slot info

**Files to create:**
- `freemail/rts.go`
- `freemail/slots.go`
- `freemail/crypto.go`
- `freemail/mailsite.go`
- `freemail/transport.go`

---

## Phase 3: SMTP Server

### 3.1 SMTP Commands
- [ ] HELO/EHLO - Greeting
- [ ] AUTH - LOGIN and PLAIN authentication
- [ ] MAIL FROM - Sender specification
- [ ] RCPT TO - Recipient specification
- [ ] DATA - Message content
- [ ] RSET - Reset state
- [ ] QUIT - Close connection

### 3.2 Integration
- [ ] Validate recipients against WoT
- [ ] Queue messages for delivery via channels
- [ ] Support standard email clients

**Files to create:**
- `freemail/smtp/server.go`
- `freemail/smtp/handler.go`
- `freemail/smtp/commands.go`

---

## Phase 4: IMAP Server

### 4.1 IMAP Commands
- [ ] LOGIN/LOGOUT - Authentication
- [ ] CAPABILITY - Feature list
- [ ] LIST/LSUB - Mailbox listing
- [ ] SELECT/EXAMINE - Open mailbox
- [ ] FETCH - Retrieve messages
- [ ] STORE - Modify flags
- [ ] SEARCH - Find messages
- [ ] COPY - Copy messages
- [ ] EXPUNGE - Delete marked messages
- [ ] APPEND - Add messages
- [ ] CREATE/DELETE - Mailbox management
- [ ] NOOP/CHECK - Maintenance

### 4.2 IMAP Features
- [ ] UID support for all commands
- [ ] Message flags (seen, deleted, flagged, etc.)
- [ ] Folder hierarchy with dot separator

**Files to create:**
- `freemail/imap/server.go`
- `freemail/imap/handler.go`
- `freemail/imap/commands.go`
- `freemail/imap/parser.go`

---

## Phase 5: Web UI

### 5.1 Pages
- [ ] Account management
- [ ] Inbox view
- [ ] Compose message
- [ ] Read message
- [ ] Folder management
- [ ] Settings

### 5.2 Features
- [ ] Real-time message updates
- [ ] Contact list from WoT
- [ ] Message search

**Files to create:**
- `freemail/web/server.go`
- `freemail/web/handlers.go`
- `freemail/web/templates/`

---

## Phase 6: CLI Tool

### 6.1 Commands
- [ ] `gofreemail accounts` - List accounts
- [ ] `gofreemail create` - Create account
- [ ] `gofreemail send` - Send message
- [ ] `gofreemail inbox` - List inbox
- [ ] `gofreemail read` - Read message
- [ ] `gofreemail serve` - Start SMTP/IMAP servers

**Files to create:**
- `cmd/gofreemail/main.go`

---

## Phase 7: Testing

### 7.1 Unit Tests
- [ ] Model tests
- [ ] Crypto tests
- [ ] SMTP protocol tests
- [ ] IMAP protocol tests

### 7.2 Integration Tests
- [ ] End-to-end message flow
- [ ] Channel establishment
- [ ] Email client compatibility

---

## File Structure (Target)

```
gohyphanet/
├── freemail/
│   ├── model.go           # Core data types
│   ├── account.go         # Account management
│   ├── message.go         # Message handling
│   ├── channel.go         # Channel management
│   ├── storage.go         # Persistence layer
│   ├── rts.go             # RTS protocol
│   ├── slots.go           # Slot management
│   ├── crypto.go          # Encryption utilities
│   ├── mailsite.go        # MailSite publishing
│   ├── transport.go       # Message transport
│   ├── smtp/              # SMTP server
│   │   ├── server.go
│   │   ├── handler.go
│   │   └── commands.go
│   ├── imap/              # IMAP server
│   │   ├── server.go
│   │   ├── handler.go
│   │   ├── commands.go
│   │   └── parser.go
│   └── web/               # Web interface
│       ├── server.go
│       ├── handlers.go
│       └── templates/
├── cmd/
│   └── gofreemail/        # CLI tool
│       └── main.go
└── GOFREEMAIL_PLAN.md     # This file
```

---

## Key Concepts

### Freemail Address Format
```
nickname@identity.freemail
```
Where `identity` is the Base32-encoded WoT identity hash.

### RTS (Ready-To-Send) Handshake
1. Sender publishes RTS message to recipient's RTS key
2. RTS contains: encrypted AES key (RSA), encrypted payload (AES)
3. Payload includes: sender mailsite URI, channel spec, slot assignments
4. Recipient verifies signature and establishes channel

### Slot-Based Messaging
- Each channel has sender slots and receiver slots
- Messages inserted at incrementing slot numbers
- Slots expire after 7 days
- "Poll ahead" checks slots beyond last used

### Channel Lifecycle
1. **Active** (read-write): Both parties can send
2. **Read-only**: Channel nearing expiration
3. **Inactive**: Channel expired, need new RTS

---

## Dependencies

- `gohyphanet/fcp` - FCP client library
- `gohyphanet/wot` - Web of Trust client
- Standard library: `crypto/aes`, `crypto/rsa`, `crypto/sha256`

---

## Default Ports

- SMTP: 3025
- IMAP: 3143
- Web UI: 3080

---

## References

- Java Freemail: https://github.com/hyphanet/plugin-Freemail
- Freenet FCP: https://github.com/freenet/wiki/wiki/FCPv2
