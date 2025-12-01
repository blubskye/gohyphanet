// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package sone

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
	"github.com/blubskye/gohyphanet/wot"
)

// Version is the GoSone client version
const Version = "0.1.0"

// ClientName is the client name reported in Sone XML
const ClientName = "GoSone"

// Core is the main Sone application component
type Core struct {
	mu sync.RWMutex

	// Dependencies
	fcpClient  *fcp.Client
	wotClient  *wot.Client
	database   Database
	eventBus   *EventBus
	notifyMgr  *NotificationManager
	uskMonitor *USKMonitor

	// Configuration
	config *Config

	// State
	localSones   map[string]*Sone
	remoteSones  map[string]*Sone
	soneInserters map[string]*SoneInserter
	soneDownloaders map[string]*SoneDownloader

	// Lifecycle
	ctx        context.Context
	cancel     context.CancelFunc
	running    bool
	wg         sync.WaitGroup
}

// Config holds configuration for the Sone core
type Config struct {
	DataDir          string        // Directory for persistent data
	FCPHost          string        // Freenet FCP host
	FCPPort          int           // Freenet FCP port
	InsertionDelay   time.Duration // Delay between insertions
	RefreshInterval  time.Duration // Interval for checking Sone updates
	SoneContext      string        // WoT context for Sone (default: "Sone")
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		DataDir:         "~/.gosone",
		FCPHost:         "localhost",
		FCPPort:         9481,
		InsertionDelay:  60 * time.Second,
		RefreshInterval: 60 * time.Second,
		SoneContext:     "Sone",
	}
}

// NewCore creates a new Sone core instance
func NewCore(config *Config) *Core {
	if config == nil {
		config = DefaultConfig()
	}

	eventBus := NewEventBus()
	database := NewMemoryDatabase(config.DataDir)

	return &Core{
		config:          config,
		database:        database,
		eventBus:        eventBus,
		notifyMgr:       NewNotificationManager(eventBus),
		localSones:      make(map[string]*Sone),
		remoteSones:     make(map[string]*Sone),
		soneInserters:   make(map[string]*SoneInserter),
		soneDownloaders: make(map[string]*SoneDownloader),
	}
}

// Start initializes and starts the Sone core
func (c *Core) Start() error {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return fmt.Errorf("core is already running")
	}
	c.mu.Unlock()

	// Create context
	c.ctx, c.cancel = context.WithCancel(context.Background())

	// Load persisted data
	if err := c.database.Load(); err != nil {
		log.Printf("Warning: failed to load database: %v", err)
	}

	// Connect to Freenet
	fcpConfig := &fcp.Config{
		Host:    c.config.FCPHost,
		Port:    c.config.FCPPort,
		Name:    ClientName,
		Version: "2.0",
	}

	var err error
	c.fcpClient, err = fcp.Connect(fcpConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to Freenet: %w", err)
	}

	// Connect to Web of Trust
	c.wotClient = wot.NewClient(c.fcpClient)
	c.wotClient.StartListening()

	// Start event bus
	c.eventBus.Start()

	c.mu.Lock()
	c.running = true
	c.mu.Unlock()

	// Load identities
	if err := c.loadIdentities(); err != nil {
		log.Printf("Warning: failed to load identities: %v", err)
	}

	// Start USK monitor for real-time Sone updates
	c.uskMonitor = NewUSKMonitor(c)
	if err := c.uskMonitor.Start(); err != nil {
		log.Printf("Warning: failed to start USK monitor: %v", err)
	}

	// Start background tasks
	c.wg.Add(1)
	go c.identityWatchLoop()

	log.Printf("GoSone v%s started", Version)
	return nil
}

// Stop gracefully shuts down the Sone core
func (c *Core) Stop() error {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return nil
	}
	c.running = false
	c.mu.Unlock()

	// Cancel context
	if c.cancel != nil {
		c.cancel()
	}

	// Wait for background tasks
	c.wg.Wait()

	// Stop USK monitor
	if c.uskMonitor != nil {
		c.uskMonitor.Stop()
	}

	// Stop inserters
	for _, inserter := range c.soneInserters {
		inserter.Stop()
	}

	// Stop event bus
	c.eventBus.Stop()

	// Save database
	if err := c.database.Save(); err != nil {
		log.Printf("Warning: failed to save database: %v", err)
	}

	// Close connections
	if c.wotClient != nil {
		c.wotClient.Close()
	}

	log.Println("GoSone stopped")
	return nil
}

