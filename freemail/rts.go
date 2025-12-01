// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package freemail

import (
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// RTS (Ready-To-Send) protocol constants
const (
	// RTSPollInterval is the interval between RTS polling attempts
	RTSPollInterval = 10 * time.Minute

	// RTSExpiration is the duration after which an RTS becomes invalid
	RTSExpiration = 24 * time.Hour

	// RTSMaxRetries is the maximum number of RTS send attempts
	RTSMaxRetries = 5

	// RTSVersion is the current RTS protocol version
	RTSVersion = 1
)

// RTSState represents the state of an RTS exchange
type RTSState int

const (
	RTSPending RTSState = iota   // RTS sent, waiting for channel establishment
	RTSAccepted                   // RTS accepted, channel established
	RTSRejected                   // RTS rejected by recipient
	RTSExpired                    // RTS expired without response
	RTSFailed                     // RTS failed after max retries
)

// String returns the string representation of RTSState
func (s RTSState) String() string {
	switch s {
	case RTSPending:
		return "pending"
	case RTSAccepted:
		return "accepted"
	case RTSRejected:
		return "rejected"
	case RTSExpired:
		return "expired"
	case RTSFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// RTSRequest represents an outgoing RTS request
type RTSRequest struct {
	ID                string    `json:"id"`
	RecipientIdentity string    `json:"recipient_identity"`
	RecipientRTSKey   string    `json:"recipient_rts_key"`
	SenderMailsiteURI string    `json:"sender_mailsite_uri"`
	SenderIdentity    string    `json:"sender_identity"`
	InitiatorSlot     int       `json:"initiator_slot"`
	ResponderSlot     int       `json:"responder_slot"`
	ChannelAESKey     []byte    `json:"channel_aes_key"`
	ChannelIV         []byte    `json:"channel_iv"`
	State             RTSState  `json:"state"`
	CreatedAt         time.Time `json:"created_at"`
	ExpiresAt         time.Time `json:"expires_at"`
	Retries           int       `json:"retries"`
	LastError         string    `json:"last_error,omitempty"`
}

// NewRTSRequest creates a new RTS request
func NewRTSRequest(recipientIdentity, recipientRTSKey, senderMailsiteURI, senderIdentity string) (*RTSRequest, error) {
	// Generate channel encryption keys
	aesKey, err := GenerateAESKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate AES key: %w", err)
	}

	iv, err := GenerateIV()
	if err != nil {
		return nil, fmt.Errorf("failed to generate IV: %w", err)
	}

	return &RTSRequest{
		ID:                fmt.Sprintf("rts-%d", time.Now().UnixNano()),
		RecipientIdentity: recipientIdentity,
		RecipientRTSKey:   recipientRTSKey,
		SenderMailsiteURI: senderMailsiteURI,
		SenderIdentity:    senderIdentity,
		InitiatorSlot:     0,
		ResponderSlot:     0,
		ChannelAESKey:     aesKey,
		ChannelIV:         iv,
		State:             RTSPending,
		CreatedAt:         time.Now(),
		ExpiresAt:         time.Now().Add(RTSExpiration),
	}, nil
}

// IsExpired checks if the RTS request has expired
func (r *RTSRequest) IsExpired() bool {
	return time.Now().After(r.ExpiresAt)
}

// RTSPayloadData is the data structure for RTS payload
type RTSPayloadData struct {
	Version           int    `json:"version"`
	SenderMailsiteURI string `json:"sender_mailsite_uri"`
	SenderIdentity    string `json:"sender_identity"`
	RecipientIdentity string `json:"recipient_identity"`
	InitiatorSlot     int    `json:"initiator_slot"`
	ResponderSlot     int    `json:"responder_slot"`
	ChannelID         string `json:"channel_id"`
	Timestamp         int64  `json:"timestamp"`
}

// RTSEncoder handles encoding/decoding of RTS messages
type RTSEncoder struct {
	privateKey *rsa.PrivateKey
}

// NewRTSEncoder creates a new RTS encoder
func NewRTSEncoder(privateKey *rsa.PrivateKey) *RTSEncoder {
	return &RTSEncoder{
		privateKey: privateKey,
	}
}

// EncodeRTS encodes an RTS request for transmission
func (e *RTSEncoder) EncodeRTS(request *RTSRequest, recipientPublicKey *rsa.PublicKey) (*RTSMessage, error) {
	// Create payload
	payload := &RTSPayloadData{
		Version:           RTSVersion,
		SenderMailsiteURI: request.SenderMailsiteURI,
		SenderIdentity:    request.SenderIdentity,
		RecipientIdentity: request.RecipientIdentity,
		InitiatorSlot:     request.InitiatorSlot,
		ResponderSlot:     request.ResponderSlot,
		ChannelID:         request.ID,
		Timestamp:         time.Now().Unix(),
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Encrypt payload with channel AES key
	encryptedPayload, err := EncryptAES(payloadJSON, request.ChannelAESKey, request.ChannelIV)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt payload: %w", err)
	}

	// Combine AES key and IV for RSA encryption
	keyMaterial := make([]byte, len(request.ChannelAESKey)+len(request.ChannelIV))
	copy(keyMaterial, request.ChannelAESKey)
	copy(keyMaterial[len(request.ChannelAESKey):], request.ChannelIV)

	// Encrypt AES key with recipient's public key
	encryptedKey, err := EncryptRSA(keyMaterial, recipientPublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt key: %w", err)
	}

	// Sign the encrypted payload
	signature, err := SignSHA256(encryptedPayload, e.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	return &RTSMessage{
		EncryptedAESKey:  encryptedKey,
		EncryptedPayload: encryptedPayload,
		Signature:        signature,
	}, nil
}

// DecodeRTS decodes an incoming RTS message
func (e *RTSEncoder) DecodeRTS(message *RTSMessage, senderPublicKey *rsa.PublicKey) (*RTSPayloadData, []byte, []byte, error) {
	// Verify signature
	if senderPublicKey != nil {
		if err := VerifySHA256(message.EncryptedPayload, message.Signature, senderPublicKey); err != nil {
			return nil, nil, nil, fmt.Errorf("signature verification failed: %w", err)
		}
	}

	// Decrypt AES key material
	keyMaterial, err := DecryptRSA(message.EncryptedAESKey, e.privateKey)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to decrypt key: %w", err)
	}

	if len(keyMaterial) != AESKeySize+AESBlockSize {
		return nil, nil, nil, fmt.Errorf("invalid key material length")
	}

	aesKey := keyMaterial[:AESKeySize]
	iv := keyMaterial[AESKeySize:]

	// Decrypt payload
	payloadJSON, err := DecryptAES(message.EncryptedPayload, aesKey, iv)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to decrypt payload: %w", err)
	}

	// Parse payload
	var payload RTSPayloadData
	if err := json.Unmarshal(payloadJSON, &payload); err != nil {
		return nil, nil, nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	return &payload, aesKey, iv, nil
}

// RTSManager manages RTS requests and responses
type RTSManager struct {
	mu sync.RWMutex

	// Data directory
	dataDir string

	// Pending outgoing RTS requests
	outgoing map[string]*RTSRequest

	// Pending incoming RTS requests awaiting processing
	incoming map[string]*RTSPayloadData

	// Encoder for RTS messages
	encoder *RTSEncoder

	// Callbacks
	onChannelEstablished func(channelID string, remoteIdentity string, aesKey, iv []byte)
	onRTSReceived        func(payload *RTSPayloadData, aesKey, iv []byte)
}

// NewRTSManager creates a new RTS manager
func NewRTSManager(dataDir string, privateKey *rsa.PrivateKey) *RTSManager {
	return &RTSManager{
		dataDir:  dataDir,
		outgoing: make(map[string]*RTSRequest),
		incoming: make(map[string]*RTSPayloadData),
		encoder:  NewRTSEncoder(privateKey),
	}
}

// SetChannelCallback sets the callback for channel establishment
func (rm *RTSManager) SetChannelCallback(callback func(channelID string, remoteIdentity string, aesKey, iv []byte)) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.onChannelEstablished = callback
}

// SetRTSReceivedCallback sets the callback for received RTS messages
func (rm *RTSManager) SetRTSReceivedCallback(callback func(payload *RTSPayloadData, aesKey, iv []byte)) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.onRTSReceived = callback
}

