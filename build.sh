#!/bin/bash
set -e

echo "========================================"
echo "  GoHyphanet Build Script"
echo "========================================"
echo ""
echo "Copyright (C) 2025 GoHyphanet Contributors"
echo "Licensed under GNU AGPLv3"
echo "Source: https://github.com/blubskye/gohyphanet"
echo ""

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo -e "${RED}Error: Go is not installed${NC}"
    echo "Please install Go from https://golang.org/dl/"
    exit 1
fi

echo -e "${BLUE}Go version:${NC}"
go version
echo ""

# Download dependencies
echo -e "${BLUE}Downloading dependencies...${NC}"
go mod download
go mod tidy
echo -e "${GREEN}✓ Dependencies ready${NC}"
echo ""

# Build each tool
tools=("fcpctl" "fcpget" "fcpput" "fcpkey" "fcpsitemgr" "copyweb" "fproxyproxy" "torrentproxy" "tunnelserver" "tunnelclient")

echo -e "${BLUE}Building tools...${NC}"
for tool in "${tools[@]}"; do
    echo -n "  Building $tool... "
    if go build -o "$tool" "./cmd/$tool"; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        exit 1
    fi
done

echo ""
echo -e "${GREEN}========================================"
echo "  Build Complete!"
echo "========================================${NC}"
echo ""
echo "Built binaries:"
ls -lh fcpctl fcpget fcpput fcpkey fcpsitemgr copyweb fproxyproxy torrentproxy tunnelserver tunnelclient 2>/dev/null || echo "No binaries found"
echo ""
echo "To install system-wide, run:"
echo "  sudo cp fcp* copyweb fproxyproxy torrentproxy tunnel* /usr/local/bin/"
echo ""
echo "Or install to \$GOPATH/bin:"
echo "  go install ./cmd/..."
echo ""
echo "Available tools:"
echo "  fcpctl       - FCP control utility"
echo "  fcpget       - Download files from Hyphanet"
echo "  fcpput       - Upload files to Hyphanet"
echo "  fcpkey       - Key management"
echo "  fcpsitemgr   - Freesite manager"
echo "  copyweb      - Mirror websites for Hyphanet"
echo "  fproxyproxy  - HTTP proxy with name resolution"
echo "  torrentproxy - BitTorrent over Hyphanet"
echo "  tunnelserver - Clearnet server for web tunnel"
echo "  tunnelclient - Client for accessing clearnet via tunnel"
echo ""
echo "Examples:"
echo "  ./fcpctl info"
echo "  ./fcpkey generate mysite"
echo "  ./copyweb https://example.com --mirror -d example --upload --site example"
echo "  ./fproxyproxy --debug"
echo "  ./torrentproxy --debug"
echo "  ./tunnelserver --debug"
echo "  ./tunnelclient -server SSK@.../"
echo ""
