.PHONY: all build clean install test help

# Build all binaries
all: build

# Build all tools
build:
	@echo "Building GoHyphanet tools..."
	go build -o fcpctl ./cmd/fcpctl
	go build -o fcpget ./cmd/fcpget
	go build -o fcpput ./cmd/fcpput
	go build -o fcpkey ./cmd/fcpkey
	go build -o fcpsitemgr ./cmd/fcpsitemgr
	go build -o copyweb ./cmd/copyweb
	go build -o fproxyproxy ./cmd/fproxyproxy
	@echo "Build complete!"

# Clean build artifacts
clean:
	@echo "Cleaning..."
	rm -f fcpctl fcpget fcpput fcpkey fcpsitemgr copyweb fproxyproxy
	go clean
	@echo "Clean complete!"

# Install to $GOPATH/bin
install:
	@echo "Installing to GOPATH/bin..."
	go install ./cmd/fcpctl
	go install ./cmd/fcpget
	go install ./cmd/fcpput
	go install ./cmd/fcpkey
	go install ./cmd/fcpsitemgr
	go install ./cmd/copyweb
	go install ./cmd/fproxyproxy
	@echo "Install complete!"

# Run tests
test:
	go test ./...

# Download dependencies
deps:
	go mod download
	go mod tidy

# Show help
help:
	@echo "GoHyphanet Build System"
	@echo ""
	@echo "Targets:"
	@echo "  make          - Build all tools"
	@echo "  make build    - Build all tools"
	@echo "  make clean    - Remove built binaries"
	@echo "  make install  - Install to GOPATH/bin"
	@echo "  make test     - Run tests"
	@echo "  make deps     - Download dependencies"
	@echo "  make help     - Show this help"
	@echo ""
	@echo "Tools:"
	@echo "  fcpctl      - FCP control utility"
	@echo "  fcpget      - Download from Hyphanet"
	@echo "  fcpput      - Upload to Hyphanet"
	@echo "  fcpkey      - Key management"
	@echo "  fcpsitemgr  - Freesite manager"
	@echo "  copyweb     - Mirror websites"
	@echo "  fproxyproxy - HTTP proxy with name resolution"
