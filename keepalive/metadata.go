// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package keepalive

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
)

// Metadata type constants
const (
	MetadataSimpleRedirect = 0
	MetadataMultiLevel     = 1
	MetadataSplitfile      = 2
	MetadataArchive        = 3
	MetadataSymbolicShort  = 4
)

// Splitfile type constants
const (
	SplitfileOnion     = 0 // Onion FEC (deprecated)
	SplitfileStandard  = 1 // Standard FEC
)

// Compression algorithm constants
const (
	CompressNone  = -1
	CompressGzip  = 0
	CompressBzip2 = 1
	CompressLzma  = 2
	CompressLzmaNew = 3
)

// MetadataParser parses Freenet metadata
type MetadataParser struct {
	data   []byte
	offset int
}

// ParsedMetadata contains parsed metadata information
type ParsedMetadata struct {
	Type          int
	MimeType      string
	CompressAlgo  int
	DataLength    int64
	SplitfileType int

	// For splitfiles
	Segments      []ParsedSegment
	TopBlocks     int
	TopCheckBlocks int
	SegmentSize   int
	CheckSegmentSize int
	DeductBlocks  int

	// For redirects
	TargetURI     string

	// For archives
	ArchiveType   string
}

// ParsedSegment contains information about a splitfile segment
type ParsedSegment struct {
	ID          int
	DataBlocks  []string // CHK URIs for data blocks
	CheckBlocks []string // CHK URIs for check blocks
}

// NewMetadataParser creates a new metadata parser
func NewMetadataParser(data []byte) *MetadataParser {
	return &MetadataParser{
		data:   data,
		offset: 0,
	}
}

// Parse parses the metadata
func (p *MetadataParser) Parse() (*ParsedMetadata, error) {
	if len(p.data) < 2 {
		return nil, fmt.Errorf("metadata too short")
	}

	// Check magic number
	if p.data[0] != 0x00 || p.data[1] != 0x01 {
		return nil, fmt.Errorf("invalid metadata magic: %x %x", p.data[0], p.data[1])
	}
	p.offset = 2

	meta := &ParsedMetadata{
		CompressAlgo: CompressNone,
	}

	// Parse document type
	docType, err := p.readShort()
	if err != nil {
		return nil, err
	}

	// Parse flags based on document type
	switch docType {
	case 0: // Document header
		if err := p.parseDocumentHeader(meta); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported document type: %d", docType)
	}

	return meta, nil
}

// parseDocumentHeader parses the main document header
func (p *MetadataParser) parseDocumentHeader(meta *ParsedMetadata) error {
	// Read flags
	flags, err := p.readShort()
	if err != nil {
		return err
	}

	hasDataLength := (flags & 0x01) != 0
	hasCompression := (flags & 0x02) != 0
	hasMimeType := (flags & 0x04) != 0
	isSplitfile := (flags & 0x08) != 0
	hasHashThisLayer := (flags & 0x10) != 0
	hasHashes := (flags & 0x20) != 0
	hasTopBlocks := (flags & 0x40) != 0

	// Read MIME type
	if hasMimeType {
		mimeType, err := p.readString()
		if err != nil {
			return err
		}
		meta.MimeType = mimeType
	}

	// Read compression info
	if hasCompression {
		compAlgo, err := p.readShort()
		if err != nil {
			return err
		}
		meta.CompressAlgo = int(compAlgo)

		// Read decompressed length
		_, err = p.readLong()
		if err != nil {
			return err
		}
	}

	// Read data length
	if hasDataLength {
		dataLen, err := p.readLong()
		if err != nil {
			return err
		}
		meta.DataLength = dataLen
	}

	// Skip hashes for this layer
	if hasHashThisLayer {
		hashCount, err := p.readShort()
		if err != nil {
			return err
		}
		for i := 0; i < int(hashCount); i++ {
			hashType, err := p.readShort()
			if err != nil {
				return err
			}
			hashLen := getHashLength(int(hashType))
			p.offset += hashLen
		}
	}

	// Skip full hashes
	if hasHashes {
		hashCount, err := p.readShort()
		if err != nil {
			return err
		}
		for i := 0; i < int(hashCount); i++ {
			hashType, err := p.readShort()
			if err != nil {
				return err
			}
			hashLen := getHashLength(int(hashType))
			p.offset += hashLen
		}
	}

	// Read top block info
	if hasTopBlocks {
		topBlocks, err := p.readInt()
		if err != nil {
			return err
		}
		meta.TopBlocks = int(topBlocks)

		topCheckBlocks, err := p.readInt()
		if err != nil {
			return err
		}
		meta.TopCheckBlocks = int(topCheckBlocks)
	}

	// Parse splitfile if present
	if isSplitfile {
		if err := p.parseSplitfile(meta); err != nil {
			return err
		}
	}

	return nil
}

