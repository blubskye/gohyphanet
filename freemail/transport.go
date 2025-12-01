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

// Transport configuration
const (
	// MaxMessageSize is the maximum message size in bytes
	MaxMessageSize = 1024 * 1024 // 1MB

	// TransportTimeout is the timeout for transport operations
	TransportTimeout = 5 * time.Minute

	// ChannelRefreshInterval is how often to check channel health
	ChannelRefreshInterval = 1 * time.Hour
)

// TransportMessage represents a message in transit
type TransportMessage struct {
	ID           string    `json:"id"`
	ChannelID    string    `json:"channel_id"`
	SenderID     string    `json:"sender_id"`
	RecipientID  string    `json:"recipient_id"`
	Subject      string    `json:"subject"`
	Body         []byte    `json:"body"`
	Headers      map[string]string `json:"headers,omitempty"`
	Timestamp    int64     `json:"timestamp"`
	SlotNumber   int       `json:"slot_number"`
}

// NewTransportMessage creates a new transport message
func NewTransportMessage(channelID, senderID, recipientID, subject string, body []byte) *TransportMessage {
	return &TransportMessage{
		ID:          fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		ChannelID:   channelID,
		SenderID:    senderID,
		RecipientID: recipientID,
		Subject:     subject,
		Body:        body,
		Headers:     make(map[string]string),
		Timestamp:   time.Now().Unix(),
	}
}

// Serialize converts the message to JSON bytes
func (tm *TransportMessage) Serialize() ([]byte, error) {
	return json.Marshal(tm)
}

// DeserializeTransportMessage parses JSON bytes into a TransportMessage
func DeserializeTransportMessage(data []byte) (*TransportMessage, error) {
	var msg TransportMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal message: %w", err)
	}
	return &msg, nil
}

// ChannelTransport manages message transport for a single channel
type ChannelTransport struct {
	mu sync.RWMutex

	channel *Channel
	crypto  *MessageCrypto
	slots   *SlotRange

	// Remote party's public key for encryption
	remotePublicKey *rsa.PublicKey

	// Outbound message queue
	outbound []*TransportMessage

	// Callbacks
	onMessageSent     func(msg *TransportMessage)
	onMessageReceived func(msg *TransportMessage)
}

// NewChannelTransport creates a new channel transport
func NewChannelTransport(channel *Channel, privateKey *rsa.PrivateKey, remotePublicKey *rsa.PublicKey) *ChannelTransport {
	return &ChannelTransport{
		channel:         channel,
		crypto:          NewMessageCrypto(privateKey),
		remotePublicKey: remotePublicKey,
		outbound:        make([]*TransportMessage, 0),
	}
}

// SetSendSlots sets the slot range for sending
func (ct *ChannelTransport) SetSendSlots(slots *SlotRange) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.slots = slots
}

// SetMessageSentCallback sets the callback for sent messages
func (ct *ChannelTransport) SetMessageSentCallback(callback func(msg *TransportMessage)) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.onMessageSent = callback
}

// SetMessageReceivedCallback sets the callback for received messages
func (ct *ChannelTransport) SetMessageReceivedCallback(callback func(msg *TransportMessage)) {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	ct.onMessageReceived = callback
}

// QueueMessage queues a message for sending
func (ct *ChannelTransport) QueueMessage(subject string, body []byte, senderID, recipientID string) *TransportMessage {
	msg := NewTransportMessage(ct.channel.ID, senderID, recipientID, subject, body)

	ct.mu.Lock()
	ct.outbound = append(ct.outbound, msg)
	ct.mu.Unlock()

	return msg
}

// GetPendingMessages returns messages waiting to be sent
func (ct *ChannelTransport) GetPendingMessages() []*TransportMessage {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	result := make([]*TransportMessage, len(ct.outbound))
	copy(result, ct.outbound)
	return result
}