// CreateRTS creates a new RTS request to initiate a channel
func (rm *RTSManager) CreateRTS(recipientIdentity, recipientRTSKey, senderMailsiteURI, senderIdentity string) (*RTSRequest, error) {
	request, err := NewRTSRequest(recipientIdentity, recipientRTSKey, senderMailsiteURI, senderIdentity)
	if err != nil {
		return nil, err
	}

	rm.mu.Lock()
	rm.outgoing[request.ID] = request
	rm.mu.Unlock()

	return request, nil
}

// EncodeRTSForSending encodes an RTS request for transmission
func (rm *RTSManager) EncodeRTSForSending(requestID string, recipientPublicKey *rsa.PublicKey) (*RTSMessage, error) {
	rm.mu.RLock()
	request, exists := rm.outgoing[requestID]
	rm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("RTS request not found: %s", requestID)
	}

	return rm.encoder.EncodeRTS(request, recipientPublicKey)
}

// ProcessIncomingRTS processes an incoming RTS message
func (rm *RTSManager) ProcessIncomingRTS(message *RTSMessage, senderPublicKey *rsa.PublicKey) (*RTSPayloadData, error) {
	payload, aesKey, iv, err := rm.encoder.DecodeRTS(message, senderPublicKey)
	if err != nil {
		return nil, err
	}

	// Store the incoming RTS
	rm.mu.Lock()
	rm.incoming[payload.ChannelID] = payload
	rm.mu.Unlock()

	// Notify via callback
	rm.mu.RLock()
	callback := rm.onRTSReceived
	rm.mu.RUnlock()

	if callback != nil {
		callback(payload, aesKey, iv)
	}

	return payload, nil
}

