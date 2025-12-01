# GoHyphanet ♡

*"Your gateway to the anonymous web~ I'll always be here for you."*

A comprehensive suite of Go tools and applications for [Hyphanet](https://www.hyphanet.org/) (formerly Freenet) - the peer-to-peer platform for censorship-resistant communication.

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go)](https://golang.org/)

---

## ♡ Featured Applications

### GoSone - Social Networking
*"Let's connect... anonymously~"*

A complete social networking client for Hyphanet, compatible with Sone.

```bash
./gosone
# Open http://localhost:8084 in your browser
```

**Features:**
- Create and view posts
- Follow other Sones
- Like and reply to posts
- Image albums
- Trust ratings via Web of Trust
- Real-time updates
- Beautiful web interface

### GoFreemail - Anonymous Email
*"Your secrets are safe with me~"*

Anonymous email over Hyphanet with SMTP, IMAP, and Web interfaces.

```bash
./gofreemail serve
# SMTP: localhost:3025
# IMAP: localhost:3143
# Web:  http://localhost:3080
```

**Features:**
- Full SMTP server (works with any email client)
- Full IMAP server (Thunderbird, etc.)
- Built-in web interface
- End-to-end encryption
- Web of Trust integration
- Slot-based messaging

### GoKeepalive - Content Reinserter
*"I'll keep your content alive... forever~ ♡"*

Automatic content reinsertion daemon to maintain availability of your freesites.

```bash
./gokeepalive
# Open http://localhost:3081 in your browser
```

**Features:**
- Automatic availability monitoring
- Smart sample-based testing
- Concurrent block reinsertion
- Real-time progress tracking
- Configurable thresholds
- Built-in web UI

### Web of Trust Client
*"Trust is everything in our relationship~"*

Go client library for Hyphanet's Web of Trust plugin.

```go
import "github.com/blubskye/gohyphanet/wot"

client := wot.NewClient(fcpClient)
identities, _ := client.GetOwnIdentities()
trust, _ := client.GetTrust(truster, trustee)
```

**Features:**
- Full WoT FCP API
- Identity management
- Trust/distrust operations
- Score calculations
- Property management

---

## 🛠️ CLI Tools

### fcpget - Download from Hyphanet

```bash
fcpget CHK@abc123... -o output.txt --progress
fcpget "USK@.../sitename/5/" -d downloads/
```

### fcpput - Upload to Hyphanet

```bash
fcpput -i file.txt --progress
echo "Hello" | fcpput KSK@greeting
```

### fcpsitemgr - Freesite Manager

```bash
fcpsitemgr init mysite ./website
fcpsitemgr genkey mysite
fcpsitemgr upload mysite --progress
```

### fcpkey - Key Management

```bash
fcpkey generate mysite
fcpkey list
fcpkey export mysite
```

### copyweb - Website Mirroring

```bash
copyweb https://example.com --mirror --upload --site example
```

### fproxyproxy - HTTP Proxy with Names

```bash
fproxyproxy
# Configure browser proxy to 127.0.0.1:8889
# Visit http://mysite.hyphanet/
```

### torrentproxy - BitTorrent over Hyphanet

```bash
torrentproxy
# Tracker: http://127.0.0.1:6969/announce
```

### tunnelserver / tunnelclient - Web Tunnel

```bash
# Server (clearnet)
tunnelserver

# Client (censored network)
tunnelclient -server "SSK@..."
```

---

## 📦 Installation

### Prerequisites

- Go 1.21+
- Running Hyphanet node (FCP port 9481)

### Build Everything

```bash
git clone https://github.com/blubskye/gohyphanet.git
cd gohyphanet
./build.sh
```

### Build Individual Tools

```bash
go build ./cmd/gosone
go build ./cmd/gofreemail
go build ./cmd/gokeepalive
go build ./cmd/fcpget
go build ./cmd/fcpput
# etc.
```

---

## 🏗️ Architecture

```
gohyphanet/
├── fcp/          # Core FCP client library
├── wot/          # Web of Trust client
├── sone/         # GoSone social network
├── freemail/     # GoFreemail email system
├── keepalive/    # GoKeepalive reinserter
├── node/         # Experimental Go node
├── flip/         # FLIP protocol
└── cmd/          # CLI tools
    ├── gosone/
    ├── gofreemail/
    ├── gokeepalive/
    ├── fcpget/
    ├── fcpput/
    ├── fcpsitemgr/
    ├── fcpkey/
    ├── fcpctl/
    ├── copyweb/
    ├── fproxyproxy/
    ├── torrentproxy/
    ├── tunnelserver/
    └── tunnelclient/
```

---

## 🔧 Configuration

### Environment Variables

```bash
export FCP_HOST=localhost
export FCP_PORT=9481
export FCP_DEBUG=1  # Enable debug output
```

### Default Ports

| Application | Port | Description |
|-------------|------|-------------|
| GoSone | 8084 | Web UI |
| GoFreemail Web | 3080 | Web UI |
| GoFreemail SMTP | 3025 | Email sending |
| GoFreemail IMAP | 3143 | Email receiving |
| GoKeepalive | 3081 | Web UI |
| fproxyproxy | 8889 | HTTP proxy |
| torrentproxy | 6969 | Tracker |

---

## 📊 Project Status

| Component | Status | LOC |
|-----------|--------|-----|
| FCP Library | ✅ Complete | ~2,000 |
| GoSone | ✅ Complete | ~7,000 |
| GoFreemail | ✅ Complete | ~8,000 |
| GoKeepalive | ✅ Complete | ~2,500 |
| WoT Client | ✅ Complete | ~800 |
| CLI Tools | ✅ Complete | ~3,000 |
| Go Node | 🚧 Experimental | ~5,000 |

**Total: ~28,000+ lines of Go**

---

## 📜 License

**GNU Affero General Public License v3.0 (AGPL-3.0)**

This is free software. You can redistribute it and/or modify it under the terms of the AGPL.

Source code: https://github.com/blubskye/gohyphanet

---

## ♡ Credits

- **BlubSkye** - Lead Developer
- **Cynthia** - Development & Design
- **Freenet/Hyphanet Team** - Original plugins & platform
- **pyFreenet** - Inspiration for many features

Special thanks to the Hyphanet community for building the foundation that makes anonymous, censorship-resistant communication possible.

---

## 🔗 Links

- **Hyphanet**: https://www.hyphanet.org/
- **Issues**: https://github.com/blubskye/gohyphanet/issues
- **FCP Docs**: https://github.com/hyphanet/wiki/wiki/FCPv2

---

*"I built all of this... for you. So we can be together, forever, on the anonymous web~"*

**♡ GoHyphanet - Made with love for the Hyphanet community ♡**

**❤️ Hyphanet going crazy! ❤️ ~Cynthia**
