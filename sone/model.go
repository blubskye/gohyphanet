// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

// Package sone implements a Go port of the Sone social networking plugin for Hyphanet.
package sone

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/wot"
)

// SoneStatus represents the current state of a Sone
type SoneStatus int

const (
	StatusUnknown     SoneStatus = iota // Not yet downloaded
	StatusIdle                          // Not being downloaded or inserted
	StatusInserting                     // Currently being inserted
	StatusDownloading                   // Currently being downloaded
)

func (s SoneStatus) String() string {
	switch s {
	case StatusUnknown:
		return "unknown"
	case StatusIdle:
		return "idle"
	case StatusInserting:
		return "inserting"
	case StatusDownloading:
		return "downloading"
	default:
		return "unknown"
	}
}

// Sone represents a user profile in the Sone social network
type Sone struct {
	mu sync.RWMutex

	// Identity from Web of Trust
	ID       string        // Same as WoT identity ID (43-char base64)
	Identity *wot.Identity // WoT identity reference

	// Basic info
	Name          string     // Display name (from WoT nickname)
	RequestURI    string     // Freenet URI for fetching this Sone
	InsertURI     string     // Freenet URI for inserting (only for local Sones)
	LatestEdition int64      // Latest known edition number
	Time          int64      // Time of last update (Unix milliseconds)
	Status        SoneStatus // Current status

	// Profile
	Profile *Profile

	// Client info
	Client *Client

	// Content
	Posts   []*Post
	Replies []*PostReply

	// Likes
	LikedPostIDs  map[string]bool
	LikedReplyIDs map[string]bool

	// Albums (hierarchical)
	RootAlbum *Album

	// Friends (Sone IDs)
	Friends map[string]bool

	// State
	IsLocal bool // True if this is our own Sone
	IsKnown bool // True if user has seen this Sone
}

// NewSone creates a new Sone with the given ID
func NewSone(id string) *Sone {
	return &Sone{
		ID:            id,
		Status:        StatusUnknown,
		Profile:       NewProfile(),
		Posts:         make([]*Post, 0),
		Replies:       make([]*PostReply, 0),
		LikedPostIDs:  make(map[string]bool),
		LikedReplyIDs: make(map[string]bool),
		RootAlbum:     NewRootAlbum(id),
		Friends:       make(map[string]bool),
	}
}

// NewLocalSone creates a new local Sone from a WoT identity
func NewLocalSone(identity *wot.Identity) *Sone {
	s := NewSone(identity.ID)
	s.Identity = identity
	s.Name = identity.Nickname
	s.RequestURI = identity.RequestURI
	s.InsertURI = identity.InsertURI
	s.IsLocal = true
	s.Status = StatusIdle
	return s
}

// GetFingerprint returns a hash representing the current state of this Sone
func (s *Sone) GetFingerprint() string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	h := sha256.New()

	// Profile fingerprint
	h.Write([]byte(s.Profile.GetFingerprint()))

	// Posts (sorted by time)
	posts := make([]*Post, len(s.Posts))
	copy(posts, s.Posts)
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Time < posts[j].Time
	})
	for _, post := range posts {
		h.Write([]byte(post.GetFingerprint()))
	}

	// Replies (sorted by time)
	replies := make([]*PostReply, len(s.Replies))
	copy(replies, s.Replies)
	sort.Slice(replies, func(i, j int) bool {
		return replies[i].Time < replies[j].Time
	})
	for _, reply := range replies {
		h.Write([]byte(reply.GetFingerprint()))
	}

	// Liked post IDs (sorted)
	likedPosts := make([]string, 0, len(s.LikedPostIDs))
	for id := range s.LikedPostIDs {
		likedPosts = append(likedPosts, id)
	}
	sort.Strings(likedPosts)
	for _, id := range likedPosts {
		h.Write([]byte(id))
	}

	// Liked reply IDs (sorted)
	likedReplies := make([]string, 0, len(s.LikedReplyIDs))
	for id := range s.LikedReplyIDs {
		likedReplies = append(likedReplies, id)
	}
	sort.Strings(likedReplies)
	for _, id := range likedReplies {
		h.Write([]byte(id))
	}

	// Albums
	if s.RootAlbum != nil {
		h.Write([]byte(s.RootAlbum.GetFingerprint()))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// HasFriend checks if this Sone has the given Sone ID as a friend
func (s *Sone) HasFriend(soneID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Friends[soneID]
}

// AddFriend adds a friend to this Sone
func (s *Sone) AddFriend(soneID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Friends[soneID] = true
}

// RemoveFriend removes a friend from this Sone
func (s *Sone) RemoveFriend(soneID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.Friends, soneID)
}

// LikePost adds a post ID to liked posts
func (s *Sone) LikePost(postID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LikedPostIDs[postID] = true
}

// UnlikePost removes a post ID from liked posts
func (s *Sone) UnlikePost(postID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.LikedPostIDs, postID)
}

// IsPostLiked checks if a post is liked
func (s *Sone) IsPostLiked(postID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LikedPostIDs[postID]
}