// AcceptRTS accepts an incoming RTS and establishes the channel
func (rm *RTSManager) AcceptRTS(channelID string, aesKey, iv []byte) error {
	rm.mu.Lock()
	payload, exists := rm.incoming[channelID]
	if !exists {
		rm.mu.Unlock()
		return fmt.Errorf("incoming RTS not found: %s", channelID)
	}
	delete(rm.incoming, channelID)
	rm.mu.Unlock()

	// Notify channel establishment
	rm.mu.RLock()
	callback := rm.onChannelEstablished
	rm.mu.RUnlock()

	if callback != nil {
		callback(channelID, payload.SenderIdentity, aesKey, iv)
	}

	return nil
}

// RejectRTS rejects an incoming RTS
func (rm *RTSManager) RejectRTS(channelID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.incoming[channelID]; !exists {
		return fmt.Errorf("incoming RTS not found: %s", channelID)
	}
	delete(rm.incoming, channelID)

	return nil
}

// MarkRTSSent marks an RTS as having been sent
func (rm *RTSManager) MarkRTSSent(requestID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if request, exists := rm.outgoing[requestID]; exists {
		request.Retries++
	}
}

// MarkRTSFailed marks an RTS as failed
func (rm *RTSManager) MarkRTSFailed(requestID string, err string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if request, exists := rm.outgoing[requestID]; exists {
		request.LastError = err
		request.Retries++
		if request.Retries >= RTSMaxRetries {
			request.State = RTSFailed
		}
	}
}

// MarkRTSAccepted marks an RTS as accepted (channel established)
func (rm *RTSManager) MarkRTSAccepted(requestID string) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if request, exists := rm.outgoing[requestID]; exists {
		request.State = RTSAccepted
	}
}

// GetPendingRTS returns all pending RTS requests
func (rm *RTSManager) GetPendingRTS() []*RTSRequest {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	var pending []*RTSRequest
	for _, request := range rm.outgoing {
		if request.State == RTSPending && !request.IsExpired() {
			pending = append(pending, request)
		}
	}
	return pending
}

// CleanExpired removes expired RTS requests
func (rm *RTSManager) CleanExpired() int {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	cleaned := 0
	for id, request := range rm.outgoing {
		if request.IsExpired() && request.State == RTSPending {
			request.State = RTSExpired
			delete(rm.outgoing, id)
			cleaned++
		}
	}
	return cleaned
}

