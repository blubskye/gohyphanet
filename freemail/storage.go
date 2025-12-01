// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package freemail

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Storage provides persistent storage for Freemail
type Storage struct {
	mu      sync.RWMutex
	dataDir string
}

// NewStorage creates a new storage instance
func NewStorage(dataDir string) *Storage {
	return &Storage{
		dataDir: dataDir,
	}
}

// Initialize creates the necessary directories
func (s *Storage) Initialize() error {
	dirs := []string{
		s.dataDir,
		filepath.Join(s.dataDir, "accounts"),
		filepath.Join(s.dataDir, "temp"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// AccountStorage handles account persistence
type AccountStorage struct {
	storage *Storage
	account *Account
	dir     string
}

// GetAccountStorage returns storage for a specific account
func (s *Storage) GetAccountStorage(accountID string) *AccountStorage {
	dir := filepath.Join(s.dataDir, "accounts", accountID)
	return &AccountStorage{
		storage: s,
		dir:     dir,
	}
}

// Initialize creates account directories
func (as *AccountStorage) Initialize() error {
	dirs := []string{
		as.dir,
		filepath.Join(as.dir, "messages"),
		filepath.Join(as.dir, "messages", "inbox"),
		filepath.Join(as.dir, "messages", "sent"),
		filepath.Join(as.dir, "messages", "trash"),
		filepath.Join(as.dir, "messages", "drafts"),
		filepath.Join(as.dir, "outbox"),
		filepath.Join(as.dir, "channels"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	return nil
}

// accountProps holds serializable account properties
type accountProps struct {
	ID           string    `json:"id"`
	Nickname     string    `json:"nickname"`
	EmailLocal   string    `json:"email_local"`
	RequestURI   string    `json:"request_uri"`
	InsertURI    string    `json:"insert_uri"`
	PasswordHash string    `json:"password_hash"`
	RTSKey       string    `json:"rts_key"`
	MailsiteURI  string    `json:"mailsite_uri"`
	MailsiteSlot int       `json:"mailsite_slot"`
	LastLogin    time.Time `json:"last_login"`
	Created      time.Time `json:"created"`
}

// SaveAccount persists account properties
func (as *AccountStorage) SaveAccount(account *Account) error {
	account.mu.RLock()
	props := accountProps{
		ID:           account.ID,
		Nickname:     account.Nickname,
		EmailLocal:   account.EmailLocal,
		RequestURI:   account.RequestURI,
		InsertURI:    account.InsertURI,
		PasswordHash: account.PasswordHash,
		LastLogin:    account.LastLogin,
		Created:      account.Created,
	}

	if account.Keys != nil {
		props.RTSKey = account.Keys.RTSKey
		props.MailsiteURI = account.Keys.MailsiteURI
		props.MailsiteSlot = account.Keys.MailsiteSlot
	}
	account.mu.RUnlock()

	data, err := json.MarshalIndent(props, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal account: %w", err)
	}

	propsPath := filepath.Join(as.dir, "account.json")
	if err := os.WriteFile(propsPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write account file: %w", err)
	}

	return nil
}

// LoadAccount loads account properties
func (as *AccountStorage) LoadAccount() (*Account, error) {
	propsPath := filepath.Join(as.dir, "account.json")

	data, err := os.ReadFile(propsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read account file: %w", err)
	}

	var props accountProps
	if err := json.Unmarshal(data, &props); err != nil {
		return nil, fmt.Errorf("failed to unmarshal account: %w", err)
	}

	account := NewAccount(props.ID, props.Nickname)
	account.EmailLocal = props.EmailLocal
	account.RequestURI = props.RequestURI
	account.InsertURI = props.InsertURI
	account.PasswordHash = props.PasswordHash
	account.LastLogin = props.LastLogin
	account.Created = props.Created

	account.Keys = &AccountKeys{
		RTSKey:       props.RTSKey,
		MailsiteURI:  props.MailsiteURI,
		MailsiteSlot: props.MailsiteSlot,
	}

	// Load messages
	if err := as.loadFolder(account.Inbox, "inbox"); err != nil {
		return nil, fmt.Errorf("failed to load inbox: %w", err)
	}
	if err := as.loadFolder(account.Sent, "sent"); err != nil {
		return nil, fmt.Errorf("failed to load sent: %w", err)
	}
	if err := as.loadFolder(account.Trash, "trash"); err != nil {
		return nil, fmt.Errorf("failed to load trash: %w", err)
	}
	if err := as.loadFolder(account.Drafts, "drafts"); err != nil {
		return nil, fmt.Errorf("failed to load drafts: %w", err)
	}

	// Load channels
	if err := as.loadChannels(account); err != nil {
		return nil, fmt.Errorf("failed to load channels: %w", err)
	}

	as.account = account
	return account, nil
}

// loadFolder loads messages into a folder
func (as *AccountStorage) loadFolder(folder *Folder, name string) error {
	folderDir := filepath.Join(as.dir, "messages", name)

	// Load UID validity
	validityPath := filepath.Join(folderDir, ".uidvalidity")
	if data, err := os.ReadFile(validityPath); err == nil {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 32); err == nil {
			folder.UIDValidity = uint32(val)
		}
	}

	// Load next UID
	nextIDPath := filepath.Join(folderDir, ".nextid")
	if data, err := os.ReadFile(nextIDPath); err == nil {
		if val, err := strconv.ParseUint(strings.TrimSpace(string(data)), 10, 32); err == nil {
			folder.NextUID = uint32(val)
		}
	}

	// Load messages
	entries, err := os.ReadDir(folderDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		// Parse UID from filename
		uid, err := strconv.ParseUint(entry.Name(), 10, 32)
		if err != nil {
			continue
		}

		msgPath := filepath.Join(folderDir, entry.Name())
		msg, err := as.loadMessage(msgPath)
		if err != nil {
			continue
		}

		msg.UID = uint32(uid)
		folder.Messages = append(folder.Messages, msg)
	}

	// Sort by UID
	sort.Slice(folder.Messages, func(i, j int) bool {
		return folder.Messages[i].UID < folder.Messages[j].UID
	})

	return nil
}

// loadMessage loads a single message from file
func (as *AccountStorage) loadMessage(path string) (*Message, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	msg := NewMessage()

	reader := bufio.NewReader(file)

	// Read headers
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}

		line = strings.TrimRight(line, "\r\n")

		// Empty line marks end of headers
		if line == "" {
			break
		}

		// Parse header
		colonIdx := strings.Index(line, ":")
		if colonIdx > 0 {
			name := strings.TrimSpace(line[:colonIdx])
			value := strings.TrimSpace(line[colonIdx+1:])

			msg.AddHeader(name, value)

			// Parse known headers
			switch strings.ToLower(name) {
			case "from":
				if addr, err := ParseEmailAddress(value); err == nil {
					msg.From = addr
				}
			case "to":
				for _, addrStr := range strings.Split(value, ",") {
					if addr, err := ParseEmailAddress(strings.TrimSpace(addrStr)); err == nil {
						msg.To = append(msg.To, addr)
					}
				}
			case "subject":
				msg.Subject = value
			case "date":
				if t, err := parseDate(value); err == nil {
					msg.Date = t
				}
			case "message-id":
				msg.MessageID = value
			case "content-type":
				msg.ContentType = value
			case "content-transfer-encoding":
				msg.ContentEncoding = value
			}
		}
	}

	// Read body
	var body bytes.Buffer
	io.Copy(&body, reader)
	msg.Body = body.Bytes()
	msg.Size = int64(len(msg.Body))

	// Get file info for received time
	if info, err := os.Stat(path); err == nil {
		msg.Received = info.ModTime()
	}

	return msg, nil
}

// SaveMessage saves a message to a folder
func (as *AccountStorage) SaveMessage(folder *Folder, msg *Message) error {
	folderDir := as.getFolderDir(folder.Name)

	// Assign UID if needed
	if msg.UID == 0 {
		msg.UID = folder.AddMessage(msg)
	}

	msgPath := filepath.Join(folderDir, fmt.Sprintf("%d", msg.UID))

	file, err := os.Create(msgPath)
	if err != nil {
		return fmt.Errorf("failed to create message file: %w", err)
	}
	defer file.Close()

	// Write headers
	for _, h := range msg.Headers {
		fmt.Fprintf(file, "%s: %s\r\n", h.Name, h.Value)
	}

	// Empty line between headers and body
	file.WriteString("\r\n")

	// Write body
	file.Write(msg.Body)

	// Update next ID
	nextIDPath := filepath.Join(folderDir, ".nextid")
	os.WriteFile(nextIDPath, []byte(fmt.Sprintf("%d", folder.NextUID)), 0600)

	return nil
}

// DeleteMessage deletes a message from storage
func (as *AccountStorage) DeleteMessage(folder *Folder, uid uint32) error {
	folderDir := as.getFolderDir(folder.Name)
	msgPath := filepath.Join(folderDir, fmt.Sprintf("%d", uid))
	return os.Remove(msgPath)
}

func (as *AccountStorage) getFolderDir(name string) string {
	name = strings.ToLower(name)
	return filepath.Join(as.dir, "messages", name)
}

// SaveFolder persists folder metadata
func (as *AccountStorage) SaveFolder(folder *Folder) error {
	folderDir := as.getFolderDir(folder.Name)

	if err := os.MkdirAll(folderDir, 0700); err != nil {
		return err
	}

	// Save UID validity
	validityPath := filepath.Join(folderDir, ".uidvalidity")
	os.WriteFile(validityPath, []byte(fmt.Sprintf("%d", folder.UIDValidity)), 0600)

	// Save next UID
	nextIDPath := filepath.Join(folderDir, ".nextid")
	os.WriteFile(nextIDPath, []byte(fmt.Sprintf("%d", folder.NextUID)), 0600)

	return nil
}

// channelProps holds serializable channel properties
type channelProps struct {
	ID             string       `json:"id"`
	RemoteIdentity string       `json:"remote_identity"`
	RemoteNickname string       `json:"remote_nickname"`
	AESKey         []byte       `json:"aes_key"`
	AESIV          []byte       `json:"aes_iv"`
	SenderSlot     int          `json:"sender_slot"`
	ReceiverSlot   int          `json:"receiver_slot"`
	State          ChannelState `json:"state"`
	CreatedAt      time.Time    `json:"created_at"`
	ExpiresAt      time.Time    `json:"expires_at"`
	LastUsed       time.Time    `json:"last_used"`
}

// SaveChannel persists a channel
func (as *AccountStorage) SaveChannel(channel *Channel) error {
	channel.mu.RLock()
	props := channelProps{
		ID:             channel.ID,
		RemoteIdentity: channel.RemoteIdentity,
		RemoteNickname: channel.RemoteNickname,
		AESKey:         channel.AESKey,
		AESIV:          channel.AESIV,
		SenderSlot:     channel.SenderSlot,
		ReceiverSlot:   channel.ReceiverSlot,
		State:          channel.State,
		CreatedAt:      channel.CreatedAt,
		ExpiresAt:      channel.ExpiresAt,
		LastUsed:       channel.LastUsed,
	}
	channel.mu.RUnlock()

	data, err := json.MarshalIndent(props, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal channel: %w", err)
	}

	channelPath := filepath.Join(as.dir, "channels", channel.ID+".json")
	if err := os.WriteFile(channelPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write channel file: %w", err)
	}

	return nil
}

// loadChannels loads all channels for an account
func (as *AccountStorage) loadChannels(account *Account) error {
	channelsDir := filepath.Join(as.dir, "channels")

	entries, err := os.ReadDir(channelsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		channelPath := filepath.Join(channelsDir, entry.Name())
		data, err := os.ReadFile(channelPath)
		if err != nil {
			continue
		}

		var props channelProps
		if err := json.Unmarshal(data, &props); err != nil {
			continue
		}

		channel := &Channel{
			ID:             props.ID,
			RemoteIdentity: props.RemoteIdentity,
			RemoteNickname: props.RemoteNickname,
			AESKey:         props.AESKey,
			AESIV:          props.AESIV,
			SenderSlot:     props.SenderSlot,
			ReceiverSlot:   props.ReceiverSlot,
			State:          props.State,
			CreatedAt:      props.CreatedAt,
			ExpiresAt:      props.ExpiresAt,
			LastUsed:       props.LastUsed,
			Outbox:         make([]*OutgoingMessage, 0),
		}

		account.Channels[channel.RemoteIdentity] = channel
	}

	return nil
}

// OutboxStorage handles outgoing message persistence
type OutboxStorage struct {
	accountStorage *AccountStorage
}

// GetOutboxStorage returns outbox storage for an account
func (as *AccountStorage) GetOutboxStorage() *OutboxStorage {
	return &OutboxStorage{accountStorage: as}
}

// SaveOutgoingMessage saves an outgoing message
func (obs *OutboxStorage) SaveOutgoingMessage(msg *OutgoingMessage) error {
	outboxDir := filepath.Join(obs.accountStorage.dir, "outbox", msg.RecipientID)

	if err := os.MkdirAll(outboxDir, 0700); err != nil {
		return err
	}

	// Save message
	msgPath := filepath.Join(outboxDir, msg.ID+".json")

	type outgoingProps struct {
		ID          string    `json:"id"`
		RecipientID string    `json:"recipient_id"`
		Subject     string    `json:"subject"`
		Body        []byte    `json:"body"`
		Retries     int       `json:"retries"`
		NextRetry   time.Time `json:"next_retry"`
		SentAt      time.Time `json:"sent_at"`
	}

	props := outgoingProps{
		ID:          msg.ID,
		RecipientID: msg.RecipientID,
		Subject:     msg.Message.Subject,
		Body:        msg.Message.Body,
		Retries:     msg.Retries,
		NextRetry:   msg.NextRetry,
		SentAt:      msg.SentAt,
	}

	data, err := json.MarshalIndent(props, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(msgPath, data, 0600)
}

// DeleteOutgoingMessage removes an outgoing message
func (obs *OutboxStorage) DeleteOutgoingMessage(recipientID, msgID string) error {
	msgPath := filepath.Join(obs.accountStorage.dir, "outbox", recipientID, msgID+".json")
	return os.Remove(msgPath)
}

// parseDate parses common email date formats
func parseDate(s string) (time.Time, error) {
	formats := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
		"Mon, 2 Jan 2006 15:04:05 -0700",
		"2 Jan 2006 15:04:05 -0700",
		"Mon, 2 Jan 2006 15:04:05 MST",
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", s)
}

// AccountManager manages multiple accounts
type AccountManager struct {
	mu       sync.RWMutex
	storage  *Storage
	accounts map[string]*Account
}

// NewAccountManager creates a new account manager
func NewAccountManager(storage *Storage) *AccountManager {
	return &AccountManager{
		storage:  storage,
		accounts: make(map[string]*Account),
	}
}

// LoadAccounts loads all accounts from storage
func (am *AccountManager) LoadAccounts() error {
	accountsDir := filepath.Join(am.storage.dataDir, "accounts")

	entries, err := os.ReadDir(accountsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		accountID := entry.Name()
		as := am.storage.GetAccountStorage(accountID)

		account, err := as.LoadAccount()
		if err != nil {
			continue
		}

		am.mu.Lock()
		am.accounts[accountID] = account
		am.mu.Unlock()
	}

	return nil
}

// GetAccount returns an account by ID
func (am *AccountManager) GetAccount(id string) *Account {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return am.accounts[id]
}

// GetAccounts returns all accounts
func (am *AccountManager) GetAccounts() []*Account {
	am.mu.RLock()
	defer am.mu.RUnlock()

	accounts := make([]*Account, 0, len(am.accounts))
	for _, acc := range am.accounts {
		accounts = append(accounts, acc)
	}
	return accounts
}

// CreateAccount creates a new account
func (am *AccountManager) CreateAccount(id, nickname, password string) (*Account, error) {
	am.mu.Lock()
	defer am.mu.Unlock()

	if _, exists := am.accounts[id]; exists {
		return nil, fmt.Errorf("account already exists: %s", id)
	}

	account := NewAccount(id, nickname)
	account.PasswordHash = hashPassword(password)

	// Initialize storage
	as := am.storage.GetAccountStorage(id)
	if err := as.Initialize(); err != nil {
		return nil, err
	}

	if err := as.SaveAccount(account); err != nil {
		return nil, err
	}

	// Save folder metadata
	as.SaveFolder(account.Inbox)
	as.SaveFolder(account.Sent)
	as.SaveFolder(account.Trash)
	as.SaveFolder(account.Drafts)

	am.accounts[id] = account
	return account, nil
}

// Authenticate verifies credentials
func (am *AccountManager) Authenticate(id, password string) (*Account, bool) {
	am.mu.RLock()
	account := am.accounts[id]
	am.mu.RUnlock()

	if account == nil {
		return nil, false
	}

	if account.PasswordHash != hashPassword(password) {
		return nil, false
	}

	account.mu.Lock()
	account.LastLogin = time.Now()
	account.mu.Unlock()

	return account, true
}

// hashPassword hashes a password (using MD5 for compatibility)
func hashPassword(password string) string {
	// Note: MD5 is used for compatibility with Java Freemail
	// In a new system, we'd use bcrypt or argon2
	hash := md5.Sum([]byte(password))
	return fmt.Sprintf("%x", hash)
}

// GetAccountByEmail finds an account by email address
func (am *AccountManager) GetAccountByEmail(email string) *Account {
	am.mu.RLock()
	defer am.mu.RUnlock()

	// Parse the email to extract identity
	addr, err := ParseEmailAddress(email)
	if err != nil {
		// Try matching by ID or nickname
		for _, acc := range am.accounts {
			if acc.ID == email || acc.Nickname == email || acc.EmailLocal == email {
				return acc
			}
		}
		return nil
	}

	// Match by identity
	for _, acc := range am.accounts {
		accAddr := acc.GetEmailAddress()
		if accAddr != nil && accAddr.Identity == addr.Identity {
			return acc
		}
	}

	return nil
}

// SaveMessage saves a message to an account's folder (convenience method on Storage)
func (s *Storage) SaveMessage(accountID, folderName string, msg *Message) error {
	as := s.GetAccountStorage(accountID)
	account, err := as.LoadAccount()
	if err != nil {
		return err
	}

	folder := account.GetFolder(folderName)
	if folder == nil {
		return fmt.Errorf("folder not found: %s", folderName)
	}

	return as.SaveMessage(folder, msg)
}
