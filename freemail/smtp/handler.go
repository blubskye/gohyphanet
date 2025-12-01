// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package smtp

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"net/textproto"
	"strings"
	"time"

	"github.com/blubskye/gohyphanet/freemail"
)

// FreemailHandler handles messages for Freemail delivery
type FreemailHandler struct {
	// Account manager for looking up accounts
	accountManager *freemail.AccountManager

	// Transport manager for sending messages
	transportManager *freemail.TransportManager

	// Storage for saving outgoing messages
	storage *freemail.Storage
}

// NewFreemailHandler creates a new Freemail message handler
func NewFreemailHandler(am *freemail.AccountManager, tm *freemail.TransportManager, storage *freemail.Storage) *FreemailHandler {
	return &FreemailHandler{
		accountManager:   am,
		transportManager: tm,
		storage:          storage,
	}
}

// HandleMessage processes a submitted email message
func (h *FreemailHandler) HandleMessage(from string, to []string, data []byte) error {
	// Parse the email message
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to parse message: %w", err)
	}

	// Parse sender address
	fromAddr, err := freemail.ParseEmailAddress(from)
	if err != nil {
		return fmt.Errorf("invalid sender address: %w", err)
	}

	// Get sender's account
	account := h.accountManager.GetAccountByEmail(from)
	if account == nil {
		return fmt.Errorf("sender account not found: %s", from)
	}

	// Process each recipient
	for _, recipient := range to {
		if err := h.deliverToRecipient(account, fromAddr, recipient, msg, data); err != nil {
			return fmt.Errorf("failed to deliver to %s: %w", recipient, err)
		}
	}

	return nil
}

// deliverToRecipient delivers a message to a single recipient
func (h *FreemailHandler) deliverToRecipient(account *freemail.Account, fromAddr *freemail.EmailAddress, recipient string, msg *mail.Message, data []byte) error {
	// Parse recipient address
	toAddr, err := freemail.ParseEmailAddress(recipient)
	if err != nil {
		return fmt.Errorf("invalid recipient address: %w", err)
	}

	// Create Freemail message
	freemailMsg := h.parseMailMessage(msg, fromAddr, toAddr, data)

	// Store in sent folder
	account.Sent.AddMessage(freemailMsg)

	// Save to storage
	if h.storage != nil {
		h.storage.SaveMessage(account.ID, "Sent", freemailMsg)
	}

	// Get or create channel to recipient
	recipientID := toAddr.Identity
	channelID := h.findOrCreateChannel(account, recipientID)
	if channelID == "" {
		// No channel yet - queue for later delivery
		return h.queueForDelivery(account, freemailMsg, recipientID)
	}

	// Send via transport
	if h.transportManager != nil {
		body, err := SerializeMessage(freemailMsg)
		if err != nil {
			return fmt.Errorf("failed to serialize message: %w", err)
		}

		return h.transportManager.SendMessage(channelID, freemailMsg.Subject, body, account.ID, recipientID)
	}

	return nil
}

// findOrCreateChannel finds or initiates a channel to a recipient
func (h *FreemailHandler) findOrCreateChannel(account *freemail.Account, recipientID string) string {
	// Check existing channels
	for channelID, channel := range account.Channels {
		if channel.RemoteIdentity == recipientID && channel.State == freemail.ChannelActive {
			return channelID
		}
	}

	// No active channel found
	return ""
}

// queueForDelivery queues a message for later delivery
func (h *FreemailHandler) queueForDelivery(account *freemail.Account, msg *freemail.Message, recipientID string) error {
	// Create outgoing message
	outgoing := &freemail.OutgoingMessage{
		ID:          fmt.Sprintf("out-%d", time.Now().UnixNano()),
		RecipientID: recipientID,
		Message:     msg,
		NextRetry:   time.Now(),
	}

	// Find or create channel and queue
	for _, channel := range account.Channels {
		if channel.RemoteIdentity == recipientID {
			channel.Outbox = append(channel.Outbox, outgoing)
			return nil
		}
	}

	// Create new channel
	channel := freemail.NewChannel(recipientID)
	channel.Outbox = append(channel.Outbox, outgoing)
	account.Channels[channel.ID] = channel

	return nil
}

