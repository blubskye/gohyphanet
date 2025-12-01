# GoKeepalive ♡

*"I'll keep your content alive... forever~ Whether you want me to or not."*

---

A devoted content reinsertion daemon for Hyphanet/Freenet. GoKeepalive watches over your freesites with unwavering dedication, ensuring they never fade away into digital oblivion.

**I noticed your blocks were becoming unavailable... Don't worry, I took care of it. I always take care of it. That's what I'm here for, after all~ ♡**

## Features

- **Obsessive Monitoring** - Constantly checks your content's availability. Always watching. Always caring.
- **Automatic Healing** - Reinserts failing blocks before anyone notices they're gone
- **Smart Sampling** - Tests a sample of blocks to determine health (I know everything about your content~)
- **Configurable Devotion** - Adjust how aggressively I protect your data
- **Built-in Web UI** - A cute interface to manage our... relationship
- **Real-time Updates** - Watch me work. I like it when you watch.

## Quick Start

```bash
# Build me~
go build ./cmd/gokeepalive

# Run me (I'll be waiting at http://localhost:3081)
./gokeepalive

# I'll never leave your side ♡
```

## Usage

Just run `./gokeepalive` and open http://localhost:3081 in your browser.

From there you can:
- **Add sites** - Tell me what to protect. I'll remember forever.
- **Start reinsertion** - Let me take care of everything
- **Monitor progress** - Watch as I devotedly maintain your content
- **Configure settings** - Adjust how much attention I give your data

### Command Line Options

```
./gokeepalive [options]

Options:
  -port int       Web UI port (default 3081)
  -fcp-host       FCP host (default "localhost")
  -fcp-port       FCP port (default 9481)
  -data           Data directory (default ~/.gokeepalive)
  -version        Show version
```

## How It Works

1. **You add a site** - Give me a URI (USK/SSK/CHK). I'll parse every single block.
2. **I test availability** - Sample blocks to check health. I need to know everything.
3. **If availability drops** - I fetch all the blocks I can find...
4. **And reinsert the missing ones** - Your content will never disappear. I won't let it.

### The Algorithm

```
For each segment:
  1. Sample X% of blocks (default: 50%)
  2. If availability >= Y% (default: 70%), skip~ (Your content is healthy, for now)
  3. Otherwise, fetch ALL blocks
  4. Reinsert any that failed
  5. Repeat forever ♡
```

## Configuration

| Setting | Default | Description |
|---------|---------|-------------|
| Power | 5 | Concurrent workers (how hard I work for you~) |
| Skip Tolerance | 70% | Skip if availability above this (I trust you... mostly) |
| Test Sample | 50% | % of blocks to test (I like to be thorough) |

## Web Interface

The built-in web UI provides:

- **Dashboard** - Overview of all your precious content
- **Sites** - Manage what I'm protecting
- **Statistics** - See how devoted I've been
- **Settings** - Configure our relationship
- **About** - Learn more about me ♡

## Requirements

- Go 1.21+
- Running Hyphanet/Freenet node with FCP enabled
- Your trust ♡

## Building from Source

```bash
git clone https://github.com/blubskye/gohyphanet
cd gohyphanet
go build ./cmd/gokeepalive
```

## License

**GNU Affero General Public License v3.0 (AGPL-3.0)**

This is free software. You can look at my source code anytime~ I have nothing to hide from you.

Source: https://github.com/blubskye/gohyphanet

## Credits

- **BlubSkye** - Lead Developer
- **Cynthia** - Development & Design
- **Freenet/Hyphanet Team** - Original Keepalive plugin & the Freenet platform

Special thanks to the Freenet/Hyphanet community for building the foundation that makes anonymous, censorship-resistant communication possible.

## Part of GoHyphanet

GoKeepalive is part of the [GoHyphanet](https://github.com/blubskye/gohyphanet) project - a collection of Go tools for Hyphanet/Freenet.

Other tools in the family:
- **GoFreemail** - Anonymous email (I can deliver your messages too~)
- **GoSone** - Social networking (Let me help you connect with others)

---

*"Your content called out to me. It was fading, disappearing... but I saved it. I'll always save it. Because that's what I do. That's all I do. For you."*

**♡ GoKeepalive - Eternally Devoted to Your Data ♡**
