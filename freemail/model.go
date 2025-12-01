// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

// Package freemail provides anonymous email over Freenet/Hyphanet.
package freemail

import (
	"crypto/rsa"
	"encoding/base32"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Version is the GoFreemail version
const Version = "0.1.0"

// ClientName is the client name
const ClientName = "GoFreemail"

// Default ports
const (
	DefaultSMTPPort = 3025
	DefaultIMAPPort = 3143
	DefaultWebPort  = 3080
)

// FreemailDomain is the domain suffix for Freemail addresses
const FreemailDomain = "freemail"

// MessageFlag represents IMAP message flags
type MessageFlag int

const (
	FlagNone MessageFlag = 0
	FlagSeen MessageFlag = 1 << iota
	FlagAnswered
	FlagFlagged
	FlagDeleted
	FlagDraft
	FlagRecent
)

// String returns the IMAP flag name
func (f MessageFlag) String() string {
	var flags []string
	if f&FlagSeen != 0 {
		flags = append(flags, `\Seen`)
	}
	if f&FlagAnswered != 0 {
		flags = append(flags, `\Answered`)
	}
	if f&FlagFlagged != 0 {
		flags = append(flags, `\Flagged`)
	}
	if f&FlagDeleted != 0 {
		flags = append(flags, `\Deleted`)
	}
	if f&FlagDraft != 0 {
		flags = append(flags, `\Draft`)
	}
	if f&FlagRecent != 0 {
		flags = append(flags, `\Recent`)
	}
	return strings.Join(flags, " ")
}

// ParseFlags parses IMAP flag strings
func ParseFlags(flagStr string) MessageFlag {
	var flags MessageFlag
	flagStr = strings.ToLower(flagStr)
	if strings.Contains(flagStr, "seen") {
		flags |= FlagSeen
	}
	if strings.Contains(flagStr, "answered") {
		flags |= FlagAnswered
	}
	if strings.Contains(flagStr, "flagged") {
		flags |= FlagFlagged
	}
	if strings.Contains(flagStr, "deleted") {
		flags |= FlagDeleted
	}
	if strings.Contains(flagStr, "draft") {
		flags |= FlagDraft
	}
	if strings.Contains(flagStr, "recent") {
		flags |= FlagRecent
	}
	return flags
}

// EmailAddress represents a Freemail email address
type EmailAddress struct {
	Local    string // Local part (username)
	Identity string // WoT identity hash (Base32)
	Domain   string // Always "freemail"
}

// Freemail address regex: user@identity.freemail
var emailRegex = regexp.MustCompile(`^([^@]+)@([a-zA-Z0-9]+)\.freemail$`)

// ParseEmailAddress parses a Freemail address
func ParseEmailAddress(addr string) (*EmailAddress, error) {
	addr = strings.TrimSpace(addr)

	// Handle angle brackets: <user@identity.freemail>
	if strings.HasPrefix(addr, "<") && strings.HasSuffix(addr, ">") {
		addr = addr[1 : len(addr)-1]
	}

	matches := emailRegex.FindStringSubmatch(addr)
	if matches == nil {
		return nil, fmt.Errorf("invalid Freemail address: %s", addr)
	}

	return &EmailAddress{
		Local:    matches[1],
		Identity: matches[2],
		Domain:   FreemailDomain,
	}, nil
}

// String returns the full email address
func (e *EmailAddress) String() string {
	return fmt.Sprintf("%s@%s.%s", e.Local, e.Identity, e.Domain)
}

// IdentityBase64 returns the identity in Base64 format
func (e *EmailAddress) IdentityBase64() (string, error) {
	// Decode from Base32
	decoded, err := base32.StdEncoding.DecodeString(strings.ToUpper(e.Identity))
	if err != nil {
		return "", fmt.Errorf("invalid Base32 identity: %w", err)
	}
	// Encode to Base64
	return base64.StdEncoding.EncodeToString(decoded), nil
}

// NewEmailAddress creates an email address from components
func NewEmailAddress(local string, identityBase64 string) *EmailAddress {
	// Convert Base64 identity to Base32
	decoded, err := base64.StdEncoding.DecodeString(identityBase64)
	if err != nil {
		return nil
	}
	identity := strings.ToLower(base32.StdEncoding.EncodeToString(decoded))
	identity = strings.TrimRight(identity, "=")

	return &EmailAddress{
		Local:    local,
		Identity: identity,
		Domain:   FreemailDomain,
	}
}

// Header represents an email header
type Header struct {
	Name  string
	Value string
}

// Message represents an email message
type Message struct {
	mu sync.RWMutex

	// Identity
	UID       uint32 // IMAP UID
	MessageID string // Message-ID header

	// Envelope
	From        *EmailAddress
	To          []*EmailAddress
	CC          []*EmailAddress
	BCC         []*EmailAddress
	Subject     string
	Date        time.Time
	InReplyTo   string
	References  []string

	// Headers
	Headers []*Header

	// Content
	ContentType     string
	ContentEncoding string
	Body            []byte

	// MIME parts (for multipart messages)
	Parts []*MessagePart

	// Flags
	Flags MessageFlag

	// Metadata
	Size     int64
	Received time.Time
}

// MessagePart represents a MIME part
type MessagePart struct {
	ContentType        string
	ContentEncoding    string
	ContentDisposition string
	Filename           string
	Body               []byte
}

// NewMessage creates a new message
func NewMessage() *Message {
	return &Message{
		Date:     time.Now(),
		Received: time.Now(),
		Headers:  make([]*Header, 0),
		To:       make([]*EmailAddress, 0),
		CC:       make([]*EmailAddress, 0),
		BCC:      make([]*EmailAddress, 0),
		Parts:    make([]*MessagePart, 0),
	}
}

// AddHeader adds a header to the message
func (m *Message) AddHeader(name, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Headers = append(m.Headers, &Header{Name: name, Value: value})
}

// GetHeader returns the value of a header
func (m *Message) GetHeader(name string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	name = strings.ToLower(name)
	for _, h := range m.Headers {
		if strings.ToLower(h.Name) == name {
			return h.Value
		}
	}
	return ""
}

// SetFlag sets a message flag
func (m *Message) SetFlag(flag MessageFlag) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Flags |= flag
}