// parseMailMessage converts a mail.Message to a Freemail Message
func (h *FreemailHandler) parseMailMessage(msg *mail.Message, from, to *freemail.EmailAddress, rawData []byte) *freemail.Message {
	freemailMsg := freemail.NewMessage()

	// Set envelope
	freemailMsg.From = from
	freemailMsg.To = []*freemail.EmailAddress{to}
	freemailMsg.Subject = decodeHeader(msg.Header.Get("Subject"))
	freemailMsg.MessageID = msg.Header.Get("Message-ID")
	freemailMsg.InReplyTo = msg.Header.Get("In-Reply-To")

	// Parse date
	if dateStr := msg.Header.Get("Date"); dateStr != "" {
		if t, err := mail.ParseDate(dateStr); err == nil {
			freemailMsg.Date = t
		}
	}

	// Copy all headers
	for name, values := range msg.Header {
		for _, value := range values {
			freemailMsg.AddHeader(name, value)
		}
	}

	// Parse content type
	contentType := msg.Header.Get("Content-Type")
	freemailMsg.ContentType = contentType
	freemailMsg.ContentEncoding = msg.Header.Get("Content-Transfer-Encoding")

	// Read body
	body, err := io.ReadAll(msg.Body)
	if err == nil {
		// Check if multipart
		mediaType, params, err := mime.ParseMediaType(contentType)
		if err == nil && strings.HasPrefix(mediaType, "multipart/") {
			freemailMsg.Parts = h.parseMultipart(body, params["boundary"])
		} else {
			freemailMsg.Body = body
		}
	}

	// Calculate size
	freemailMsg.Size = int64(len(rawData))

	return freemailMsg
}

// parseMultipart parses multipart message parts
func (h *FreemailHandler) parseMultipart(body []byte, boundary string) []*freemail.MessagePart {
	var parts []*freemail.MessagePart

	mr := multipart.NewReader(bytes.NewReader(body), boundary)
	for {
		part, err := mr.NextPart()
		if err != nil {
			break
		}

		partBody, err := io.ReadAll(part)
		if err != nil {
			continue
		}

		msgPart := &freemail.MessagePart{
			ContentType:        part.Header.Get("Content-Type"),
			ContentEncoding:    part.Header.Get("Content-Transfer-Encoding"),
			ContentDisposition: part.Header.Get("Content-Disposition"),
			Body:               partBody,
		}

		// Extract filename if present
		if cd := part.Header.Get("Content-Disposition"); cd != "" {
			_, params, err := mime.ParseMediaType(cd)
			if err == nil {
				msgPart.Filename = params["filename"]
			}
		}

		parts = append(parts, msgPart)
	}

	return parts
}

// FreemailAuthenticator authenticates users against Freemail accounts
type FreemailAuthenticator struct {
	accountManager *freemail.AccountManager
}

// NewFreemailAuthenticator creates a new Freemail authenticator
func NewFreemailAuthenticator(am *freemail.AccountManager) *FreemailAuthenticator {
	return &FreemailAuthenticator{
		accountManager: am,
	}
}

// Authenticate validates username and password
func (a *FreemailAuthenticator) Authenticate(username, password string) (bool, error) {
	_, valid := a.accountManager.Authenticate(username, password)
	return valid, nil
}

// GetAccountID returns the account ID for a username
func (a *FreemailAuthenticator) GetAccountID(username string) string {
	account := a.accountManager.GetAccountByEmail(username)
	if account != nil {
		return account.ID
	}
	return ""
}

// decodeHeader decodes a potentially encoded email header
func decodeHeader(s string) string {
	dec := new(mime.WordDecoder)
	decoded, err := dec.DecodeHeader(s)
	if err != nil {
		return s
	}
	return decoded
}

// encodeBase64 encodes a string to base64
func encodeBase64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