// parseSplitfile parses splitfile metadata
func (p *MetadataParser) parseSplitfile(meta *ParsedMetadata) error {
	meta.Type = MetadataSplitfile

	// Read splitfile type
	sfType, err := p.readShort()
	if err != nil {
		return err
	}
	meta.SplitfileType = int(sfType)

	// Read splitfile params based on type
	switch meta.SplitfileType {
	case SplitfileStandard:
		// Read segment info
		segmentSize, err := p.readInt()
		if err != nil {
			return err
		}
		meta.SegmentSize = int(segmentSize)

		checkSegSize, err := p.readInt()
		if err != nil {
			return err
		}
		meta.CheckSegmentSize = int(checkSegSize)

		// Read segments
		segmentCount, err := p.readInt()
		if err != nil {
			return err
		}

		// Read deduct blocks for cross-segment
		deductBlocks, err := p.readInt()
		if err != nil {
			return err
		}
		meta.DeductBlocks = int(deductBlocks)

		// Parse each segment
		for i := 0; i < int(segmentCount); i++ {
			seg, err := p.parseSegment(i)
			if err != nil {
				return err
			}
			meta.Segments = append(meta.Segments, *seg)
		}

	case SplitfileOnion:
		return fmt.Errorf("onion FEC is deprecated and not supported")

	default:
		return fmt.Errorf("unknown splitfile type: %d", meta.SplitfileType)
	}

	return nil
}

// parseSegment parses a single segment
func (p *MetadataParser) parseSegment(id int) (*ParsedSegment, error) {
	seg := &ParsedSegment{
		ID: id,
	}

	// Read data block count
	dataCount, err := p.readInt()
	if err != nil {
		return nil, err
	}

	// Read check block count
	checkCount, err := p.readInt()
	if err != nil {
		return nil, err
	}

	// Read data block keys
	for i := 0; i < int(dataCount); i++ {
		key, err := p.readCHKKey()
		if err != nil {
			return nil, err
		}
		seg.DataBlocks = append(seg.DataBlocks, key)
	}

	// Read check block keys
	for i := 0; i < int(checkCount); i++ {
		key, err := p.readCHKKey()
		if err != nil {
			return nil, err
		}
		seg.CheckBlocks = append(seg.CheckBlocks, key)
	}

	return seg, nil
}

// readCHKKey reads a CHK key from the metadata
func (p *MetadataParser) readCHKKey() (string, error) {
	// CHK key structure:
	// - 32 bytes: routing key
	// - 32 bytes: crypto key
	// - 5 bytes: extra (crypto algorithm, control document, compression)

	if p.offset+69 > len(p.data) {
		return "", fmt.Errorf("not enough data for CHK key")
	}

	routingKey := p.data[p.offset : p.offset+32]
	p.offset += 32

	cryptoKey := p.data[p.offset : p.offset+32]
	p.offset += 32

	extra := p.data[p.offset : p.offset+5]
	p.offset += 5

	// Build CHK URI
	uri := fmt.Sprintf("CHK@%s,%s,%s",
		base64FreenetEncode(routingKey),
		base64FreenetEncode(cryptoKey),
		encodeExtra(extra),
	)

	return uri, nil
}

// Helper functions for reading binary data

