package keys

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
)

// FreenetURI represents a Freenet/Hyphanet URI
// Format: freenet:[KeyType@]RoutingKey,CryptoKey,Extra[/docname][/meta1/meta2/...]
type FreenetURI struct {
	KeyType          string   // "CHK", "SSK", "KSK", "USK"
	DocName          string   // document name (SSK, KSK, USK)
	MetaStr          []string // manifest path components
	RoutingKey       []byte   // 32 bytes (CHK, SSK, USK) - for SSK it's pubKeyHash
	CryptoKey        []byte   // 32 bytes (CHK, SSK, USK)
	Extra            []byte   // 5 bytes - algorithm info
	SuggestedEdition int64    // only for USK (-1 = not specified)
	hashCodeValue    int
	hasHashCode      bool
}

// Valid key types
var ValidKeyTypes = []string{"CHK", "SSK", "KSK", "USK"}

// ParseFreenetURI parses a Freenet URI string
func ParseFreenetURI(uriStr string) (*FreenetURI, error) {
	// Strip common prefixes
	uriStr = strings.TrimPrefix(uriStr, "freenet:")
	uriStr = strings.TrimPrefix(uriStr, "http://")
	uriStr = strings.TrimPrefix(uriStr, "https://")
	uriStr = strings.TrimPrefix(uriStr, "//")

	// Find @ separator
	atIdx := strings.Index(uriStr, "@")
	if atIdx == -1 {
		return nil, fmt.Errorf("no @ separator in URI")
	}

	keyType := strings.ToUpper(uriStr[:atIdx])
	remainder := uriStr[atIdx+1:]

	// Validate key type
	if !isValidKeyType(keyType) {
		return nil, fmt.Errorf("invalid key type: %s", keyType)
	}

	// Parse meta strings (work backwards from slashes)
	var metaStr []string
	for {
		slashIdx := strings.LastIndex(remainder, "/")
		if slashIdx == -1 {
			break
		}
		meta, err := url.PathUnescape(remainder[slashIdx+1:])
		if err != nil {
			return nil, fmt.Errorf("invalid meta string encoding: %w", err)
		}
		metaStr = append([]string{meta}, metaStr...)
		remainder = remainder[:slashIdx]
	}

	// Extract docname for SSK/USK/KSK
	var docName string
	var suggestedEdition int64 = -1

	if keyType == "KSK" {
		if len(metaStr) == 0 {
			return nil, fmt.Errorf("no docname for KSK")
		}
		docName = metaStr[0]
		metaStr = metaStr[1:]
		return &FreenetURI{
			KeyType:          keyType,
			DocName:          docName,
			MetaStr:          metaStr,
			SuggestedEdition: -1,
		}, nil
	}

	if keyType == "SSK" || keyType == "USK" {
		if len(metaStr) > 0 {
			docName = metaStr[0]
			metaStr = metaStr[1:]

			if keyType == "USK" {
				if len(metaStr) == 0 {
					return nil, fmt.Errorf("no edition for USK")
				}
				edition, err := strconv.ParseInt(metaStr[0], 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid edition: %w", err)
				}
				suggestedEdition = edition
				metaStr = metaStr[1:]
			}
		}
	}

	// For CHK: strip file extensions from the key part
	if keyType == "CHK" {
		if dotIdx := strings.LastIndex(remainder, "."); dotIdx != -1 {
			// Check if it looks like a file extension (no / after the dot)
			if !strings.Contains(remainder[dotIdx:], "/") {
				remainder = remainder[:dotIdx]
			}
		}
	}

	// Parse routing key, crypto key, extra
	parts := strings.Split(remainder, ",")

	var routingKey, cryptoKey, extra []byte
	var err error

	if len(parts) > 0 && parts[0] != "" {
		routingKey, err = base64.StdEncoding.DecodeString(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid routing key encoding: %w", err)
		}
		if keyType == "CHK" || keyType == "SSK" || keyType == "USK" {
			if len(routingKey) != 32 {
				return nil, fmt.Errorf("routing key must be 32 bytes, got %d", len(routingKey))
			}
		}
	}

	if len(parts) > 1 && parts[1] != "" {
		cryptoKey, err = base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid crypto key encoding: %w", err)
		}
		if len(cryptoKey) != 32 {
			return nil, fmt.Errorf("crypto key must be 32 bytes, got %d", len(cryptoKey))
		}
	}

	if len(parts) > 2 && parts[2] != "" {
		extra, err = base64.StdEncoding.DecodeString(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid extra bytes encoding: %w", err)
		}
	}

	return &FreenetURI{
		KeyType:          keyType,
		DocName:          docName,
		MetaStr:          metaStr,
		RoutingKey:       routingKey,
		CryptoKey:        cryptoKey,
		Extra:            extra,
		SuggestedEdition: suggestedEdition,
	}, nil
}