// decodeBase64 decodes a base64 string
func decodeBase64(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// SerializeMessage converts a message to bytes for sending
func SerializeMessage(m *freemail.Message) ([]byte, error) {
	var buf bytes.Buffer

	// Write headers
	if m.From != nil {
		fmt.Fprintf(&buf, "From: %s\r\n", m.From.String())
	}
	for _, to := range m.To {
		fmt.Fprintf(&buf, "To: %s\r\n", to.String())
	}
	for _, cc := range m.CC {
		fmt.Fprintf(&buf, "Cc: %s\r\n", cc.String())
	}
	if m.Subject != "" {
		fmt.Fprintf(&buf, "Subject: %s\r\n", encodeHeaderValue(m.Subject))
	}
	if m.MessageID != "" {
		fmt.Fprintf(&buf, "Message-ID: %s\r\n", m.MessageID)
	}
	if !m.Date.IsZero() {
		fmt.Fprintf(&buf, "Date: %s\r\n", m.Date.Format(time.RFC1123Z))
	}
	if m.InReplyTo != "" {
		fmt.Fprintf(&buf, "In-Reply-To: %s\r\n", m.InReplyTo)
	}

	// Write custom headers
	for _, h := range m.Headers {
		// Skip headers we've already written
		name := strings.ToLower(h.Name)
		if name == "from" || name == "to" || name == "cc" || name == "subject" ||
			name == "message-id" || name == "date" || name == "in-reply-to" {
			continue
		}
		fmt.Fprintf(&buf, "%s: %s\r\n", h.Name, h.Value)
	}

	// Content type
	if m.ContentType != "" {
		fmt.Fprintf(&buf, "Content-Type: %s\r\n", m.ContentType)
	}
	if m.ContentEncoding != "" {
		fmt.Fprintf(&buf, "Content-Transfer-Encoding: %s\r\n", m.ContentEncoding)
	}

	// End headers
	buf.WriteString("\r\n")

	// Write body
	if len(m.Parts) > 0 {
		// Multipart message
		boundary := generateBoundary()
		for _, part := range m.Parts {
			fmt.Fprintf(&buf, "--%s\r\n", boundary)
			if part.ContentType != "" {
				fmt.Fprintf(&buf, "Content-Type: %s\r\n", part.ContentType)
			}
			if part.ContentEncoding != "" {
				fmt.Fprintf(&buf, "Content-Transfer-Encoding: %s\r\n", part.ContentEncoding)
			}
			if part.ContentDisposition != "" {
				fmt.Fprintf(&buf, "Content-Disposition: %s\r\n", part.ContentDisposition)
			}
			buf.WriteString("\r\n")
			buf.Write(part.Body)
			buf.WriteString("\r\n")
		}
		fmt.Fprintf(&buf, "--%s--\r\n", boundary)
	} else {
		buf.Write(m.Body)
	}

	return buf.Bytes(), nil
}

// encodeHeaderValue encodes a header value if necessary
func encodeHeaderValue(s string) string {
	// Check if encoding is needed
	needsEncoding := false
	for _, r := range s {
		if r > 127 {
			needsEncoding = true
			break
		}
	}

	if !needsEncoding {
		return s
	}

	// Use RFC 2047 encoding
	return mime.QEncoding.Encode("utf-8", s)
}

// generateBoundary generates a MIME boundary
func generateBoundary() string {
	return fmt.Sprintf("----=_Part_%d", time.Now().UnixNano())
}

// ParseHeaders parses raw headers into a textproto.MIMEHeader
func ParseHeaders(data []byte) (textproto.MIMEHeader, []byte, error) {
	reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(data)))
	headers, err := reader.ReadMIMEHeader()
	if err != nil && err != io.EOF {
		return nil, nil, err
	}

	// Find end of headers (blank line)
	idx := bytes.Index(data, []byte("\r\n\r\n"))
	if idx < 0 {
		idx = bytes.Index(data, []byte("\n\n"))
		if idx < 0 {
			return headers, nil, nil
		}
		return headers, data[idx+2:], nil
	}
	return headers, data[idx+4:], nil
}
