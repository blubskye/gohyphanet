package keys

import (
	"bytes"
	"crypto/dsa"
	"fmt"
)

const (
	ClientSSKCryptoKeyLength = 32
	ClientSSKExtraLength     = 5
)

// ClientSSK represents a client-level Signed Subspace Key
// Contains document name, public key, and decryption information
type ClientSSK struct {
	cryptoAlgorithm byte            // encryption algorithm
	docName         string          // document name
	pubKey          *dsa.PublicKey  // can be nil initially
	pubKeyHash      []byte          // 32 bytes
	cryptoKey       []byte          // 32 bytes - decryption key
	ehDocname       []byte          // 32 bytes - E_cryptoKey(H(docname))
	hashCodeValue   int
	cachedNodeKey   *NodeSSK
}

// NewClientSSK creates a new ClientSSK
func NewClientSSK(docName string, pubKeyHash, extra []byte, pubKey *dsa.PublicKey, cryptoKey []byte) (*ClientSSK, error) {
	if docName == "" {
		return nil, fmt.Errorf("document name cannot be empty")
	}
	if len(pubKeyHash) != NodeSSKPubKeyHashSize {
		return nil, fmt.Errorf("pubKeyHash must be %d bytes, got %d", NodeSSKPubKeyHashSize, len(pubKeyHash))
	}
	if len(cryptoKey) != ClientSSKCryptoKeyLength {
		return nil, fmt.Errorf("cryptoKey must be %d bytes, got %d", ClientSSKCryptoKeyLength, len(cryptoKey))
	}
	if len(extra) < ClientSSKExtraLength {
		return nil, fmt.Errorf("extra bytes must be at least %d bytes, got %d", ClientSSKExtraLength, len(extra))
	}

	cryptoAlgorithm := extra[2]
	if cryptoAlgorithm != AlgoAESPCFB256SHA256 && cryptoAlgorithm != AlgoAESCTR256SHA256 {
		return nil, fmt.Errorf("invalid crypto algorithm: %d", cryptoAlgorithm)
	}

	// Verify pubKey matches hash if provided
	if pubKey != nil {
		computedHash := hashPublicKey(pubKey)
		if !bytes.Equal(computedHash, pubKeyHash) {
			return nil, fmt.Errorf("pubKey hash mismatch")
		}
	}

	// Make copies to ensure immutability
	pubKeyHashCopy := make([]byte, NodeSSKPubKeyHashSize)
	copy(pubKeyHashCopy, pubKeyHash)
	cryptoKeyCopy := make([]byte, ClientSSKCryptoKeyLength)
	copy(cryptoKeyCopy, cryptoKey)

	// Calculate E(H(docname))
	hashedDocname := HashDocname(docName)
	ehDocname, err := EncryptHashedDocname(hashedDocname, cryptoKeyCopy)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt hashed docname: %w", err)
	}

	return &ClientSSK{
		cryptoAlgorithm: cryptoAlgorithm,
		docName:         docName,
		pubKey:          pubKey,
		pubKeyHash:      pubKeyHashCopy,
		cryptoKey:       cryptoKeyCopy,
		ehDocname:       ehDocname,
		hashCodeValue:   calculateHash(pubKeyHashCopy, cryptoKeyCopy, ehDocname, []byte(docName)),
	}, nil
}

// NewClientSSKFromURI creates a ClientSSK from a FreenetURI
func NewClientSSKFromURI(uri *FreenetURI) (*ClientSSK, error) {
	if uri.KeyType != "SSK" && uri.KeyType != "USK" {
		return nil, fmt.Errorf("not an SSK/USK URI")
	}

	return NewClientSSK(
		uri.DocName,
		uri.RoutingKey, // For SSK, routingKey field contains pubKeyHash
		uri.Extra,
		nil, // pubKey not provided in URI
		uri.CryptoKey,
	)
}

// GetSSKExtraBytes creates the extra bytes for an SSK
func GetSSKExtraBytes(cryptoAlgorithm byte) []byte {
	extra := make([]byte, ClientSSKExtraLength)
	extra[0] = NodeSSKVersion
	extra[1] = 0 // 0 = fetch (public), 1 = insert (private)
	extra[2] = cryptoAlgorithm
	extra[3] = byte(HashSHA256 >> 8)
	extra[4] = byte(HashSHA256 & 0xFF)
	return extra
}

// GetURI converts this ClientSSK to a FreenetURI
func (c *ClientSSK) GetURI() *FreenetURI {
	return &FreenetURI{
		KeyType:    "SSK",
		DocName:    c.docName,
		RoutingKey: c.pubKeyHash, // Note: stored as routingKey in URI
		CryptoKey:  c.cryptoKey,
		Extra:      GetSSKExtraBytes(c.cryptoAlgorithm),
	}
}