// RTSFetcher fetches incoming RTS messages from Freenet
type RTSFetcher struct {
	mu sync.RWMutex

	rtsManager *RTSManager
	fetcher    RTSKeyFetcher // Interface for fetching RTS data

	// Account's RTS key for incoming messages
	rtsKey string

	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// RTSKeyFetcher interface for fetching RTS data from Freenet
type RTSKeyFetcher interface {
	// FetchRTSKey fetches data from an RTS key
	FetchRTSKey(key string) ([]byte, error)

	// GetPublicKey retrieves a public key for an identity
	GetPublicKey(identity string) (*rsa.PublicKey, error)
}

// NewRTSFetcher creates a new RTS fetcher
func NewRTSFetcher(rtsManager *RTSManager, fetcher RTSKeyFetcher, rtsKey string) *RTSFetcher {
	return &RTSFetcher{
		rtsManager: rtsManager,
		fetcher:    fetcher,
		rtsKey:     rtsKey,
		stopChan:   make(chan struct{}),
	}
}

// Start begins polling for RTS messages
func (rf *RTSFetcher) Start() {
	rf.mu.Lock()
	if rf.running {
		rf.mu.Unlock()
		return
	}
	rf.running = true
	rf.stopChan = make(chan struct{})
	rf.mu.Unlock()

	rf.wg.Add(1)
	go rf.pollLoop()
}

// Stop stops the RTS fetcher
func (rf *RTSFetcher) Stop() {
	rf.mu.Lock()
	if !rf.running {
		rf.mu.Unlock()
		return
	}
	rf.running = false
	close(rf.stopChan)
	rf.mu.Unlock()

	rf.wg.Wait()
}

// pollLoop is the main polling loop
func (rf *RTSFetcher) pollLoop() {
	defer rf.wg.Done()

	ticker := time.NewTicker(RTSPollInterval)
	defer ticker.Stop()

	// Initial poll
	rf.pollOnce()

	for {
		select {
		case <-rf.stopChan:
			return
		case <-ticker.C:
			rf.pollOnce()
		}
	}
}

// pollOnce performs one RTS polling attempt
func (rf *RTSFetcher) pollOnce() {
	// Fetch RTS data from our RTS key
	data, err := rf.fetcher.FetchRTSKey(rf.rtsKey)
	if err != nil || data == nil {
		return
	}

	// Parse the RTS message
	rtsMsg, err := DeserializeRTSMessage(data)
	if err != nil {
		return
	}

	// We need to determine the sender's identity to verify the signature
	// For now, process without signature verification
	// In a full implementation, the RTS would include sender identity info
	_, err = rf.rtsManager.ProcessIncomingRTS(rtsMsg, nil)
	if err != nil {
		return
	}
}

// SerializeRTSMessage serializes an RTS message for transmission
func SerializeRTSMessage(msg *RTSMessage) []byte {
	// Format: keyLen(4) | key | payloadLen(4) | payload | sigLen(4) | sig
	keyLen := len(msg.EncryptedAESKey)
	payloadLen := len(msg.EncryptedPayload)
	sigLen := len(msg.Signature)

	totalLen := 12 + keyLen + payloadLen + sigLen
	data := make([]byte, totalLen)

	offset := 0

	// Key length and key
	data[offset] = byte(keyLen >> 24)
	data[offset+1] = byte(keyLen >> 16)
	data[offset+2] = byte(keyLen >> 8)
	data[offset+3] = byte(keyLen)
	offset += 4
	copy(data[offset:], msg.EncryptedAESKey)
	offset += keyLen

	// Payload length and payload
	data[offset] = byte(payloadLen >> 24)
	data[offset+1] = byte(payloadLen >> 16)
	data[offset+2] = byte(payloadLen >> 8)
	data[offset+3] = byte(payloadLen)
	offset += 4
	copy(data[offset:], msg.EncryptedPayload)
	offset += payloadLen

	// Signature length and signature
	data[offset] = byte(sigLen >> 24)
	data[offset+1] = byte(sigLen >> 16)
	data[offset+2] = byte(sigLen >> 8)
	data[offset+3] = byte(sigLen)
	offset += 4
	copy(data[offset:], msg.Signature)

	return data
}

// DeserializeRTSMessage deserializes an RTS message from bytes
func DeserializeRTSMessage(data []byte) (*RTSMessage, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("data too short")
	}

	offset := 0

	// Key length and key
	keyLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
	offset += 4
	if offset+keyLen > len(data) {
		return nil, fmt.Errorf("invalid key length")
	}
	encryptedKey := make([]byte, keyLen)
	copy(encryptedKey, data[offset:offset+keyLen])
	offset += keyLen

	// Payload length and payload
	if offset+4 > len(data) {
		return nil, fmt.Errorf("data too short for payload length")
	}
	payloadLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
	offset += 4
	if offset+payloadLen > len(data) {
		return nil, fmt.Errorf("invalid payload length")
	}
	encryptedPayload := make([]byte, payloadLen)
	copy(encryptedPayload, data[offset:offset+payloadLen])
	offset += payloadLen

	// Signature length and signature
	if offset+4 > len(data) {
		return nil, fmt.Errorf("data too short for signature length")
	}
	sigLen := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
	offset += 4
	if offset+sigLen > len(data) {
		return nil, fmt.Errorf("invalid signature length")
	}
	signature := make([]byte, sigLen)
	copy(signature, data[offset:offset+sigLen])

	return &RTSMessage{
		EncryptedAESKey:  encryptedKey,
		EncryptedPayload: encryptedPayload,
		Signature:        signature,
	}, nil
}