// EncryptMessage encrypts a message for transmission
func (ct *ChannelTransport) EncryptMessage(msg *TransportMessage) (*EncryptedMessage, error) {
	// Serialize message
	data, err := msg.Serialize()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize message: %w", err)
	}

	// Check size
	if len(data) > MaxMessageSize {
		return nil, fmt.Errorf("message too large: %d bytes (max %d)", len(data), MaxMessageSize)
	}

	// Encrypt using channel's AES key if available, otherwise use RSA
	ct.mu.RLock()
	channel := ct.channel
	ct.mu.RUnlock()

	if channel.AESKey != nil && channel.AESIV != nil {
		// Use channel's symmetric key
		encrypted, err := EncryptAES(data, channel.AESKey, channel.AESIV)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt: %w", err)
		}

		return &EncryptedMessage{
			EncryptedBody: encrypted,
			IV:            channel.AESIV,
		}, nil
	}

	// Fall back to RSA encryption for initial messages
	return ct.crypto.EncryptMessage(data, ct.remotePublicKey)
}

// DecryptMessage decrypts a received message
func (ct *ChannelTransport) DecryptMessage(encrypted *EncryptedMessage, senderPublicKey *rsa.PublicKey) (*TransportMessage, error) {
	var data []byte
	var err error

	ct.mu.RLock()
	channel := ct.channel
	ct.mu.RUnlock()

	if channel.AESKey != nil && channel.AESIV != nil {
		// Use channel's symmetric key
		data, err = DecryptAES(encrypted.EncryptedBody, channel.AESKey, encrypted.IV)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt: %w", err)
		}
	} else {
		// Use RSA decryption
		data, err = ct.crypto.DecryptMessage(encrypted, senderPublicKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt: %w", err)
		}
	}

	// Deserialize
	msg, err := DeserializeTransportMessage(data)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize: %w", err)
	}

	return msg, nil
}

// MarkMessageSent marks a message as sent and removes from queue
func (ct *ChannelTransport) MarkMessageSent(msgID string, slotNumber int) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	for i, msg := range ct.outbound {
		if msg.ID == msgID {
			msg.SlotNumber = slotNumber
			ct.outbound = append(ct.outbound[:i], ct.outbound[i+1:]...)

			if ct.onMessageSent != nil {
				go ct.onMessageSent(msg)
			}
			break
		}
	}
}

// ProcessReceivedMessage processes a received encrypted message
func (ct *ChannelTransport) ProcessReceivedMessage(encrypted *EncryptedMessage, senderPublicKey *rsa.PublicKey) (*TransportMessage, error) {
	msg, err := ct.DecryptMessage(encrypted, senderPublicKey)
	if err != nil {
		return nil, err
	}

	ct.mu.RLock()
	callback := ct.onMessageReceived
	ct.mu.RUnlock()

	if callback != nil {
		go callback(msg)
	}

	return msg, nil
}

// TransportManager manages all channel transports
type TransportManager struct {
	mu sync.RWMutex

	// Private key for decryption
	privateKey *rsa.PrivateKey

	// Channel transports
	transports map[string]*ChannelTransport

	// Slot manager
	slotManager *SlotManager

	// RTS manager
	rtsManager *RTSManager

	// Mailsite fetcher for getting remote public keys
	mailsiteFetcher *MailsiteFetcher

	// FCP interface for Freenet operations
	fcpInterface FCPInterface

	// Callbacks
	onChannelCreated  func(channelID string)
	onMessageReceived func(channelID string, msg *TransportMessage)
}

// FCPInterface defines the interface for Freenet operations
type FCPInterface interface {
	// InsertData inserts data to a key
	InsertData(key string, data []byte) error

	// FetchData fetches data from a key
	FetchData(key string) ([]byte, error)

	// GetPublicKey retrieves a public key for an identity
	GetPublicKey(identity string) (*rsa.PublicKey, error)
}