// ClearFlag clears a message flag
func (m *Message) ClearFlag(flag MessageFlag) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Flags &^= flag
}

// HasFlag checks if a flag is set
func (m *Message) HasFlag(flag MessageFlag) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Flags&flag != 0
}

// Folder represents a mailbox folder
type Folder struct {
	mu sync.RWMutex

	Name        string
	Path        string // Full path (e.g., "INBOX.Subfolder")
	UIDValidity uint32
	NextUID     uint32
	Messages    []*Message
	Subfolders  []*Folder
}

// NewFolder creates a new folder
func NewFolder(name string) *Folder {
	return &Folder{
		Name:        name,
		Path:        name,
		UIDValidity: uint32(time.Now().Unix()),
		NextUID:     1,
		Messages:    make([]*Message, 0),
		Subfolders:  make([]*Folder, 0),
	}
}

// AddMessage adds a message to the folder
func (f *Folder) AddMessage(msg *Message) uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()

	msg.UID = f.NextUID
	f.NextUID++
	f.Messages = append(f.Messages, msg)

	return msg.UID
}

// GetMessage returns a message by UID
func (f *Folder) GetMessage(uid uint32) *Message {
	f.mu.RLock()
	defer f.mu.RUnlock()

	for _, msg := range f.Messages {
		if msg.UID == uid {
			return msg
		}
	}
	return nil
}

// GetMessageBySeq returns a message by sequence number (1-based)
func (f *Folder) GetMessageBySeq(seq int) *Message {
	f.mu.RLock()
	defer f.mu.RUnlock()

	if seq < 1 || seq > len(f.Messages) {
		return nil
	}
	return f.Messages[seq-1]
}

// Count returns the number of messages
func (f *Folder) Count() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.Messages)
}

// Expunge removes messages marked as deleted
func (f *Folder) Expunge() []uint32 {
	f.mu.Lock()
	defer f.mu.Unlock()

	var expunged []uint32
	var remaining []*Message

	for _, msg := range f.Messages {
		if msg.HasFlag(FlagDeleted) {
			expunged = append(expunged, msg.UID)
		} else {
			remaining = append(remaining, msg)
		}
	}

	f.Messages = remaining
	return expunged
}

// AccountKeys holds the cryptographic keys for an account
type AccountKeys struct {
	// RSA keypair for signing and decryption
	PrivateKey *rsa.PrivateKey
	PublicKey  *rsa.PublicKey

	// RTS key for Ready-To-Send protocol
	RTSKey string

	// Mailsite URI
	MailsiteURI string
	MailsiteSlot int
}

// Account represents a Freemail account
type Account struct {
	mu sync.RWMutex

	// Identity
	ID          string // WoT identity (Base64)
	Nickname    string
	RequestURI  string
	InsertURI   string

	// Email
	EmailLocal string // Local part of email address

	// Authentication
	PasswordHash string // MD5 hash of password

	// Keys
	Keys *AccountKeys

	// Folders
	Inbox *Folder
	Sent  *Folder
	Trash *Folder
	Drafts *Folder

	// Custom folders
	Folders map[string]*Folder

	// Channels
	Channels map[string]*Channel

	// State
	LastLogin time.Time
	Created   time.Time
}

// NewAccount creates a new account
func NewAccount(id string, nickname string) *Account {
	return &Account{
		ID:       id,
		Nickname: nickname,
		Inbox:    NewFolder("INBOX"),
		Sent:     NewFolder("Sent"),
		Trash:    NewFolder("Trash"),
		Drafts:   NewFolder("Drafts"),
		Folders:  make(map[string]*Folder),
		Channels: make(map[string]*Channel),
		Created:  time.Now(),
	}
}

