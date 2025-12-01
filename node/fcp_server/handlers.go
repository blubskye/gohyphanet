package fcp_server

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/blubskye/gohyphanet/node/keys"
	"github.com/blubskye/gohyphanet/node/requests"
	"github.com/blubskye/gohyphanet/node/store"
)

// handleClientGet processes a ClientGet request
func (c *FCPConnection) handleClientGet(msg *FCPMessage) error {
	// Extract required fields
	identifier, ok := msg.Fields["Identifier"]
	if !ok {
		return c.SendProtocolError(
			ProtocolErrorMissingField,
			"ClientGet must contain Identifier",
			"",
			false,
		)
	}

	uriStr, ok := msg.Fields["URI"]
	if !ok {
		return c.SendProtocolError(
			ProtocolErrorMissingField,
			"ClientGet must contain URI",
			identifier,
			false,
		)
	}

	global := msg.Fields["Global"] == "true"

	// Parse URI
	uri, err := keys.ParseFreenetURI(uriStr)
	if err != nil {
		return c.SendProtocolError(
			ProtocolErrorFreenetURIParseError,
			fmt.Sprintf("Invalid URI: %v", err),
			identifier,
			global,
		)
	}

	// Check for identifier collision
	c.mu.Lock()
	if _, exists := c.activeRequests[identifier]; exists {
		c.mu.Unlock()
		return c.SendProtocolError(
			ProtocolErrorIdentifierCollision,
			"Request with this identifier already exists",
			identifier,
			global,
		)
	}

	// Create request tracking
	req := &FCPRequest{
		Identifier: identifier,
		URI:        uriStr,
		Status:     RequestStatusPending,
		StartTime:  time.Now().Unix(),
		Priority:   2, // Default priority
		Global:     global,
		cancel:     make(chan struct{}),
	}
	c.activeRequests[identifier] = req
	c.mu.Unlock()

	log.Printf("[%s] ClientGet: %s (identifier: %s)", c.id, uriStr, identifier)

	// Extract the key from the URI
	var key keys.Key
	var err2 error
	switch uri.KeyType {
	case "CHK":
		clientCHK, err2 := uri.ToClientCHK()
		if err2 == nil && clientCHK != nil {
			key = clientCHK.GetNodeCHK()
		}
	case "SSK", "KSK":
		clientSSK, err2 := uri.ToClientSSK()
		if err2 == nil && clientSSK != nil {
			nodeSSK, _ := clientSSK.GetNodeKey(false)
			key = nodeSSK
		}
	default:
		c.removeRequest(identifier)
		return c.SendGetFailed(identifier, 27, "Unsupported key type: "+uri.KeyType)
	}

	if key == nil {
		c.removeRequest(identifier)
		msg := "Could not extract key from URI"
		if err2 != nil {
			msg = fmt.Sprintf("%s: %v", msg, err2)
		}
		return c.SendGetFailed(identifier, 27, msg)
	}

	// Try to fetch from local datastore first
	go c.performGet(identifier, key, req)

	return nil
}

// performGet executes the actual get operation
func (c *FCPConnection) performGet(identifier string, key keys.Key, req *FCPRequest) {
	// Update status
	c.mu.Lock()
	if r, exists := c.activeRequests[identifier]; exists {
		r.Status = RequestStatusRunning
	}
	c.mu.Unlock()

	// Check local datastore
	meta := store.NewBlockMetadata()
	block, err := c.server.datastore.Fetch(
		key.GetRoutingKey(),
		key.GetFullKey(),
		false, // dontPromote
		true,  // canReadClientCache
		true,  // canReadSlashdotCache
		false, // ignoreOldBlocks
		meta,
	)

	if err != nil || block == nil {
		// Not found locally
		c.removeRequest(identifier)
		c.SendGetFailed(identifier, 13, "Data not found in local store")
		return
	}

	// Verify it's a KeyBlock
	keyBlock, ok := block.(store.KeyBlock)
	if !ok {
		c.removeRequest(identifier)
		c.SendGetFailed(identifier, 27, "Invalid block type")
		return
	}

	// Send success response with data
	log.Printf("[%s] ClientGet found data locally: %s", c.id, identifier)

	// Send DataFound message
	c.SendMessage(&FCPMessage{
		Name: "DataFound",
		Fields: map[string]string{
			"Identifier": identifier,
			"Global":     strconv.FormatBool(req.Global),
			"DataLength": strconv.Itoa(len(keyBlock.GetRawData())),
			"Metadata.ContentType": "application/octet-stream",
		},
	})

	// Send AllData message with the actual data
	c.SendMessage(&FCPMessage{
		Name: "AllData",
		Fields: map[string]string{
			"Identifier": identifier,
			"Global":     strconv.FormatBool(req.Global),
			"StartupTime": strconv.FormatInt(time.Now().Unix(), 10),
			"CompletionTime": strconv.FormatInt(time.Now().Unix(), 10),
		},
		Data: keyBlock.GetRawData(),
	})

	c.removeRequest(identifier)
}