// loadIdentities loads local identities from WoT
func (c *Core) loadIdentities() error {
	identities, err := c.wotClient.GetOwnIdentities()
	if err != nil {
		return err
	}

	for _, identity := range identities {
		if identity.HasContext(c.config.SoneContext) {
			sone := NewLocalSone(identity)
			c.addLocalSone(sone)
		}
	}

	return nil
}

// identityWatchLoop monitors WoT for identity changes
func (c *Core) identityWatchLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			c.refreshIdentities()
		}
	}
}

// refreshIdentities checks for new/updated identities
func (c *Core) refreshIdentities() {
	// Get trusted Sone identities
	identities, err := c.wotClient.GetIdentitiesByScore("", "+", c.config.SoneContext)
	if err != nil {
		log.Printf("Failed to get identities: %v", err)
		return
	}

	for _, identity := range identities {
		c.mu.RLock()
		_, exists := c.remoteSones[identity.ID]
		c.mu.RUnlock()

		if !exists {
			// New Sone discovered
			sone := NewSone(identity.ID)
			sone.Identity = identity
			sone.Name = identity.Nickname
			sone.RequestURI = identity.RequestURI
			c.addRemoteSone(sone)
			c.eventBus.PublishSoneDiscovered(sone)
		}
	}
}

// addLocalSone adds a local Sone
func (c *Core) addLocalSone(sone *Sone) {
	c.mu.Lock()
	c.localSones[sone.ID] = sone
	c.mu.Unlock()

	c.database.StoreSone(sone)

	// Start inserter
	inserter := NewSoneInserter(c, sone)
	c.soneInserters[sone.ID] = inserter
	inserter.Start()

	c.eventBus.Publish(Event{
		Type: EventLocalSoneAdded,
		Sone: sone,
	})
}

// addRemoteSone adds a remote Sone
func (c *Core) addRemoteSone(sone *Sone) {
	c.mu.Lock()
	c.remoteSones[sone.ID] = sone
	c.mu.Unlock()

	c.database.StoreSone(sone)

	// Start downloader
	downloader := NewSoneDownloader(c, sone)
	c.soneDownloaders[sone.ID] = downloader
	downloader.Start()
}

// GetLocalSones returns all local Sones
func (c *Core) GetLocalSones() []*Sone {
	c.mu.RLock()
	defer c.mu.RUnlock()

	sones := make([]*Sone, 0, len(c.localSones))
	for _, s := range c.localSones {
		sones = append(sones, s)
	}
	return sones
}

// GetLocalSone returns a local Sone by ID
func (c *Core) GetLocalSone(id string) *Sone {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.localSones[id]
}

// GetSone returns a Sone by ID (local or remote)
func (c *Core) GetSone(id string) *Sone {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if sone, ok := c.localSones[id]; ok {
		return sone
	}
	return c.remoteSones[id]
}

// GetAllSones returns all known Sones
func (c *Core) GetAllSones() []*Sone {
	c.mu.RLock()
	defer c.mu.RUnlock()

	sones := make([]*Sone, 0, len(c.localSones)+len(c.remoteSones))
	for _, s := range c.localSones {
		sones = append(sones, s)
	}
	for _, s := range c.remoteSones {
		sones = append(sones, s)
	}
	return sones
}

// CreatePost creates a new post for a local Sone
func (c *Core) CreatePost(soneID string, text string, recipientID *string) (*Post, error) {
	sone := c.GetLocalSone(soneID)
	if sone == nil {
		return nil, fmt.Errorf("sone not found: %s", soneID)
	}

	post := NewPost(soneID, text)
	post.RecipientID = recipientID

	sone.mu.Lock()
	sone.Posts = append(sone.Posts, post)
	sone.Time = time.Now().UnixMilli()
	sone.mu.Unlock()

	c.database.StorePost(post)
	c.eventBus.PublishNewPostFound(post)

	// Trigger re-insertion
	if inserter, ok := c.soneInserters[soneID]; ok {
		inserter.TriggerInsert()
	}

	return post, nil
}