// GetEmailAddress returns the Freemail address for this account
func (a *Account) GetEmailAddress() *EmailAddress {
	local := a.EmailLocal
	if local == "" {
		local = a.Nickname
	}
	return NewEmailAddress(local, a.ID)
}

// GetFolder returns a folder by name
func (a *Account) GetFolder(name string) *Folder {
	a.mu.RLock()
	defer a.mu.RUnlock()

	name = strings.ToUpper(name)
	switch name {
	case "INBOX":
		return a.Inbox
	case "SENT":
		return a.Sent
	case "TRASH":
		return a.Trash
	case "DRAFTS":
		return a.Drafts
	default:
		return a.Folders[name]
	}
}

// CreateFolder creates a new folder
func (a *Account) CreateFolder(name string) (*Folder, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	name = strings.ToUpper(name)

	// Check if exists
	if _, exists := a.Folders[name]; exists {
		return nil, fmt.Errorf("folder already exists: %s", name)
	}

	// Reserved names
	reserved := map[string]bool{
		"INBOX": true, "SENT": true, "TRASH": true, "DRAFTS": true,
	}
	if reserved[name] {
		return nil, fmt.Errorf("cannot create reserved folder: %s", name)
	}

	folder := NewFolder(name)
	a.Folders[name] = folder

	return folder, nil
}

// DeleteFolder deletes a folder
func (a *Account) DeleteFolder(name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	name = strings.ToUpper(name)

	// Reserved names
	reserved := map[string]bool{
		"INBOX": true, "SENT": true, "TRASH": true, "DRAFTS": true,
	}
	if reserved[name] {
		return fmt.Errorf("cannot delete reserved folder: %s", name)
	}

	if _, exists := a.Folders[name]; !exists {
		return fmt.Errorf("folder not found: %s", name)
	}

	delete(a.Folders, name)
	return nil
}

// ListFolders returns all folder names
func (a *Account) ListFolders() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	folders := []string{"INBOX", "Sent", "Trash", "Drafts"}
	for name := range a.Folders {
		folders = append(folders, name)
	}
	return folders
}

// ChannelState represents the state of a channel
type ChannelState int

const (
	ChannelActive ChannelState = iota
	ChannelReadOnly
	ChannelInactive
)

// Channel represents a communication channel with another Freemail user
type Channel struct {
	mu sync.RWMutex

	// Identity
	ID              string // Channel identifier
	RemoteIdentity  string // Remote user's WoT identity
	RemoteNickname  string

	// Keys
	AESKey []byte // Symmetric key for this channel
	AESIV  []byte // Initialization vector

	// Slots
	SenderSlot   int
	ReceiverSlot int

	// State
	State       ChannelState
	CreatedAt   time.Time
	ExpiresAt   time.Time
	LastUsed    time.Time

	// Message queue
	Outbox []*OutgoingMessage
}

// OutgoingMessage represents a message waiting to be sent
type OutgoingMessage struct {
	ID          string
	RecipientID string
	Message     *Message
	Retries     int
	NextRetry   time.Time
	SentAt      time.Time
}

// NewChannel creates a new channel
func NewChannel(remoteIdentity string) *Channel {
	return &Channel{
		ID:             fmt.Sprintf("ch-%d", time.Now().UnixNano()),
		RemoteIdentity: remoteIdentity,
		State:          ChannelActive,
		CreatedAt:      time.Now(),
		ExpiresAt:      time.Now().Add(7 * 24 * time.Hour), // 7 days
		Outbox:         make([]*OutgoingMessage, 0),
	}
}

// IsExpired checks if the channel has expired
func (c *Channel) IsExpired() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return time.Now().After(c.ExpiresAt)
}

// QueueMessage adds a message to the outbox
func (c *Channel) QueueMessage(msg *Message, recipientID string) *OutgoingMessage {
	c.mu.Lock()
	defer c.mu.Unlock()

	outgoing := &OutgoingMessage{
		ID:          fmt.Sprintf("out-%d", time.Now().UnixNano()),
		RecipientID: recipientID,
		Message:     msg,
		NextRetry:   time.Now(),
	}

	c.Outbox = append(c.Outbox, outgoing)
	return outgoing
}

// RTSMessage represents a Ready-To-Send message
type RTSMessage struct {
	// Encrypted with recipient's public key
	EncryptedAESKey []byte

	// Encrypted with AES key
	EncryptedPayload []byte

	// Signature
	Signature []byte
}

// RTSPayload is the decrypted RTS payload
type RTSPayload struct {
	SenderMailsiteURI string
	SenderIdentity    string
	RecipientIdentity string
	InitiatorSlot     int
	ResponderSlot     int
	Timestamp         int64
}