// String converts the URI to a string representation
func (uri *FreenetURI) String() string {
	var b strings.Builder

	b.WriteString(uri.KeyType)
	b.WriteString("@")

	if uri.KeyType != "KSK" {
		if uri.RoutingKey != nil {
			b.WriteString(base64.StdEncoding.EncodeToString(uri.RoutingKey))
		}
		if uri.CryptoKey != nil {
			b.WriteString(",")
			b.WriteString(base64.StdEncoding.EncodeToString(uri.CryptoKey))
		}
		if uri.Extra != nil {
			b.WriteString(",")
			b.WriteString(base64.StdEncoding.EncodeToString(uri.Extra))
		}
		if uri.DocName != "" {
			b.WriteString("/")
		}
	}

	if uri.DocName != "" {
		b.WriteString(url.PathEscape(uri.DocName))
	}

	if uri.KeyType == "USK" && uri.SuggestedEdition >= 0 {
		b.WriteString("/")
		b.WriteString(strconv.FormatInt(uri.SuggestedEdition, 10))
	}

	for _, meta := range uri.MetaStr {
		b.WriteString("/")
		b.WriteString(url.PathEscape(meta))
	}

	return b.String()
}

// ToClientCHK converts this URI to a ClientCHK (if it's a CHK URI)
func (uri *FreenetURI) ToClientCHK() (*ClientCHK, error) {
	if uri.KeyType != "CHK" {
		return nil, fmt.Errorf("not a CHK URI")
	}
	return NewClientCHKFromURI(uri)
}

// ToClientSSK converts this URI to a ClientSSK (if it's an SSK/USK URI)
func (uri *FreenetURI) ToClientSSK() (*ClientSSK, error) {
	if uri.KeyType != "SSK" && uri.KeyType != "USK" {
		return nil, fmt.Errorf("not an SSK/USK URI")
	}
	return NewClientSSKFromURI(uri)
}

// WriteBinary writes the URI in binary format
func (uri *FreenetURI) WriteBinary(w io.Writer) error {
	// Write key type as byte
	var keyTypeByte byte
	switch uri.KeyType {
	case "CHK":
		keyTypeByte = 1
	case "SSK":
		keyTypeByte = 2
	case "KSK":
		keyTypeByte = 3
	case "USK":
		return fmt.Errorf("cannot write USKs as binary keys")
	default:
		return fmt.Errorf("unknown key type: %s", uri.KeyType)
	}

	if err := binary.Write(w, binary.BigEndian, keyTypeByte); err != nil {
		return err
	}

	// Write keys for CHK/SSK
	if uri.KeyType != "KSK" {
		if len(uri.RoutingKey) != 32 {
			return fmt.Errorf("routing key must be 32 bytes")
		}
		if _, err := w.Write(uri.RoutingKey); err != nil {
			return err
		}

		if len(uri.CryptoKey) != 32 {
			return fmt.Errorf("crypto key must be 32 bytes")
		}
		if _, err := w.Write(uri.CryptoKey); err != nil {
			return err
		}

		if _, err := w.Write(uri.Extra); err != nil {
			return err
		}
	}

	// Write docname for non-CHK
	if uri.KeyType != "CHK" {
		if err := writeUTF(w, uri.DocName); err != nil {
			return err
		}
	}

	// Write meta strings
	if err := binary.Write(w, binary.BigEndian, int32(len(uri.MetaStr))); err != nil {
		return err
	}
	for _, meta := range uri.MetaStr {
		if err := writeUTF(w, meta); err != nil {
			return err
		}
	}

	return nil
}

