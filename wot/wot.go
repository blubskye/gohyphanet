// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

// Package wot provides a client for the Web of Trust (WoT) Freenet plugin.
// WoT is an identity and trust management system used by applications like Sone.
package wot

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/blubskye/gohyphanet/fcp"
)

// Identity represents a WoT identity (remote or local)
type Identity struct {
	ID                    string            // 43-char base64 routing key
	Nickname              string            // Display name (max 30 chars)
	RequestURI            string            // Freenet URI for fetching
	InsertURI             string            // Only for OwnIdentity
	Contexts              []string          // Context labels (e.g., "Sone")
	Properties            map[string]string // Custom key-value properties
	PublishesTrustList    bool              // Whether identity publishes trust
	CurrentEditionFetchState string         // NotFetched, ParsingFailed, Fetched
	IsOwn                 bool              // True if this is a local identity
}

// Trust represents a trust relationship between two identities
type Trust struct {
	TrusterID string // Identity giving trust
	TrusteeID string // Identity receiving trust
	Value     int8   // -100 to +100
	Comment   string // Trust comment
}

// Score represents a computed reputation score
type Score struct {
	TrusterID string // OwnIdentity computing the score
	TrusteeID string // Identity being scored
	Value     int    // Computed score value
	Rank      int    // Distance from trust tree root
	Capacity  int    // Trust points identity can assign
}

// Client provides access to the Web of Trust plugin via FCP
type Client struct {
	fcp           *fcp.Client
	mu            sync.Mutex
	msgCounter    uint64
	pendingReqs   map[string]chan *fcp.Message
	pendingLock   sync.Mutex
	subscriptions map[string]*Subscription
	subLock       sync.RWMutex
	eventChan     chan Event
	listening     bool
	listenErr     error
}

// Subscription represents an active WoT subscription
type Subscription struct {
	ID   string
	Type string // "Identities", "Trusts", or "Scores"
}

// Event types for subscription notifications
type EventType int

const (
	EventIdentityAdded EventType = iota
	EventIdentityUpdated
	EventIdentityRemoved
	EventTrustAdded
	EventTrustUpdated
	EventTrustRemoved
	EventScoreUpdated
	EventSyncBegin
	EventSyncEnd
)

// Event represents a WoT subscription event
type Event struct {
	Type     EventType
	Identity *Identity
	Trust    *Trust
	Score    *Score
}

// NewClient creates a new WoT client using an existing FCP connection
func NewClient(fcpClient *fcp.Client) *Client {
	return &Client{
		fcp:           fcpClient,
		pendingReqs:   make(map[string]chan *fcp.Message),
		subscriptions: make(map[string]*Subscription),
		eventChan:     make(chan Event, 100),
	}
}

// Connect creates a new FCP connection and WoT client
func Connect(config *fcp.Config) (*Client, error) {
	if config == nil {
		config = fcp.DefaultConfig()
	}

	fcpClient, err := fcp.Connect(config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Freenet: %w", err)
	}

	return NewClient(fcpClient), nil
}

// Close closes the WoT client and underlying FCP connection
func (c *Client) Close() error {
	return c.fcp.Close()
}

// Events returns the channel for receiving subscription events
func (c *Client) Events() <-chan Event {
	return c.eventChan
}

// generateID creates a unique message identifier
func (c *Client) generateID() string {
	n := atomic.AddUint64(&c.msgCounter, 1)
	return fmt.Sprintf("wot-%d", n)
}

// sendAndWait sends a message and waits for response
func (c *Client) sendAndWait(msg *fcp.Message) (*fcp.Message, error) {
	id := msg.Fields["Identifier"]
	if id == "" {
		id = c.generateID()
		msg.Fields["Identifier"] = id
	}

	// Create response channel
	respChan := make(chan *fcp.Message, 1)
	c.pendingLock.Lock()
	c.pendingReqs[id] = respChan
	c.pendingLock.Unlock()

	defer func() {
		c.pendingLock.Lock()
		delete(c.pendingReqs, id)
		c.pendingLock.Unlock()
	}()

	// Send the message
	if err := c.fcp.SendMessage(msg); err != nil {
		return nil, err
	}

	// Wait for response
	resp := <-respChan

	// Check for error
	if resp.Name == "Error" {
		return nil, fmt.Errorf("WoT error: %s - %s",
			resp.Fields["Code"], resp.Fields["Description"])
	}

	return resp, nil
}

// StartListening starts the background listener for FCP messages
func (c *Client) StartListening() {
	if c.listening {
		return
	}
	c.listening = true

	go func() {
		for c.listening {
			msg, err := c.fcp.ReceiveMessage()
			if err != nil {
				c.listenErr = err
				c.listening = false
				return
			}

			c.handleMessage(msg)
		}
	}()
}