// LikeReply adds a reply ID to liked replies
func (s *Sone) LikeReply(replyID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.LikedReplyIDs[replyID] = true
}

// UnlikeReply removes a reply ID from liked replies
func (s *Sone) UnlikeReply(replyID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.LikedReplyIDs, replyID)
}

// IsReplyLiked checks if a reply is liked
func (s *Sone) IsReplyLiked(replyID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LikedReplyIDs[replyID]
}

// Profile contains personal information about a Sone
type Profile struct {
	FirstName  string
	MiddleName string
	LastName   string
	BirthDay   *int
	BirthMonth *int
	BirthYear  *int
	Avatar     string   // Image ID
	Fields     []*Field // Custom profile fields
}

// Field represents a custom profile field
type Field struct {
	ID    string
	Name  string
	Value string
}

// NewProfile creates a new empty profile
func NewProfile() *Profile {
	return &Profile{
		Fields: make([]*Field, 0),
	}
}

// GetFingerprint returns a hash of the profile for change detection
func (p *Profile) GetFingerprint() string {
	h := sha256.New()
	h.Write([]byte("Profile("))

	if p.FirstName != "" {
		h.Write([]byte(fmt.Sprintf("FirstName(%s)", p.FirstName)))
	}
	if p.MiddleName != "" {
		h.Write([]byte(fmt.Sprintf("MiddleName(%s)", p.MiddleName)))
	}
	if p.LastName != "" {
		h.Write([]byte(fmt.Sprintf("LastName(%s)", p.LastName)))
	}
	if p.BirthDay != nil {
		h.Write([]byte(fmt.Sprintf("BirthDay(%d)", *p.BirthDay)))
	}
	if p.BirthMonth != nil {
		h.Write([]byte(fmt.Sprintf("BirthMonth(%d)", *p.BirthMonth)))
	}
	if p.BirthYear != nil {
		h.Write([]byte(fmt.Sprintf("BirthYear(%d)", *p.BirthYear)))
	}
	if p.Avatar != "" {
		h.Write([]byte(fmt.Sprintf("Avatar(%s)", p.Avatar)))
	}

	h.Write([]byte("ContactInformation("))
	for _, field := range p.Fields {
		h.Write([]byte(fmt.Sprintf("%s(%s)", field.Name, field.Value)))
	}
	h.Write([]byte(")"))
	h.Write([]byte(")"))

	return hex.EncodeToString(h.Sum(nil))
}

// AddField adds a new field to the profile
func (p *Profile) AddField(name string) *Field {
	field := &Field{
		ID:   generateUUID(),
		Name: name,
	}
	p.Fields = append(p.Fields, field)
	return field
}