func (p *MetadataParser) readByte() (byte, error) {
	if p.offset >= len(p.data) {
		return 0, fmt.Errorf("unexpected end of data")
	}
	b := p.data[p.offset]
	p.offset++
	return b, nil
}

func (p *MetadataParser) readShort() (int16, error) {
	if p.offset+2 > len(p.data) {
		return 0, fmt.Errorf("unexpected end of data")
	}
	val := binary.BigEndian.Uint16(p.data[p.offset:])
	p.offset += 2
	return int16(val), nil
}

func (p *MetadataParser) readInt() (int32, error) {
	if p.offset+4 > len(p.data) {
		return 0, fmt.Errorf("unexpected end of data")
	}
	val := binary.BigEndian.Uint32(p.data[p.offset:])
	p.offset += 4
	return int32(val), nil
}

func (p *MetadataParser) readLong() (int64, error) {
	if p.offset+8 > len(p.data) {
		return 0, fmt.Errorf("unexpected end of data")
	}
	val := binary.BigEndian.Uint64(p.data[p.offset:])
	p.offset += 8
	return int64(val), nil
}

func (p *MetadataParser) readString() (string, error) {
	// String is length-prefixed (2 bytes)
	length, err := p.readShort()
	if err != nil {
		return "", err
	}
	if length < 0 {
		return "", fmt.Errorf("negative string length")
	}
	if p.offset+int(length) > len(p.data) {
		return "", fmt.Errorf("string extends past end of data")
	}
	s := string(p.data[p.offset : p.offset+int(length)])
	p.offset += int(length)
	return s, nil
}

// getHashLength returns the byte length for a hash type
func getHashLength(hashType int) int {
	switch hashType {
	case 0: // SHA-256
		return 32
	case 1: // SHA-384
		return 48
	case 2: // SHA-512
		return 64
	default:
		return 32 // Default to SHA-256 length
	}
}

// base64FreenetEncode encodes bytes to Freenet's base64 variant
func base64FreenetEncode(data []byte) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789~-"

	var result bytes.Buffer

	for i := 0; i < len(data); i += 3 {
		var block uint32
		remaining := len(data) - i

		switch remaining {
		case 1:
			block = uint32(data[i]) << 16
		case 2:
			block = uint32(data[i])<<16 | uint32(data[i+1])<<8
		default:
			block = uint32(data[i])<<16 | uint32(data[i+1])<<8 | uint32(data[i+2])
		}

		result.WriteByte(alphabet[(block>>18)&0x3F])
		result.WriteByte(alphabet[(block>>12)&0x3F])

		if remaining > 1 {
			result.WriteByte(alphabet[(block>>6)&0x3F])
		}
		if remaining > 2 {
			result.WriteByte(alphabet[block&0x3F])
		}
	}

	return result.String()
}

// encodeExtra encodes the CHK extra bytes
func encodeExtra(extra []byte) string {
	return base64FreenetEncode(extra)
}

// ExtractBlocksFromSite extracts all block URIs from a site
func ExtractBlocksFromSite(site *Site, meta *ParsedMetadata) {
	for _, parsedSeg := range meta.Segments {
		segment := NewSegment(parsedSeg.ID)

		// Add data blocks
		for i, uri := range parsedSeg.DataBlocks {
			block := NewBlock(uri, parsedSeg.ID, i, true)
			segment.AddBlock(block)
		}

		// Add check blocks
		for i, uri := range parsedSeg.CheckBlocks {
			block := NewBlock(uri, parsedSeg.ID, len(parsedSeg.DataBlocks)+i, false)
			segment.AddBlock(block)
		}

		site.AddSegment(segment)
	}

	site.BlockCount = 0
	for _, seg := range site.Segments {
		site.BlockCount += seg.Size()
	}
}

// ParseSimpleManifest parses a simple manifest format (line-based)
func ParseSimpleManifest(data []byte) ([]string, error) {
	var keys []string

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Check if it's a CHK/SSK/USK key
		if strings.HasPrefix(line, "CHK@") ||
			strings.HasPrefix(line, "SSK@") ||
			strings.HasPrefix(line, "USK@") {
			keys = append(keys, line)
		}
	}

	return keys, nil
}