// RTSSender sends RTS messages to Freenet
type RTSSender struct {
	mu sync.RWMutex

	rtsManager *RTSManager
	inserter   RTSInserter // Interface for inserting RTS data

	running  bool
	stopChan chan struct{}
	wg       sync.WaitGroup
}

// RTSInserter interface for inserting RTS data to Freenet
type RTSInserter interface {
	// InsertRTS inserts an RTS message to a key
	InsertRTS(key string, data []byte) error

	// GetPublicKey retrieves a public key for an identity
	GetPublicKey(identity string) (*rsa.PublicKey, error)
}

// NewRTSSender creates a new RTS sender
func NewRTSSender(rtsManager *RTSManager, inserter RTSInserter) *RTSSender {
	return &RTSSender{
		rtsManager: rtsManager,
		inserter:   inserter,
		stopChan:   make(chan struct{}),
	}
}

// Start begins sending pending RTS messages
func (rs *RTSSender) Start() {
	rs.mu.Lock()
	if rs.running {
		rs.mu.Unlock()
		return
	}
	rs.running = true
	rs.stopChan = make(chan struct{})
	rs.mu.Unlock()

	rs.wg.Add(1)
	go rs.sendLoop()
}

// Stop stops the RTS sender
func (rs *RTSSender) Stop() {
	rs.mu.Lock()
	if !rs.running {
		rs.mu.Unlock()
		return
	}
	rs.running = false
	close(rs.stopChan)
	rs.mu.Unlock()

	rs.wg.Wait()
}

// sendLoop is the main sending loop
func (rs *RTSSender) sendLoop() {
	defer rs.wg.Done()

	ticker := time.NewTicker(RTSPollInterval)
	defer ticker.Stop()

	// Initial send attempt
	rs.sendPending()

	for {
		select {
		case <-rs.stopChan:
			return
		case <-ticker.C:
			rs.sendPending()
		}
	}
}

// sendPending sends all pending RTS messages
func (rs *RTSSender) sendPending() {
	pending := rs.rtsManager.GetPendingRTS()

	for _, request := range pending {
		// Get recipient's public key
		pubKey, err := rs.inserter.GetPublicKey(request.RecipientIdentity)
		if err != nil {
			rs.rtsManager.MarkRTSFailed(request.ID, err.Error())
			continue
		}

		// Encode the RTS message
		rtsMsg, err := rs.rtsManager.EncodeRTSForSending(request.ID, pubKey)
		if err != nil {
			rs.rtsManager.MarkRTSFailed(request.ID, err.Error())
			continue
		}

		// Serialize and insert
		data := SerializeRTSMessage(rtsMsg)
		if err := rs.inserter.InsertRTS(request.RecipientRTSKey, data); err != nil {
			rs.rtsManager.MarkRTSFailed(request.ID, err.Error())
			continue
		}

		rs.rtsManager.MarkRTSSent(request.ID)
	}

	// Clean expired RTS requests
	rs.rtsManager.CleanExpired()
}

// SendRTSNow immediately sends an RTS request
func (rs *RTSSender) SendRTSNow(requestID string) error {
	rs.rtsManager.mu.RLock()
	request, exists := rs.rtsManager.outgoing[requestID]
	rs.rtsManager.mu.RUnlock()

	if !exists {
		return fmt.Errorf("RTS request not found: %s", requestID)
	}

	// Get recipient's public key
	pubKey, err := rs.inserter.GetPublicKey(request.RecipientIdentity)
	if err != nil {
		rs.rtsManager.MarkRTSFailed(requestID, err.Error())
		return fmt.Errorf("failed to get public key: %w", err)
	}

	// Encode the RTS message
	rtsMsg, err := rs.rtsManager.EncodeRTSForSending(requestID, pubKey)
	if err != nil {
		rs.rtsManager.MarkRTSFailed(requestID, err.Error())
		return fmt.Errorf("failed to encode RTS: %w", err)
	}

	// Serialize and insert
	data := SerializeRTSMessage(rtsMsg)
	if err := rs.inserter.InsertRTS(request.RecipientRTSKey, data); err != nil {
		rs.rtsManager.MarkRTSFailed(requestID, err.Error())
		return fmt.Errorf("failed to insert RTS: %w", err)
	}

	rs.rtsManager.MarkRTSSent(requestID)
	return nil
}
