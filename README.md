# GoHyphanet

A comprehensive suite of command-line tools for interacting with [Hyphanet](https://www.hyphanet.org/) (formerly Freenet), written in Go.

[![License: AGPL v3](https://img.shields.io/badge/License-AGPL%20v3-blue.svg)](https://www.gnu.org/licenses/agpl-3.0)
[![Go Version](https://img.shields.io/badge/Go-1.21%2B-00ADD8?logo=go)](https://golang.org/)

## 🌟 Features

- **Complete FCP Implementation** - Full support for Hyphanet's FCP (Freenet Client Protocol)
- **Modern CLI Tools** - Fast, reliable command-line utilities with progress bars and debug modes
- **Freesite Management** - Easy creation and deployment of Hyphanet websites
- **Website Mirroring** - Clone existing websites and republish them on Hyphanet
- **HTTP Proxy** - Browse Hyphanet sites using human-friendly domain names
- **BitTorrent over Hyphanet** - Anonymous, censorship-resistant file sharing
- **Web Tunnel** - Access clearnet websites through Hyphanet's anonymous network
- **Key Management** - SQLite and JSON-based keystore for managing your Hyphanet keys
- **Cross-Platform** - Works on Linux, macOS, and Windows

## 📦 Installation

### Prerequisites

- Go 1.21 or higher
- Access to a running Hyphanet node (FCP port 9481)
- wget (for `copyweb` tool)

### Building from Source

```bash
git clone https://github.com/blubskye/gohyphanet.git
cd gohyphanet
./build.sh
```

Or using Make:

```bash
make build
```

### Installing System-Wide

```bash
sudo cp fcp* copyweb fproxyproxy torrentproxy tunnel* /usr/local/bin/
```

Or to your Go bin directory:

```bash
make install
```

## 🛠️ Tools

### fcpget - Download from Hyphanet

Download files and freesites from Hyphanet with progress tracking.

```bash
# Download to stdout
fcpget KSK@mykey

# Download to file with progress
fcpget CHK@abc123... -o output.txt --progress

# Download multiple files
fcpget CHK@file1 CHK@file2 CHK@file3 -d downloads/

# Debug stuck downloads
FCP_DEBUG=1 fcpget CHK@... --debug --progress
```

**Features:**
- Progress bars with download speed
- Multiple file downloads
- Automatic retries
- Debug mode for troubleshooting
- Configurable timeouts

### fcpput - Upload to Hyphanet

Insert files into Hyphanet with real-time progress tracking.

```bash
# Upload file to KSK
fcpput KSK@mykey -i file.txt --progress

# Generate CHK from stdin
echo "Hello Hyphanet" | fcpput --progress

# Upload with debug output
fcpput -i large.zip --debug --progress
```

**Features:**
- Progress bars with upload speed
- Multiple key types (CHK, KSK, SSK, USK)
- Compression options
- Priority control
- Automatic retries

### fcpsitemgr - Freesite Manager

Complete freesite management system for creating and deploying Hyphanet websites.

```bash
# Initialize a new site
fcpsitemgr init mysite ./website

# Generate keys for the site
fcpsitemgr genkey mysite

# Upload site with progress
fcpsitemgr upload mysite --progress --debug

# List all sites
fcpsitemgr list

# Get site information
fcpsitemgr info mysite
```

**Features:**
- Automatic site initialization
- USK-based versioning
- Progress tracking for uploads
- Multi-file uploads with manifest
- MIME type detection
- Version management

### copyweb - Website Mirroring

Clone existing websites and prepare them for Hyphanet deployment.

```bash
# Download a single page
copyweb https://example.com -d example_site

# Mirror entire site
copyweb https://example.com --mirror -d example_site

# Download and upload to Hyphanet in one command
copyweb https://example.com --mirror --upload --site mysite -d mysite --debug
```

**Features:**
- Uses wget for robust downloading
- Automatic link conversion
- Resource downloading (images, CSS, JS)
- Optional auto-upload to Hyphanet
- Integration with fcpsitemgr

### fproxyproxy - HTTP Proxy with Name Resolution

HTTP proxy that translates human-friendly names to Hyphanet URIs.

```bash
# Start proxy
fproxyproxy

# Custom addresses
fproxyproxy -listen :8080 -fproxy localhost:8888

# With debug output
fproxyproxy --debug
```

**Features:**
- Web-based admin interface
- Name-to-URI mapping
- Transparent proxying through FProxy
- JSON-based name storage
- Direct FProxy access support

**Usage:**
1. Start fproxyproxy
2. Configure as HTTP proxy in browser (127.0.0.1:8889)
3. Visit `http://localhost:8889/admin` to manage names
4. Access sites: `http://mysite.hyphanet/`

### torrentproxy - BitTorrent over Hyphanet

Route BitTorrent traffic through Hyphanet's anonymous network.

```bash
# Start proxy
torrentproxy

# Custom configuration
torrentproxy -tracker :7000 -peer :6882 --debug
```

**Features:**
- BitTorrent protocol support
- Tracker emulation
- Hyphanet storage backend
- Anonymous file sharing
- Torrent registry database

**Usage:**
1. Start torrentproxy
2. Configure torrent client:
   - Tracker: `http://127.0.0.1:6969/announce`
   - Port: 6881
3. Add torrents and share anonymously!

### tunnelserver - Clearnet Server for Web Tunnel

Run a server on the clearnet that proxies web requests through Hyphanet.

```bash
# Start tunnel server
tunnelserver

# With custom admin interface
tunnelserver --admin :9090 --debug
```

**Features:**
- Rate limiting
- Allowlist/blocklist support
- Web admin interface
- Statistics tracking
- Request/response queue via Hyphanet

**Usage:**
1. Run on a clearnet-accessible server
2. Share the public key with clients
3. Monitor via admin interface at `http://localhost:8080`

### tunnelclient - Client for Accessing Clearnet via Tunnel

Connect to a tunnel server through Hyphanet to access clearnet anonymously.

```bash
# Connect to tunnel server
tunnelclient -server "SSK@abc123.../" --debug

# Custom proxy port
tunnelclient -server "SSK@.../" --proxy :9999
```

**Features:**
- Local HTTP proxy
- Automatic request routing
- Response polling
- Statistics display
- Browser integration

**Usage:**
1. Start client with server's public key
2. Configure browser to use proxy at 127.0.0.1:8890
3. Browse the web through Hyphanet!

### fcpkey - Key Management

Manage your Hyphanet cryptographic keys.

```bash
# Generate a new key pair
fcpkey generate mysite

# List all keys
fcpkey list

# Export keys
fcpkey export mysite

# Import keys
fcpkey import mysite SSK@...
```

### fcpctl - FCP Control Utility

Low-level FCP operations and node information.

```bash
# Get node information
fcpctl info

# Check connection
fcpctl ping
```

## 📖 Quick Start Guide

### 1. Upload Your First File

```bash
# Create a test file
echo "Hello, Hyphanet!" > test.txt

# Upload and get the CHK
./fcpput -i test.txt --progress

# Output: CHK@abc123.../test.txt
```

### 2. Create and Deploy a Freesite

```bash
# Create a simple website
mkdir mysite
echo '<html><body><h1>My Freesite</h1></body></html>' > mysite/index.html

# Initialize the site
./fcpsitemgr init mysite ./mysite

# Generate keys
./fcpsitemgr genkey mysite

# Upload to Hyphanet
./fcpsitemgr upload mysite --progress

# Output: Your site is available at USK@.../mysite/0/
```

### 3. Mirror an Existing Website

```bash
# Mirror and upload in one command
./copyweb https://example.com --mirror --upload --site example -d example
```

### 4. Browse with Friendly Names

```bash
# Start the proxy
./fproxyproxy

# Configure browser to use proxy at 127.0.0.1:8889
# Visit http://localhost:8889/admin
# Add mapping: example.hyphanet -> USK@.../example/0/
# Browse to: http://example.hyphanet/
```

### 5. Share Files Anonymously via BitTorrent

```bash
# Start torrent proxy
./torrentproxy --debug

# Configure torrent client:
#   Tracker: http://127.0.0.1:6969/announce
#   Port: 6881

# Add torrents and start sharing!
```

### 6. Access Clearnet Through Hyphanet Tunnel

**On a clearnet server:**
```bash
# Start tunnel server
./tunnelserver

# Note the public key displayed
# Share this key with clients
```

**On client (censored network):**
```bash
# Start tunnel client
./tunnelclient -server "SSK@server-public-key.../"

# Configure browser:
#   Proxy: 127.0.0.1
#   Port: 8890

# Browse any website through Hyphanet!
```

## 🔧 Configuration

### Environment Variables

- `FCP_HOST` - Hyphanet node hostname (default: localhost)
- `FCP_PORT` - FCP port (default: 9481)
- `FCP_DEBUG` - Enable FCP protocol debugging (1 to enable)

### Debug Mode

Enable comprehensive debug output for troubleshooting:

```bash
FCP_DEBUG=1 ./fcpsitemgr upload mysite --debug --progress
```

This shows:
- All FCP messages sent and received
- Connection status
- Progress updates
- Handler registrations
- Timing information

## 📝 Examples

### Download a Freesite

```bash
# Download a specific page
./fcpget "USK@abc.../sitename/5/index.html" -o index.html --progress
```

### Upload Multiple Files

```bash
# Upload several files at once
./fcpget CHK@file1 CHK@file2 CHK@file3 -d downloads/ --progress
```

### Create a Blog Freesite

```bash
# Create directory structure
mkdir -p myblog/{posts,images,css}

# Add content
echo '<html>...</html>' > myblog/index.html

# Initialize and upload
./fcpsitemgr init myblog ./myblog
./fcpsitemgr genkey myblog
./fcpsitemgr upload myblog --progress
```

### Update a Freesite

```bash
# Make changes to your site
echo 'Updated content' > mysite/news.html

# Upload new version (version auto-increments)
./fcpsitemgr upload mysite --progress

# New version available at USK@.../mysite/1/
```

### Anonymous Torrenting

```bash
# Start torrent proxy
./torrentproxy

# In your torrent client (qBittorrent, Transmission, etc.):
# 1. Set tracker to: http://127.0.0.1:6969/announce
# 2. Set port to: 6881
# 3. Add torrents - all traffic goes through Hyphanet!
```

### Bypass Censorship

```bash
# On clearnet server
./tunnelserver --debug

# On client in censored region
./tunnelclient -server "SSK@server-key.../" --debug

# Configure browser proxy to 127.0.0.1:8890
# Access any blocked website!
```

## 🏗️ Architecture

### FCP Library (`fcp/`)

Core library implementing Hyphanet's FCP protocol:

- **client.go** - FCP connection and message handling
- **operations.go** - High-level operations (get, put)
- **keymanager.go** - Key storage (JSON and SQLite)
- **version.go** - Version and license information

### Command-Line Tools (`cmd/`)

Each tool is a separate package:

- `fcpget/` - Download utility
- `fcpput/` - Upload utility
- `fcpsitemgr/` - Freesite manager
- `copyweb/` - Website mirroring
- `fproxyproxy/` - HTTP proxy with name resolution
- `torrentproxy/` - BitTorrent over Hyphanet
- `tunnelserver/` - Clearnet server for web tunnel
- `tunnelclient/` - Client for web tunnel
- `fcpkey/` - Key management
- `fcpctl/` - Control utility

## 🤝 Contributing

Contributions are welcome! Please feel free to submit pull requests or open issues.

### Development Setup

```bash
git clone https://github.com/blubskye/gohyphanet.git
cd gohyphanet
go mod download
make build
```

### Running Tests

```bash
make test
```

## 📜 License

This program is free software: you can redistribute it and/or modify it under the terms of the GNU Affero General Public License as published by the Free Software Foundation, either version 3 of the License, or (at your option) any later version.

See [LICENSE](LICENSE) file for details.

## 🙏 Credits and Acknowledgments

GoHyphanet is inspired by and builds upon the excellent work of:

- **[pyFreenet](https://github.com/hyphanet/pyFreenet)** - The original Python implementation of Freenet/Hyphanet tools. Many of GoHyphanet's features, especially `fproxyproxy` and the name service concept, are directly inspired by pyFreenet's innovative approach to making Hyphanet more user-friendly. Special thanks to the pyFreenet contributors for pioneering these ideas.

- **[Hyphanet Project](https://www.hyphanet.org/)** - The peer-to-peer platform for censorship-resistant communication and publishing.

- **FCP Protocol** - The Freenet Client Protocol that makes all of this possible.

GoHyphanet aims to provide a modern, performant Go implementation while honoring the design principles and user experience innovations pioneered by pyFreenet.

## 🔗 Links

- **Hyphanet Website**: https://www.hyphanet.org/
- **pyFreenet**: https://github.com/hyphanet/pyFreenet
- **FCP Documentation**: https://github.com/hyphanet/wiki/wiki/FCPv2
- **Report Issues**: https://github.com/blubskye/gohyphanet/issues

## 📊 Project Status

GoHyphanet is under active development. Current status:

- ✅ Core FCP implementation
- ✅ File upload/download with progress
- ✅ Freesite management
- ✅ Website mirroring
- ✅ HTTP proxy with name service
- ✅ BitTorrent over Hyphanet
- ✅ Web tunnel (clearnet access)
- ✅ Key management (JSON & SQLite)
- 🚧 Advanced name service features (in progress)
- 🚧 DHT support for torrents (planned)
- 🚧 Comprehensive test coverage (in progress)
- 📋 Documentation improvements (ongoing)

## 💡 Use Cases

### 🔒 Privacy & Anonymity
- **Anonymous Browsing**: Access websites without revealing your IP
- **Private File Sharing**: Share files through BitTorrent over Hyphanet
- **Secure Publishing**: Publish content that can't be taken down

### 🌍 Censorship Resistance
- **Bypass Blocks**: Access blocked websites via web tunnel
- **Unrestricted Access**: Share information in restricted regions
- **Freedom of Speech**: Publish without fear of censorship

### 📚 Content Preservation
- **Archive Websites**: Mirror important content to Hyphanet
- **Distribute Knowledge**: Share educational materials freely
- **Historical Records**: Preserve content that might be removed

### 🛠️ Development & Research
- **Test Networks**: Experiment with distributed systems
- **Academic Research**: Study anonymous networks
- **Protocol Development**: Build on Hyphanet's infrastructure

## 💡 Tips and Tricks

### Speed Up Downloads

```bash
# Use higher priority (lower number = higher priority)
fcpget CHK@... -o file.txt --priority 1
```

### Batch Operations

```bash
# Upload multiple files
for file in *.txt; do
    fcpput -i "$file" KSK@myfiles/$file
done
```

### Monitor Upload Progress

```bash
# Detailed progress with debug info
FCP_DEBUG=1 fcpsitemgr upload mysite --progress --debug 2>&1 | tee upload.log
```

### Backup Your Keys

```bash
# Export all keys to backup
fcpkey export --all > keys-backup.json
```

### Run Tunnel Server as Service

```bash
# Create systemd service file
sudo nano /etc/systemd/system/tunnelserver.service

# Add:
[Unit]
Description=Hyphanet Tunnel Server
After=network.target

[Service]
ExecStart=/usr/local/bin/tunnelserver
Restart=always
User=tunnelserver

[Install]
WantedBy=multi-user.target

# Enable and start
sudo systemctl enable tunnelserver
sudo systemctl start tunnelserver
```

## ⚠️ Security & Privacy Notes

### General Security
- Always verify Hyphanet URIs before accessing
- Use HTTPS when available (even through tunnel)
- Keep your private keys secure
- Regularly update GoHyphanet

### BitTorrent Privacy
- Traffic is routed through Hyphanet's anonymous network
- Configure your torrent client to prevent IP leaks
- Be aware that download speeds will be slower
- Only share legal content

### Web Tunnel Security
- **Server operators**: Can see clearnet requests from clients
- **Clients**: Should only use trusted tunnel servers
- **Both**: Use allowlists/blocklists appropriately
- **Encryption**: Always prefer HTTPS when tunneling

### Freesite Publishing
- Your freesite's insert key is your password - keep it safe!
- Request keys are public - share them freely
- Version numbers prevent content replacement
- Consider what metadata you include

## 🐛 Troubleshooting

### Connection Issues

```bash
# Check if Hyphanet is running
./fcpctl info

# Verify FCP port
netstat -an | grep 9481

# Enable debug mode
FCP_DEBUG=1 ./fcpget CHK@... --debug
```

### Slow Performance

- Hyphanet is slower than direct connections by design
- Increase timeout values if needed
- Use progress mode to monitor activity
- Consider running multiple operations in parallel

### Upload/Download Stuck

```bash
# Check progress with debug
FCP_DEBUG=1 ./fcpput -i file.zip --debug --progress

# Monitor FCP messages
# Look for SimpleProgress messages
# Check if blocks are being uploaded/downloaded
```

### Tunnel Not Working

```bash
# Server: Check if running
curl http://localhost:8080/stats

# Client: Verify server key
./tunnelclient -server "SSK@correct-key.../" --debug

# Check browser proxy settings
# Verify FCP connection on both ends
```

---

**Made with ❤️ for the Hyphanet community**

**❤️ Hyphanet going crazy!❤️ ~Cynthia**


*Preserving freedom of speech and privacy in the digital age*