// handleMessage processes incoming FCP messages
func (c *Client) handleMessage(msg *fcp.Message) {
	// Check if this is a response to a pending request
	id := msg.Fields["Identifier"]
	if id != "" {
		c.pendingLock.Lock()
		if ch, ok := c.pendingReqs[id]; ok {
			ch <- msg
			c.pendingLock.Unlock()
			return
		}
		c.pendingLock.Unlock()
	}

	// Handle subscription events
	switch msg.Name {
	case "BeginSynchronizationEvent":
		c.eventChan <- Event{Type: EventSyncBegin}
	case "EndSynchronizationEvent":
		c.eventChan <- Event{Type: EventSyncEnd}
	case "ObjectChangedEvent":
		c.handleObjectChanged(msg)
	}
}

// handleObjectChanged processes subscription update events
func (c *Client) handleObjectChanged(msg *fcp.Message) {
	subType := msg.Fields["SubscriptionType"]

	switch subType {
	case "Identities":
		// Parse identity from After fields
		identity := parseIdentityFromFields(msg.Fields, "After.")
		if identity != nil {
			c.eventChan <- Event{
				Type:     EventIdentityUpdated,
				Identity: identity,
			}
		}
	case "Trusts":
		trust := parseTrustFromFields(msg.Fields, "After.")
		if trust != nil {
			c.eventChan <- Event{
				Type:  EventTrustUpdated,
				Trust: trust,
			}
		}
	case "Scores":
		score := parseScoreFromFields(msg.Fields, "After.")
		if score != nil {
			c.eventChan <- Event{
				Type:  EventScoreUpdated,
				Score: score,
			}
		}
	}
}

// Ping tests the connection to WoT
func (c *Client) Ping() error {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "Ping",
		},
	}

	resp, err := c.sendAndWait(msg)
	if err != nil {
		return err
	}

	if resp.Fields["Message"] != "Pong" {
		return fmt.Errorf("unexpected response: %s", resp.Fields["Message"])
	}

	return nil
}

// GetOwnIdentities retrieves all local identities
func (c *Client) GetOwnIdentities() ([]*Identity, error) {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "GetOwnIdentities",
		},
	}

	resp, err := c.sendAndWait(msg)
	if err != nil {
		return nil, err
	}

	return parseOwnIdentitiesResponse(resp), nil
}

// GetIdentitiesByScore retrieves identities filtered by score
func (c *Client) GetIdentitiesByScore(trusterID string, selection string, context string) ([]*Identity, error) {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "GetIdentitiesByScore",
			"Selection":  selection, // "+", "-", or "0"
			"Context":    context,
		},
	}

	if trusterID != "" {
		msg.Fields["Truster"] = trusterID
	}

	resp, err := c.sendAndWait(msg)
	if err != nil {
		return nil, err
	}

	return parseIdentitiesResponse(resp), nil
}

// GetIdentity retrieves a specific identity
func (c *Client) GetIdentity(identityID string, trusterID string) (*Identity, error) {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "GetIdentity",
			"Identity":   identityID,
		},
	}

	if trusterID != "" {
		msg.Fields["Truster"] = trusterID
	}

	resp, err := c.sendAndWait(msg)
	if err != nil {
		return nil, err
	}

	return parseIdentityFromFields(resp.Fields, ""), nil
}

// AddIdentity adds a remote identity to track
func (c *Client) AddIdentity(requestURI string) (*Identity, error) {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "AddIdentity",
			"RequestURI": requestURI,
		},
	}

	resp, err := c.sendAndWait(msg)
	if err != nil {
		return nil, err
	}

	return &Identity{
		ID:       resp.Fields["ID"],
		Nickname: resp.Fields["Nickname"],
	}, nil
}

// SetTrust sets trust from one identity to another
func (c *Client) SetTrust(trusterID, trusteeID string, value int8, comment string) error {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "SetTrust",
			"Truster":    trusterID,
			"Trustee":    trusteeID,
			"Value":      strconv.Itoa(int(value)),
			"Comment":    comment,
		},
	}

	_, err := c.sendAndWait(msg)
	return err
}

// RemoveTrust removes a trust relationship
func (c *Client) RemoveTrust(trusterID, trusteeID string) error {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "RemoveTrust",
			"Truster":    trusterID,
			"Trustee":    trusteeID,
		},
	}

	_, err := c.sendAndWait(msg)
	return err
}

// GetTrust retrieves trust between two identities
func (c *Client) GetTrust(trusterID, trusteeID string) (*Trust, error) {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "GetTrust",
			"Truster":    trusterID,
			"Trustee":    trusteeID,
		},
	}

	resp, err := c.sendAndWait(msg)
	if err != nil {
		return nil, err
	}

	return parseTrustFromFields(resp.Fields, "Trusts.0."), nil
}