// CreateReply creates a reply to a post
func (c *Core) CreateReply(soneID string, postID string, text string) (*PostReply, error) {
	sone := c.GetLocalSone(soneID)
	if sone == nil {
		return nil, fmt.Errorf("sone not found: %s", soneID)
	}

	reply := NewPostReply(soneID, postID, text)

	sone.mu.Lock()
	sone.Replies = append(sone.Replies, reply)
	sone.Time = time.Now().UnixMilli()
	sone.mu.Unlock()

	c.database.StoreReply(reply)
	c.eventBus.PublishNewReplyFound(reply)

	// Trigger re-insertion
	if inserter, ok := c.soneInserters[soneID]; ok {
		inserter.TriggerInsert()
	}

	return reply, nil
}

// DeletePost removes a post
func (c *Core) DeletePost(soneID string, postID string) error {
	sone := c.GetLocalSone(soneID)
	if sone == nil {
		return fmt.Errorf("sone not found: %s", soneID)
	}

	sone.mu.Lock()
	for i, post := range sone.Posts {
		if post.ID == postID {
			sone.Posts = append(sone.Posts[:i], sone.Posts[i+1:]...)
			sone.Time = time.Now().UnixMilli()
			break
		}
	}
	sone.mu.Unlock()

	c.database.RemovePost(postID)

	// Trigger re-insertion
	if inserter, ok := c.soneInserters[soneID]; ok {
		inserter.TriggerInsert()
	}

	return nil
}

// DeleteReply removes a reply
func (c *Core) DeleteReply(soneID string, replyID string) error {
	sone := c.GetLocalSone(soneID)
	if sone == nil {
		return fmt.Errorf("sone not found: %s", soneID)
	}

	sone.mu.Lock()
	for i, reply := range sone.Replies {
		if reply.ID == replyID {
			sone.Replies = append(sone.Replies[:i], sone.Replies[i+1:]...)
			sone.Time = time.Now().UnixMilli()
			break
		}
	}
	sone.mu.Unlock()

	c.database.RemoveReply(replyID)

	// Trigger re-insertion
	if inserter, ok := c.soneInserters[soneID]; ok {
		inserter.TriggerInsert()
	}

	return nil
}

// LikePost likes a post
func (c *Core) LikePost(soneID string, postID string) error {
	sone := c.GetLocalSone(soneID)
	if sone == nil {
		return fmt.Errorf("sone not found: %s", soneID)
	}

	sone.LikePost(postID)

	// Trigger re-insertion
	if inserter, ok := c.soneInserters[soneID]; ok {
		inserter.TriggerInsert()
	}

	return nil
}

// UnlikePost unlikes a post
func (c *Core) UnlikePost(soneID string, postID string) error {
	sone := c.GetLocalSone(soneID)
	if sone == nil {
		return fmt.Errorf("sone not found: %s", soneID)
	}

	sone.UnlikePost(postID)

	// Trigger re-insertion
	if inserter, ok := c.soneInserters[soneID]; ok {
		inserter.TriggerInsert()
	}

	return nil
}

// FollowSone adds a Sone to the follow list
func (c *Core) FollowSone(localSoneID string, remoteSoneID string) error {
	sone := c.GetLocalSone(localSoneID)
	if sone == nil {
		return fmt.Errorf("sone not found: %s", localSoneID)
	}

	sone.AddFriend(remoteSoneID)
	c.database.AddFriend(localSoneID, remoteSoneID)

	// Subscribe to USK updates for the followed Sone
	if c.uskMonitor != nil {
		c.uskMonitor.SubscribeSone(remoteSoneID)
	}

	return nil
}

// UnfollowSone removes a Sone from the follow list
func (c *Core) UnfollowSone(localSoneID string, remoteSoneID string) error {
	sone := c.GetLocalSone(localSoneID)
	if sone == nil {
		return fmt.Errorf("sone not found: %s", localSoneID)
	}

	sone.RemoveFriend(remoteSoneID)
	c.database.RemoveFriend(localSoneID, remoteSoneID)

	// Unsubscribe from USK updates if no longer followed by any local Sone
	if c.uskMonitor != nil {
		stillFollowed := false
		for _, ls := range c.localSones {
			if _, ok := ls.Friends[remoteSoneID]; ok {
				stillFollowed = true
				break
			}
		}
		if !stillFollowed {
			c.uskMonitor.UnsubscribeSone(remoteSoneID)
		}
	}

	return nil
}