// Clone creates a deep copy of the URI
func (uri *FreenetURI) Clone() *FreenetURI {
	clone := &FreenetURI{
		KeyType:          uri.KeyType,
		DocName:          uri.DocName,
		SuggestedEdition: uri.SuggestedEdition,
		hashCodeValue:    uri.hashCodeValue,
		hasHashCode:      uri.hasHashCode,
	}

	if uri.RoutingKey != nil {
		clone.RoutingKey = make([]byte, len(uri.RoutingKey))
		copy(clone.RoutingKey, uri.RoutingKey)
	}

	if uri.CryptoKey != nil {
		clone.CryptoKey = make([]byte, len(uri.CryptoKey))
		copy(clone.CryptoKey, uri.CryptoKey)
	}

	if uri.Extra != nil {
		clone.Extra = make([]byte, len(uri.Extra))
		copy(clone.Extra, uri.Extra)
	}

	if uri.MetaStr != nil {
		clone.MetaStr = make([]string, len(uri.MetaStr))
		copy(clone.MetaStr, uri.MetaStr)
	}

	return clone
}

// Equals checks if two URIs are equal
func (uri *FreenetURI) Equals(other *FreenetURI) bool {
	if other == nil {
		return false
	}
	if uri.KeyType != other.KeyType ||
		uri.DocName != other.DocName ||
		uri.SuggestedEdition != other.SuggestedEdition {
		return false
	}
	if !bytes.Equal(uri.RoutingKey, other.RoutingKey) ||
		!bytes.Equal(uri.CryptoKey, other.CryptoKey) ||
		!bytes.Equal(uri.Extra, other.Extra) {
		return false
	}
	if len(uri.MetaStr) != len(other.MetaStr) {
		return false
	}
	for i := range uri.MetaStr {
		if uri.MetaStr[i] != other.MetaStr[i] {
			return false
		}
	}
	return true
}

// GetSiteName returns the site name portion of the URI
func (uri *FreenetURI) GetSiteName() string {
	if uri.KeyType == "USK" || uri.KeyType == "SSK" {
		return uri.DocName
	}
	return ""
}

// GetEdition returns the edition for USK URIs
func (uri *FreenetURI) GetEdition() int64 {
	return uri.SuggestedEdition
}

// SetEdition sets the edition for USK URIs
func (uri *FreenetURI) SetEdition(edition int64) error {
	if uri.KeyType != "USK" {
		return fmt.Errorf("cannot set edition on non-USK URI")
	}
	uri.SuggestedEdition = edition
	uri.hasHashCode = false // Invalidate cached hash
	return nil
}

// Helper functions

func isValidKeyType(keyType string) bool {
	for _, valid := range ValidKeyTypes {
		if keyType == valid {
			return true
		}
	}
	return false
}

func writeUTF(w io.Writer, s string) error {
	bytes := []byte(s)
	if err := binary.Write(w, binary.BigEndian, uint16(len(bytes))); err != nil {
		return err
	}
	_, err := w.Write(bytes)
	return err
}

func readUTF(r io.Reader) (string, error) {
	var length uint16
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return "", err
	}
	bytes := make([]byte, length)
	if _, err := io.ReadFull(r, bytes); err != nil {
		return "", err
	}
	return string(bytes), nil
}