// handleClientPut processes a ClientPut request
func (c *FCPConnection) handleClientPut(msg *FCPMessage) error {
	// Extract required fields
	identifier, ok := msg.Fields["Identifier"]
	if !ok {
		return c.SendProtocolError(
			ProtocolErrorMissingField,
			"ClientPut must contain Identifier",
			"",
			false,
		)
	}

	uriStr, ok := msg.Fields["URI"]
	if !ok {
		return c.SendProtocolError(
			ProtocolErrorMissingField,
			"ClientPut must contain URI",
			identifier,
			false,
		)
	}

	global := msg.Fields["Global"] == "true"

	// Parse URI
	uri, err := keys.ParseFreenetURI(uriStr)
	if err != nil {
		return c.SendProtocolError(
			ProtocolErrorFreenetURIParseError,
			fmt.Sprintf("Invalid URI: %v", err),
			identifier,
			global,
		)
	}

	// Check for identifier collision
	c.mu.Lock()
	if _, exists := c.activeRequests[identifier]; exists {
		c.mu.Unlock()
		return c.SendProtocolError(
			ProtocolErrorIdentifierCollision,
			"Request with this identifier already exists",
			identifier,
			global,
		)
	}

	// Create request tracking
	req := &FCPRequest{
		Identifier: identifier,
		URI:        uriStr,
		Status:     RequestStatusPending,
		StartTime:  time.Now().Unix(),
		Priority:   2,
		Global:     global,
		cancel:     make(chan struct{}),
	}
	c.activeRequests[identifier] = req
	c.mu.Unlock()

	log.Printf("[%s] ClientPut: %s (identifier: %s)", c.id, uriStr, identifier)

	// For CHK, we can store the data directly
	// For SSK, we need the private key
	if uri.KeyType == "CHK" {
		// Store data in datastore
		if len(msg.Data) > 0 {
			go c.performPut(identifier, uri, msg.Data, req)
		} else {
			c.removeRequest(identifier)
			return c.SendPutFailed(identifier, 27, "No data provided")
		}
	} else {
		c.removeRequest(identifier)
		return c.SendPutFailed(identifier, 27, "Only CHK inserts supported currently")
	}

	return nil
}

// performPut executes the actual put operation
func (c *FCPConnection) performPut(identifier string, uri *keys.FreenetURI, data []byte, req *FCPRequest) {
	// Update status
	c.mu.Lock()
	if r, exists := c.activeRequests[identifier]; exists {
		r.Status = RequestStatusRunning
	}
	c.mu.Unlock()

	// Pad data to CHK block size if needed
	if len(data) < store.CHKDataLength {
		padded := make([]byte, store.CHKDataLength)
		copy(padded, data)
		data = padded
	} else if len(data) > store.CHKDataLength {
		c.removeRequest(identifier)
		c.SendPutFailed(identifier, 9, "Data too large for single CHK block")
		return
	}

	// Create headers (simplified - just hash identifier)
	headers := make([]byte, store.CHKTotalHeadersLength)
	headers[0] = 0x00 // Hash identifier high byte
	headers[1] = 0x01 // Hash identifier low byte (SHA-256)

	// Create client CHK from data
	clientCHK, err := keys.NewClientCHKFromData(data)
	if err != nil {
		c.removeRequest(identifier)
		c.SendPutFailed(identifier, 9, fmt.Sprintf("Failed to create CHK: %v", err))
		return
	}

	// Create CHK block
	chkBlock, err := store.NewCHKBlock(data, headers, clientCHK.GetNodeCHK(), false)
	if err != nil {
		c.removeRequest(identifier)
		c.SendPutFailed(identifier, 9, fmt.Sprintf("Failed to create CHK block: %v", err))
		return
	}

	// Store the client key for later retrieval
	chkBlock.SetClientKey(clientCHK)

	// Store in datastore
	err = c.server.datastore.Put(chkBlock, data, headers, false, false)
	if err != nil {
		c.removeRequest(identifier)
		c.SendPutFailed(identifier, 9, fmt.Sprintf("Failed to store block: %v", err))
		return
	}

	// Generate the final CHK URI
	finalURI := chkBlock.GetClientKey().GetURI().String()

	log.Printf("[%s] ClientPut successful: %s -> %s", c.id, identifier, finalURI)

	// Send PutSuccessful message
	c.SendMessage(&FCPMessage{
		Name: "PutSuccessful",
		Fields: map[string]string{
			"Identifier":     identifier,
			"Global":         strconv.FormatBool(req.Global),
			"URI":            finalURI,
			"StartupTime":    strconv.FormatInt(req.StartTime, 10),
			"CompletionTime": strconv.FormatInt(time.Now().Unix(), 10),
		},
	})

	c.removeRequest(identifier)
}