// GetScore retrieves computed score for an identity
func (c *Client) GetScore(trusterID, trusteeID string) (*Score, error) {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "GetScore",
			"Truster":    trusterID,
			"Trustee":    trusteeID,
		},
	}

	resp, err := c.sendAndWait(msg)
	if err != nil {
		return nil, err
	}

	return parseScoreFromFields(resp.Fields, "Scores.0."), nil
}

// AddContext adds a context to an identity
func (c *Client) AddContext(identityID, context string) error {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "AddContext",
			"Identity":   identityID,
			"Context":    context,
		},
	}

	_, err := c.sendAndWait(msg)
	return err
}

// RemoveContext removes a context from an identity
func (c *Client) RemoveContext(identityID, context string) error {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "RemoveContext",
			"Identity":   identityID,
			"Context":    context,
		},
	}

	_, err := c.sendAndWait(msg)
	return err
}

// SetProperty sets a property on an identity
func (c *Client) SetProperty(identityID, property, value string) error {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "SetProperty",
			"Identity":   identityID,
			"Property":   property,
			"Value":      value,
		},
	}

	_, err := c.sendAndWait(msg)
	return err
}

// GetProperty gets a property from an identity
func (c *Client) GetProperty(identityID, property string) (string, error) {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "GetProperty",
			"Identity":   identityID,
			"Property":   property,
		},
	}

	resp, err := c.sendAndWait(msg)
	if err != nil {
		return "", err
	}

	return resp.Fields["Property"], nil
}

// RemoveProperty removes a property from an identity
func (c *Client) RemoveProperty(identityID, property string) error {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "RemoveProperty",
			"Identity":   identityID,
			"Property":   property,
		},
	}

	_, err := c.sendAndWait(msg)
	return err
}

// Subscribe creates a subscription for real-time updates
func (c *Client) Subscribe(subscriptionType string) (*Subscription, error) {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "Subscribe",
			"To":         subscriptionType, // "Identities", "Trusts", or "Scores"
		},
	}

	resp, err := c.sendAndWait(msg)
	if err != nil {
		return nil, err
	}

	sub := &Subscription{
		ID:   resp.Fields["SubscriptionID"],
		Type: subscriptionType,
	}

	c.subLock.Lock()
	c.subscriptions[sub.ID] = sub
	c.subLock.Unlock()

	return sub, nil
}

// Unsubscribe cancels a subscription
func (c *Client) Unsubscribe(subscriptionID string) error {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "Unsubscribe",
			"SubscriptionID": subscriptionID,
		},
	}

	_, err := c.sendAndWait(msg)
	if err != nil {
		return err
	}

	c.subLock.Lock()
	delete(c.subscriptions, subscriptionID)
	c.subLock.Unlock()

	return nil
}

// CreateIdentity creates a new local identity
func (c *Client) CreateIdentity(nickname, context string, publishTrustList bool) (*Identity, error) {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName":       "plugins.WebOfTrust.WebOfTrust",
			"Identifier":       c.generateID(),
			"Message":          "CreateIdentity",
			"Nickname":         nickname,
			"Context":          context,
			"PublishTrustList": strconv.FormatBool(publishTrustList),
		},
	}

	resp, err := c.sendAndWait(msg)
	if err != nil {
		return nil, err
	}

	return &Identity{
		ID:         resp.Fields["ID"],
		Nickname:   nickname,
		RequestURI: resp.Fields["RequestURI"],
		InsertURI:  resp.Fields["InsertURI"],
		Contexts:   []string{context},
		IsOwn:      true,
	}, nil
}

// RandomName generates a random identity nickname
func (c *Client) RandomName() (string, error) {
	msg := &fcp.Message{
		Name: "FCPPluginMessage",
		Fields: map[string]string{
			"PluginName": "plugins.WebOfTrust.WebOfTrust",
			"Identifier": c.generateID(),
			"Message":    "RandomName",
		},
	}

	resp, err := c.sendAndWait(msg)
	if err != nil {
		return "", err
	}

	return resp.Fields["Name"], nil
}

// Helper functions for parsing responses

func parseOwnIdentitiesResponse(msg *fcp.Message) []*Identity {
	amountStr := msg.Fields["Amount"]
	if amountStr == "" {
		return nil
	}

	amount, _ := strconv.Atoi(amountStr)
	identities := make([]*Identity, 0, amount)

	for i := 0; i < amount; i++ {
		prefix := fmt.Sprintf("%d", i)
		id := &Identity{
			ID:         msg.Fields["ID"+prefix],
			Nickname:   msg.Fields["Nickname"+prefix],
			RequestURI: msg.Fields["RequestURI"+prefix],
			InsertURI:  msg.Fields["InsertURI"+prefix],
			IsOwn:      true,
			Contexts:   parseContexts(msg.Fields, fmt.Sprintf("Contexts%d.", i)),
			Properties: parseProperties(msg.Fields, fmt.Sprintf("Properties%d.", i)),
		}

		// Fallback to deprecated field name
		if id.ID == "" {
			id.ID = msg.Fields["Identity"+prefix]
		}

		identities = append(identities, id)
	}

	return identities
}