// NewTransportManager creates a new transport manager
func NewTransportManager(privateKey *rsa.PrivateKey, dataDir string) *TransportManager {
	return &TransportManager{
		privateKey:  privateKey,
		transports:  make(map[string]*ChannelTransport),
		slotManager: NewSlotManager(dataDir),
		rtsManager:  NewRTSManager(dataDir, privateKey),
	}
}

// SetFCPInterface sets the FCP interface
func (tm *TransportManager) SetFCPInterface(fcp FCPInterface) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.fcpInterface = fcp
}

// SetMailsiteFetcher sets the mailsite fetcher
func (tm *TransportManager) SetMailsiteFetcher(fetcher *MailsiteFetcher) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.mailsiteFetcher = fetcher
}

// SetChannelCreatedCallback sets the callback for channel creation
func (tm *TransportManager) SetChannelCreatedCallback(callback func(channelID string)) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.onChannelCreated = callback
}

// SetMessageReceivedCallback sets the global message received callback
func (tm *TransportManager) SetMessageReceivedCallback(callback func(channelID string, msg *TransportMessage)) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.onMessageReceived = callback
}

// InitiateChannel initiates a new channel with a remote identity
func (tm *TransportManager) InitiateChannel(recipientIdentity, recipientRTSKey, senderMailsiteURI, senderIdentity string) (string, error) {
	// Create RTS request
	request, err := tm.rtsManager.CreateRTS(recipientIdentity, recipientRTSKey, senderMailsiteURI, senderIdentity)
	if err != nil {
		return "", fmt.Errorf("failed to create RTS: %w", err)
	}

	// Create channel
	channel := NewChannel(recipientIdentity)
	channel.ID = request.ID
	channel.AESKey = request.ChannelAESKey
	channel.AESIV = request.ChannelIV

	// Get recipient's public key
	tm.mu.RLock()
	fcp := tm.fcpInterface
	tm.mu.RUnlock()

	if fcp == nil {
		return "", fmt.Errorf("FCP interface not configured")
	}

	pubKey, err := fcp.GetPublicKey(recipientIdentity)
	if err != nil {
		return "", fmt.Errorf("failed to get recipient public key: %w", err)
	}

	// Create transport
	transport := NewChannelTransport(channel, tm.privateKey, pubKey)

	// Initialize slots
	tm.slotManager.InitializeChannel(channel.ID,
		fmt.Sprintf("%s-send", channel.ID),
		fmt.Sprintf("%s-recv", channel.ID))

	// Store transport
	tm.mu.Lock()
	tm.transports[channel.ID] = transport
	tm.mu.Unlock()

	// Set up message callback
	transport.SetMessageReceivedCallback(func(msg *TransportMessage) {
		tm.mu.RLock()
		callback := tm.onMessageReceived
		tm.mu.RUnlock()
		if callback != nil {
			callback(channel.ID, msg)
		}
	})

	return channel.ID, nil
}

// AcceptChannel accepts an incoming channel from an RTS
func (tm *TransportManager) AcceptChannel(channelID string, remoteIdentity string, aesKey, iv []byte) error {
	// Create channel
	channel := NewChannel(remoteIdentity)
	channel.ID = channelID
	channel.AESKey = aesKey
	channel.AESIV = iv

	// Get sender's public key
	tm.mu.RLock()
	fcp := tm.fcpInterface
	tm.mu.RUnlock()

	if fcp == nil {
		return fmt.Errorf("FCP interface not configured")
	}

	pubKey, err := fcp.GetPublicKey(remoteIdentity)
	if err != nil {
		return fmt.Errorf("failed to get sender public key: %w", err)
	}

	// Create transport
	transport := NewChannelTransport(channel, tm.privateKey, pubKey)

	// Initialize slots
	tm.slotManager.InitializeChannel(channelID,
		fmt.Sprintf("%s-send", channelID),
		fmt.Sprintf("%s-recv", channelID))

	// Store transport
	tm.mu.Lock()
	tm.transports[channelID] = transport
	callback := tm.onChannelCreated
	tm.mu.Unlock()

	// Set up message callback
	transport.SetMessageReceivedCallback(func(msg *TransportMessage) {
		tm.mu.RLock()
		msgCallback := tm.onMessageReceived
		tm.mu.RUnlock()
		if msgCallback != nil {
			msgCallback(channelID, msg)
		}
	})

	// Notify channel creation
	if callback != nil {
		callback(channelID)
	}

	// Accept the RTS
	return tm.rtsManager.AcceptRTS(channelID, aesKey, iv)
}