// handleGenerateSSK generates a new SSK keypair
func (c *FCPConnection) handleGenerateSSK(msg *FCPMessage) error {
	identifier := msg.Fields["Identifier"]

	log.Printf("[%s] GenerateSSK (identifier: %s)", c.id, identifier)

	// Generate SSK keypair
	privKey, pubKey, err := keys.GenerateSSKKeypair()
	if err != nil {
		return c.SendProtocolError(
			ProtocolErrorInternalError,
			fmt.Sprintf("Failed to generate SSK keypair: %v", err),
			identifier,
			false,
		)
	}

	// Create URIs
	insertURI := fmt.Sprintf("SSK@%s/", hex.EncodeToString(privKey))
	requestURI := fmt.Sprintf("SSK@%s/", hex.EncodeToString(pubKey))

	// Send SSKKeypair message
	return c.SendMessage(&FCPMessage{
		Name: "SSKKeypair",
		Fields: map[string]string{
			"Identifier": identifier,
			"InsertURI":  insertURI,
			"RequestURI": requestURI,
		},
	})
}

// handleGetNode returns node information
func (c *FCPConnection) handleGetNode(msg *FCPMessage) error {
	identifier := msg.Fields["Identifier"]

	log.Printf("[%s] GetNode (identifier: %s)", c.id, identifier)

	// Get location from location manager
	location := c.server.locationMgr.GetLocation()
	stats := c.server.locationMgr.GetStats()

	// Send NodeData message
	return c.SendMessage(&FCPMessage{
		Name: "NodeData",
		Fields: map[string]string{
			"Identifier":     identifier,
			"opennet":        "false",
			"location":       fmt.Sprintf("%.6f", location),
			"swaps":          strconv.Itoa(stats.SwapsSinceReset),
			"version":        "GoHyphanet 0.1",
			"Build":          "1",
		},
	})
}

// handleDisconnect closes the connection
func (c *FCPConnection) handleDisconnect(msg *FCPMessage) error {
	log.Printf("[%s] Disconnect requested", c.id)
	c.Close()
	return nil
}

// handleRemoveRequest removes an active request
func (c *FCPConnection) handleRemoveRequest(msg *FCPMessage) error {
	identifier := msg.Fields["Identifier"]
	global := msg.Fields["Global"] == "true"

	log.Printf("[%s] RemoveRequest: %s", c.id, identifier)

	c.removeRequest(identifier)

	return c.SendMessage(&FCPMessage{
		Name: "RequestRemoved",
		Fields: map[string]string{
			"Identifier": identifier,
			"Global":     strconv.FormatBool(global),
		},
	})
}

// SendGetFailed sends a GetFailed message
func (c *FCPConnection) SendGetFailed(identifier string, code int, description string) error {
	return c.SendMessage(&FCPMessage{
		Name: "GetFailed",
		Fields: map[string]string{
			"Identifier":      identifier,
			"Code":            strconv.Itoa(code),
			"CodeDescription": requests.StatusDataNotFound.String(),
			"ExtraDescription": description,
			"Fatal":           "false",
		},
	})
}

// SendPutFailed sends a PutFailed message
func (c *FCPConnection) SendPutFailed(identifier string, code int, description string) error {
	return c.SendMessage(&FCPMessage{
		Name: "PutFailed",
		Fields: map[string]string{
			"Identifier":       identifier,
			"Code":             strconv.Itoa(code),
			"CodeDescription":  "Internal error",
			"ExtraDescription": description,
			"Fatal":            "false",
		},
	})
}

// removeRequest removes a request from the active requests map
func (c *FCPConnection) removeRequest(identifier string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if req, exists := c.activeRequests[identifier]; exists {
		req.Cancel()
		delete(c.activeRequests, identifier)
	}
}

// GenerateRandomBytes generates random bytes for identifiers
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	return b, err
}