// GetPostFeed returns posts for a Sone's feed
func (c *Core) GetPostFeed(soneID string) []*Post {
	sone := c.GetLocalSone(soneID)
	if sone == nil {
		return nil
	}

	// Get posts from followed Sones and own posts
	posts := make([]*Post, 0)

	// Own posts
	posts = append(posts, sone.Posts...)

	// Friends' posts
	for friendID := range sone.Friends {
		if friend := c.GetSone(friendID); friend != nil {
			posts = append(posts, friend.Posts...)
		}
	}

	// Sort by time, newest first
	// (already handled by database layer)
	return posts
}

// GetNotifications returns current notifications
func (c *Core) GetNotifications() []*Notification {
	return c.notifyMgr.GetNotifications()
}

// DismissNotification dismisses a notification
func (c *Core) DismissNotification(id string) {
	c.notifyMgr.DismissNotification(id)
}

// EventBus returns the event bus
func (c *Core) EventBus() *EventBus {
	return c.eventBus
}

// Database returns the database
func (c *Core) Database() Database {
	return c.database
}

// FCPClient returns the FCP client
func (c *Core) FCPClient() *fcp.Client {
	return c.fcpClient
}

// SoneInserter handles inserting a local Sone to Hyphanet
type SoneInserter struct {
	core        *Core
	sone        *Sone
	fingerprint string
	running     bool
	triggerChan chan struct{}
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
}