func parseIdentitiesResponse(msg *fcp.Message) []*Identity {
	amountStr := msg.Fields["Identities.Amount"]
	if amountStr == "" {
		return nil
	}

	amount, _ := strconv.Atoi(amountStr)
	identities := make([]*Identity, 0, amount)

	for i := 0; i < amount; i++ {
		prefix := fmt.Sprintf("Identities.%d.", i)
		id := parseIdentityFromFields(msg.Fields, prefix)
		if id != nil {
			identities = append(identities, id)
		}
	}

	return identities
}

func parseIdentityFromFields(fields map[string]string, prefix string) *Identity {
	id := &Identity{
		ID:                    fields[prefix+"ID"],
		Nickname:              fields[prefix+"Nickname"],
		RequestURI:            fields[prefix+"RequestURI"],
		InsertURI:             fields[prefix+"InsertURI"],
		CurrentEditionFetchState: fields[prefix+"CurrentEditionFetchState"],
	}

	if id.ID == "" {
		return nil
	}

	id.IsOwn = fields[prefix+"Type"] == "OwnIdentity"
	id.PublishesTrustList = fields[prefix+"PublishesTrustList"] == "true"
	id.Contexts = parseContexts(fields, prefix+"Contexts.")
	id.Properties = parseProperties(fields, prefix+"Properties.")

	return id
}

func parseContexts(fields map[string]string, prefix string) []string {
	amountStr := fields[prefix+"Amount"]
	if amountStr == "" {
		// Try old format
		var contexts []string
		for i := 0; ; i++ {
			ctx := fields[fmt.Sprintf("%sContext%d", prefix, i)]
			if ctx == "" {
				break
			}
			contexts = append(contexts, ctx)
		}
		return contexts
	}

	amount, _ := strconv.Atoi(amountStr)
	contexts := make([]string, 0, amount)

	for i := 0; i < amount; i++ {
		ctx := fields[fmt.Sprintf("%s%d.Name", prefix, i)]
		if ctx != "" {
			contexts = append(contexts, ctx)
		}
	}

	return contexts
}

func parseProperties(fields map[string]string, prefix string) map[string]string {
	props := make(map[string]string)

	amountStr := fields[prefix+"Amount"]
	if amountStr == "" {
		// Try old format
		for i := 0; ; i++ {
			name := fields[fmt.Sprintf("%sProperty%d.Name", prefix, i)]
			if name == "" {
				break
			}
			value := fields[fmt.Sprintf("%sProperty%d.Value", prefix, i)]
			props[name] = value
		}
		return props
	}

	amount, _ := strconv.Atoi(amountStr)
	for i := 0; i < amount; i++ {
		name := fields[fmt.Sprintf("%s%d.Name", prefix, i)]
		value := fields[fmt.Sprintf("%s%d.Value", prefix, i)]
		if name != "" {
			props[name] = value
		}
	}

	return props
}

func parseTrustFromFields(fields map[string]string, prefix string) *Trust {
	valueStr := fields[prefix+"Value"]
	if valueStr == "" || valueStr == "Nonexistent" {
		return nil
	}

	value, _ := strconv.Atoi(valueStr)

	return &Trust{
		TrusterID: fields[prefix+"Truster"],
		TrusteeID: fields[prefix+"Trustee"],
		Value:     int8(value),
		Comment:   fields[prefix+"Comment"],
	}
}

func parseScoreFromFields(fields map[string]string, prefix string) *Score {
	valueStr := fields[prefix+"Value"]
	if valueStr == "" || valueStr == "Nonexistent" {
		return nil
	}

	value, _ := strconv.Atoi(valueStr)
	rank, _ := strconv.Atoi(fields[prefix+"Rank"])
	capacity, _ := strconv.Atoi(fields[prefix+"Capacity"])

	return &Score{
		TrusterID: fields[prefix+"Truster"],
		TrusteeID: fields[prefix+"Trustee"],
		Value:     value,
		Rank:      rank,
		Capacity:  capacity,
	}
}

// HasContext checks if an identity has a specific context
func (id *Identity) HasContext(context string) bool {
	for _, ctx := range id.Contexts {
		if strings.EqualFold(ctx, context) {
			return true
		}
	}
	return false
}

// GetProperty retrieves a property value from an identity
func (id *Identity) GetProperty(name string) string {
	if id.Properties == nil {
		return ""
	}
	return id.Properties[name]
}