// GetTransport returns the transport for a channel
func (tm *TransportManager) GetTransport(channelID string) *ChannelTransport {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.transports[channelID]
}

// SendMessage sends a message through a channel
func (tm *TransportManager) SendMessage(channelID, subject string, body []byte, senderID, recipientID string) error {
	transport := tm.GetTransport(channelID)
	if transport == nil {
		return fmt.Errorf("channel not found: %s", channelID)
	}

	// Queue message
	msg := transport.QueueMessage(subject, body, senderID, recipientID)

	// Encrypt
	encrypted, err := transport.EncryptMessage(msg)
	if err != nil {
		return fmt.Errorf("failed to encrypt: %w", err)
	}

	// Allocate slot
	slot := tm.slotManager.AllocateSendSlot(channelID)

	// Serialize encrypted message
	data := encrypted.Serialize()

	// Insert to Freenet
	tm.mu.RLock()
	fcp := tm.fcpInterface
	tm.mu.RUnlock()

	if fcp == nil {
		return fmt.Errorf("FCP interface not configured")
	}

	if err := fcp.InsertData(slot.Key, data); err != nil {
		return fmt.Errorf("failed to insert message: %w", err)
	}

	// Mark sent
	transport.MarkMessageSent(msg.ID, slot.Number)
	tm.slotManager.MarkSent(channelID, slot.Number, msg.ID)

	return nil
}

// ProcessIncomingData processes incoming data from a slot
func (tm *TransportManager) ProcessIncomingData(channelID string, slotNumber int, data []byte) (*TransportMessage, error) {
	transport := tm.GetTransport(channelID)
	if transport == nil {
		return nil, fmt.Errorf("channel not found: %s", channelID)
	}

	// Deserialize encrypted message
	encrypted, err := DeserializeEncryptedMessage(data)
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize: %w", err)
	}

	// Get sender's public key
	tm.mu.RLock()
	fcp := tm.fcpInterface
	channel := transport.channel
	tm.mu.RUnlock()

	var senderPubKey *rsa.PublicKey
	if fcp != nil && channel != nil {
		senderPubKey, _ = fcp.GetPublicKey(channel.RemoteIdentity)
	}

	// Decrypt and process
	msg, err := transport.ProcessReceivedMessage(encrypted, senderPubKey)
	if err != nil {
		return nil, fmt.Errorf("failed to process message: %w", err)
	}

	// Mark slot as received
	tm.slotManager.MarkReceived(channelID, slotNumber, msg.ID)

	return msg, nil
}

// GetSlotManager returns the slot manager
func (tm *TransportManager) GetSlotManager() *SlotManager {
	return tm.slotManager
}

// GetRTSManager returns the RTS manager
func (tm *TransportManager) GetRTSManager() *RTSManager {
	return tm.rtsManager
}

// ListChannels returns all channel IDs
func (tm *TransportManager) ListChannels() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	channels := make([]string, 0, len(tm.transports))
	for id := range tm.transports {
		channels = append(channels, id)
	}
	return channels
}

// CloseChannel closes a channel
func (tm *TransportManager) CloseChannel(channelID string) error {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, exists := tm.transports[channelID]; !exists {
		return fmt.Errorf("channel not found: %s", channelID)
	}

	delete(tm.transports, channelID)
	return nil
}

// Save persists transport state
func (tm *TransportManager) Save() error {
	return tm.slotManager.Save()
}

// Load loads transport state
func (tm *TransportManager) Load() error {
	return tm.slotManager.Load()
}