// GetNodeKey returns the corresponding NodeSSK (for routing)
func (c *ClientSSK) GetNodeKey(clone bool) (*NodeSSK, error) {
	if c.cachedNodeKey == nil {
		var err error
		c.cachedNodeKey, err = NewNodeSSK(
			c.pubKeyHash,
			c.ehDocname,
			c.pubKey,
			c.cryptoAlgorithm,
		)
		if err != nil {
			return nil, err
		}
	}

	if clone {
		return c.cachedNodeKey.Clone().(*NodeSSK), nil
	}
	return c.cachedNodeKey, nil
}

// GetDocName returns the document name
func (c *ClientSSK) GetDocName() string {
	return c.docName
}

// GetPubKeyHash returns the public key hash
func (c *ClientSSK) GetPubKeyHash() []byte {
	return c.pubKeyHash
}

// GetCryptoKey returns the decryption key
func (c *ClientSSK) GetCryptoKey() []byte {
	return c.cryptoKey
}

// GetCryptoAlgorithm returns the crypto algorithm
func (c *ClientSSK) GetCryptoAlgorithm() byte {
	return c.cryptoAlgorithm
}

// GetEncryptedHashedDocname returns E(H(docname))
func (c *ClientSSK) GetEncryptedHashedDocname() []byte {
	return c.ehDocname
}

// GetPubKey returns the public key if available
func (c *ClientSSK) GetPubKey() *dsa.PublicKey {
	return c.pubKey
}

// SetPubKey sets the public key if it matches the hash
func (c *ClientSSK) SetPubKey(pubKey *dsa.PublicKey) error {
	if c.pubKey != nil {
		// Already set, verify it's the same
		if c.pubKey.Y.Cmp(pubKey.Y) == 0 {
			return nil
		}
		return fmt.Errorf("pubKey already set to different value")
	}

	// Verify hash matches
	computedHash := hashPublicKey(pubKey)
	if !bytes.Equal(computedHash, c.pubKeyHash) {
		return fmt.Errorf("pubkey hash mismatch")
	}

	c.pubKey = pubKey

	// Update cached node key if it exists
	if c.cachedNodeKey != nil {
		c.cachedNodeKey.SetPubKey(pubKey)
	}

	return nil
}

// GetRoutingKey returns the routing key (for compatibility)
func (c *ClientSSK) GetRoutingKey() []byte {
	if c.cachedNodeKey != nil {
		return c.cachedNodeKey.GetRoutingKey()
	}
	// Calculate it on the fly
	return makeSSKRoutingKey(c.pubKeyHash, c.ehDocname)
}

// Equals checks if two ClientSSKs are equal
func (c *ClientSSK) Equals(other *ClientSSK) bool {
	if other == nil {
		return false
	}
	return c.cryptoAlgorithm == other.cryptoAlgorithm &&
		c.docName == other.docName &&
		bytes.Equal(c.pubKeyHash, other.pubKeyHash) &&
		bytes.Equal(c.cryptoKey, other.cryptoKey) &&
		bytes.Equal(c.ehDocname, other.ehDocname)
}

// HashCode returns a hash code for this key
func (c *ClientSSK) HashCode() int {
	return c.hashCodeValue
}

// Clone creates a deep copy of this ClientSSK
func (c *ClientSSK) Clone() *ClientSSK {
	pubKeyHashCopy := make([]byte, len(c.pubKeyHash))
	copy(pubKeyHashCopy, c.pubKeyHash)
	cryptoKeyCopy := make([]byte, len(c.cryptoKey))
	copy(cryptoKeyCopy, c.cryptoKey)
	ehDocnameCopy := make([]byte, len(c.ehDocname))
	copy(ehDocnameCopy, c.ehDocname)

	clone := &ClientSSK{
		cryptoAlgorithm: c.cryptoAlgorithm,
		docName:         c.docName,
		pubKey:          c.pubKey, // DSA keys are immutable
		pubKeyHash:      pubKeyHashCopy,
		cryptoKey:       cryptoKeyCopy,
		ehDocname:       ehDocnameCopy,
		hashCodeValue:   c.hashCodeValue,
	}

	// Don't copy cached node key - let it be recreated if needed
	return clone
}

// ToNormalizedDouble converts key to 0.0-1.0 range for routing
func (c *ClientSSK) ToNormalizedDouble() float64 {
	nodeKey, err := c.GetNodeKey(false)
	if err != nil {
		return 0.0
	}
	return nodeKey.ToNormalizedDouble()
}

// VerifyDocname verifies that the docname matches the encrypted hashed docname
func (c *ClientSSK) VerifyDocname() error {
	hashedDocname := HashDocname(c.docName)
	expectedEH, err := EncryptHashedDocname(hashedDocname, c.cryptoKey)
	if err != nil {
		return err
	}

	if !bytes.Equal(expectedEH, c.ehDocname) {
		return fmt.Errorf("encrypted hashed docname mismatch")
	}

	return nil
}