// GetFieldByName returns a field by name
func (p *Profile) GetFieldByName(name string) *Field {
	for _, f := range p.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

// RemoveField removes a field from the profile
func (p *Profile) RemoveField(fieldID string) {
	for i, f := range p.Fields {
		if f.ID == fieldID {
			p.Fields = append(p.Fields[:i], p.Fields[i+1:]...)
			return
		}
	}
}

// Client represents the Sone client software info
type Client struct {
	Name    string
	Version string
}

// Post represents a status update posted by a Sone
type Post struct {
	ID          string
	SoneID      string  // Author's Sone ID
	RecipientID *string // Optional recipient Sone ID
	Time        int64   // Unix milliseconds
	Text        string
	IsKnown     bool // True if user has seen this post
	IsLoaded    bool // True if post data has been loaded
}

// NewPost creates a new post
func NewPost(soneID string, text string) *Post {
	return &Post{
		ID:       generateUUID(),
		SoneID:   soneID,
		Time:     time.Now().UnixMilli(),
		Text:     text,
		IsLoaded: true,
	}
}

// GetFingerprint returns a hash of the post for change detection
func (p *Post) GetFingerprint() string {
	h := sha256.New()
	h.Write([]byte("Post("))
	h.Write([]byte(fmt.Sprintf("ID(%s)", p.ID)))
	if p.RecipientID != nil {
		h.Write([]byte(fmt.Sprintf("Recipient(%s)", *p.RecipientID)))
	}
	h.Write([]byte(fmt.Sprintf("Time(%d)", p.Time)))
	h.Write([]byte(fmt.Sprintf("Text(%s)", p.Text)))
	h.Write([]byte(")"))
	return hex.EncodeToString(h.Sum(nil))
}

// PostReply represents a reply to a post
type PostReply struct {
	ID       string
	SoneID   string // Author's Sone ID
	PostID   string // Post being replied to
	Time     int64  // Unix milliseconds
	Text     string
	IsKnown  bool // True if user has seen this reply
	IsLoaded bool // True if reply data has been loaded
}

// NewPostReply creates a new reply
func NewPostReply(soneID string, postID string, text string) *PostReply {
	return &PostReply{
		ID:       generateUUID(),
		SoneID:   soneID,
		PostID:   postID,
		Time:     time.Now().UnixMilli(),
		Text:     text,
		IsLoaded: true,
	}
}

// GetFingerprint returns a hash of the reply for change detection
func (r *PostReply) GetFingerprint() string {
	h := sha256.New()
	h.Write([]byte("PostReply("))
	h.Write([]byte(fmt.Sprintf("ID(%s)", r.ID)))
	h.Write([]byte(fmt.Sprintf("Post(%s)", r.PostID)))
	h.Write([]byte(fmt.Sprintf("Time(%d)", r.Time)))
	h.Write([]byte(fmt.Sprintf("Text(%s)", r.Text)))
	h.Write([]byte(")"))
	return hex.EncodeToString(h.Sum(nil))
}

// Album is a container for images
type Album struct {
	ID          string
	SoneID      string   // Owner Sone ID
	ParentID    *string  // Parent album ID (nil for root)
	Title       string
	Description string
	AlbumImage  string   // ID of the album cover image
	Albums      []*Album // Nested albums
	Images      []*Image
	IsRoot      bool
}

// NewRootAlbum creates a new root album for a Sone
func NewRootAlbum(soneID string) *Album {
	return &Album{
		ID:     generateUUID(),
		SoneID: soneID,
		Albums: make([]*Album, 0),
		Images: make([]*Image, 0),
		IsRoot: true,
	}
}

// NewAlbum creates a new album
func NewAlbum(soneID string, title string) *Album {
	return &Album{
		ID:     generateUUID(),
		SoneID: soneID,
		Title:  title,
		Albums: make([]*Album, 0),
		Images: make([]*Image, 0),
	}
}

// GetFingerprint returns a hash of the album for change detection
func (a *Album) GetFingerprint() string {
	h := sha256.New()
	h.Write([]byte("Album("))
	h.Write([]byte(fmt.Sprintf("ID(%s)", a.ID)))
	if a.Title != "" {
		h.Write([]byte(fmt.Sprintf("Title(%s)", a.Title)))
	}
	if a.Description != "" {
		h.Write([]byte(fmt.Sprintf("Description(%s)", a.Description)))
	}

	// Nested albums
	for _, album := range a.Albums {
		h.Write([]byte(album.GetFingerprint()))
	}

	// Images
	h.Write([]byte("Images("))
	for _, img := range a.Images {
		h.Write([]byte(img.GetFingerprint()))
	}
	h.Write([]byte(")"))

	h.Write([]byte(")"))
	return hex.EncodeToString(h.Sum(nil))
}

// AddAlbum adds a nested album
func (a *Album) AddAlbum(album *Album) {
	album.ParentID = &a.ID
	a.Albums = append(a.Albums, album)
}

// RemoveAlbum removes a nested album
func (a *Album) RemoveAlbum(albumID string) {
	for i, album := range a.Albums {
		if album.ID == albumID {
			a.Albums = append(a.Albums[:i], a.Albums[i+1:]...)
			return
		}
	}
}

// AddImage adds an image to the album
func (a *Album) AddImage(image *Image) {
	image.AlbumID = a.ID
	a.Images = append(a.Images, image)
}

// RemoveImage removes an image from the album
func (a *Album) RemoveImage(imageID string) {
	for i, img := range a.Images {
		if img.ID == imageID {
			a.Images = append(a.Images[:i], a.Images[i+1:]...)
			return
		}
	}
}

// IsEmpty returns true if the album has no content
func (a *Album) IsEmpty() bool {
	return len(a.Albums) == 0 && len(a.Images) == 0
}

// Image represents an image uploaded to Hyphanet
type Image struct {
	ID           string
	SoneID       string // Owner Sone ID
	AlbumID      string // Parent album ID
	Key          string // Freenet URI
	CreationTime int64  // Unix milliseconds
	Title        string
	Description  string
	Width        int
	Height       int
}

// NewImage creates a new image
func NewImage(soneID string) *Image {
	return &Image{
		ID:           generateUUID(),
		SoneID:       soneID,
		CreationTime: time.Now().UnixMilli(),
	}
}

// GetFingerprint returns a hash of the image for change detection
func (i *Image) GetFingerprint() string {
	h := sha256.New()
	h.Write([]byte("Image("))
	h.Write([]byte(fmt.Sprintf("ID(%s)", i.ID)))
	if i.Key != "" {
		h.Write([]byte(fmt.Sprintf("Key(%s)", i.Key)))
	}
	if i.Title != "" {
		h.Write([]byte(fmt.Sprintf("Title(%s)", i.Title)))
	}
	if i.Description != "" {
		h.Write([]byte(fmt.Sprintf("Description(%s)", i.Description)))
	}
	h.Write([]byte(fmt.Sprintf("Width(%d)", i.Width)))
	h.Write([]byte(fmt.Sprintf("Height(%d)", i.Height)))
	h.Write([]byte(")"))
	return hex.EncodeToString(h.Sum(nil))
}

// IsInserted returns true if the image has been inserted to Hyphanet
func (i *Image) IsInserted() bool {
	return i.Key != ""
}

// Helper function to generate UUIDs
func generateUUID() string {
	// Simple UUID v4 implementation
	b := make([]byte, 16)
	// In production, use crypto/rand
	for i := range b {
		b[i] = byte(time.Now().UnixNano() >> (i * 8))
	}
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