// NewSoneInserter creates a new Sone inserter
func NewSoneInserter(core *Core, sone *Sone) *SoneInserter {
	ctx, cancel := context.WithCancel(context.Background())
	return &SoneInserter{
		core:        core,
		sone:        sone,
		triggerChan: make(chan struct{}, 1),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// Start begins the insertion loop
func (si *SoneInserter) Start() {
	si.mu.Lock()
	if si.running {
		si.mu.Unlock()
		return
	}
	si.running = true
	si.mu.Unlock()

	si.fingerprint = si.sone.GetFingerprint()

	go si.insertLoop()
}

// Stop stops the inserter
func (si *SoneInserter) Stop() {
	si.mu.Lock()
	si.running = false
	si.mu.Unlock()
	si.cancel()
}

// TriggerInsert triggers an immediate insertion check
func (si *SoneInserter) TriggerInsert() {
	select {
	case si.triggerChan <- struct{}{}:
	default:
	}
}

func (si *SoneInserter) insertLoop() {
	ticker := time.NewTicker(si.core.config.InsertionDelay)
	defer ticker.Stop()

	for {
		select {
		case <-si.ctx.Done():
			return
		case <-ticker.C:
			si.checkAndInsert()
		case <-si.triggerChan:
			// Wait for insertion delay
			time.Sleep(si.core.config.InsertionDelay)
			si.checkAndInsert()
		}
	}
}

func (si *SoneInserter) checkAndInsert() {
	newFingerprint := si.sone.GetFingerprint()
	if newFingerprint == si.fingerprint {
		return // No changes
	}

	si.core.eventBus.PublishSoneInserting(si.sone)

	// Generate XML
	xml, err := si.sone.ToXML(ClientName, Version)
	if err != nil {
		log.Printf("Failed to generate Sone XML: %v", err)
		si.core.eventBus.PublishSoneInsertAborted(si.sone, err.Error())
		return
	}

	// Insert to Hyphanet
	si.sone.mu.Lock()
	si.sone.Status = StatusInserting
	edition := si.sone.LatestEdition + 1
	uri := fmt.Sprintf("%s/sone.xml", si.sone.InsertURI)
	si.sone.mu.Unlock()

	identifier := fmt.Sprintf("sone-insert-%s-%d", si.sone.ID, edition)
	if err := si.core.fcpClient.ClientPut(uri, identifier, xml); err != nil {
		log.Printf("Failed to insert Sone: %v", err)
		si.sone.mu.Lock()
		si.sone.Status = StatusIdle
		si.sone.mu.Unlock()
		si.core.eventBus.PublishSoneInsertAborted(si.sone, err.Error())
		return
	}

	// Update state
	si.sone.mu.Lock()
	si.sone.LatestEdition = edition
	si.sone.Status = StatusIdle
	si.sone.mu.Unlock()

	si.fingerprint = newFingerprint
	si.core.eventBus.PublishSoneInserted(si.sone, edition)
	log.Printf("Sone %s inserted at edition %d", si.sone.Name, edition)
}

// SoneDownloader handles downloading remote Sones
type SoneDownloader struct {
	core    *Core
	sone    *Sone
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
}

// NewSoneDownloader creates a new Sone downloader
func NewSoneDownloader(core *Core, sone *Sone) *SoneDownloader {
	ctx, cancel := context.WithCancel(context.Background())
	return &SoneDownloader{
		core:   core,
		sone:   sone,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins the download loop
func (sd *SoneDownloader) Start() {
	sd.mu.Lock()
	if sd.running {
		sd.mu.Unlock()
		return
	}
	sd.running = true
	sd.mu.Unlock()

	go sd.downloadLoop()
}

// Stop stops the downloader
func (sd *SoneDownloader) Stop() {
	sd.mu.Lock()
	sd.running = false
	sd.mu.Unlock()
	sd.cancel()
}

func (sd *SoneDownloader) downloadLoop() {
	// Initial download
	sd.fetchSone()

	ticker := time.NewTicker(sd.core.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-sd.ctx.Done():
			return
		case <-ticker.C:
			sd.fetchSone()
		}
	}
}

func (sd *SoneDownloader) fetchSone() {
	sd.sone.mu.Lock()
	sd.sone.Status = StatusDownloading
	uri := fmt.Sprintf("%s/sone.xml", sd.sone.RequestURI)
	sd.sone.mu.Unlock()

	identifier := fmt.Sprintf("sone-fetch-%s-%d", sd.sone.ID, time.Now().UnixMilli())
	if err := sd.core.fcpClient.ClientGet(uri, identifier); err != nil {
		log.Printf("Failed to request Sone %s: %v", sd.sone.Name, err)
		sd.sone.mu.Lock()
		sd.sone.Status = StatusIdle
		sd.sone.mu.Unlock()
		return
	}

	// Note: Response handling would be done via FCP message handlers
	// This is simplified - full implementation would register handlers
	sd.sone.mu.Lock()
	sd.sone.Status = StatusIdle
	sd.sone.mu.Unlock()
}

// ProcessFetchedSone processes a fetched Sone XML
func (c *Core) ProcessFetchedSone(soneID string, data []byte) error {
	sone, err := ParseSoneXML(data, soneID)
	if err != nil {
		return fmt.Errorf("failed to parse Sone XML: %w", err)
	}

	c.mu.Lock()
	existing := c.remoteSones[soneID]
	if existing != nil {
		// Update existing Sone
		existing.mu.Lock()
		existing.Time = sone.Time
		existing.Profile = sone.Profile
		existing.Posts = sone.Posts
		existing.Replies = sone.Replies
		existing.LikedPostIDs = sone.LikedPostIDs
		existing.LikedReplyIDs = sone.LikedReplyIDs
		existing.RootAlbum = sone.RootAlbum
		existing.Client = sone.Client
		existing.Status = StatusIdle
		existing.mu.Unlock()

		c.mu.Unlock()
		c.database.StoreSone(existing)
		c.eventBus.PublishSoneUpdated(existing)
	} else {
		c.remoteSones[soneID] = sone
		c.mu.Unlock()
		c.database.StoreSone(sone)
		c.eventBus.PublishSoneDiscovered(sone)
	}

	// Check for new posts/replies
	for _, post := range sone.Posts {
		if !c.database.IsPostKnown(post.ID) {
			c.eventBus.PublishNewPostFound(post)
		}
	}

	for _, reply := range sone.Replies {
		if !c.database.IsReplyKnown(reply.ID) {
			c.eventBus.PublishNewReplyFound(reply)
		}
	}

	return nil
}
