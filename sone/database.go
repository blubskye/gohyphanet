// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package sone

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Database provides storage for Sone data
type Database interface {
	// Sone operations
	GetSone(id string) *Sone
	GetAllSones() []*Sone
	GetLocalSones() []*Sone
	StoreSone(sone *Sone) error
	RemoveSone(id string) error

	// Post operations
	GetPost(id string) *Post
	GetPostsBySone(soneID string) []*Post
	GetAllPosts() []*Post
	StorePost(post *Post) error
	RemovePost(id string) error
	IsPostKnown(id string) bool
	SetPostKnown(id string, known bool) error

	// Reply operations
	GetReply(id string) *PostReply
	GetRepliesByPost(postID string) []*PostReply
	GetRepliesBySone(soneID string) []*PostReply
	StoreReply(reply *PostReply) error
	RemoveReply(id string) error
	IsReplyKnown(id string) bool
	SetReplyKnown(id string, known bool) error

	// Album operations
	GetAlbum(id string) *Album
	GetAlbumsBySone(soneID string) []*Album
	StoreAlbum(album *Album) error
	RemoveAlbum(id string) error

	// Image operations
	GetImage(id string) *Image
	GetImagesByAlbum(albumID string) []*Image
	StoreImage(image *Image) error
	RemoveImage(id string) error

	// Friends
	GetFriends(soneID string) []string
	AddFriend(soneID, friendID string) error
	RemoveFriend(soneID, friendID string) error
	IsFriend(soneID, friendID string) bool

	// Bookmarks
	GetBookmarkedPosts() []string
	AddBookmark(postID string) error
	RemoveBookmark(postID string) error
	IsBookmarked(postID string) bool

	// Persistence
	Save() error
	Load() error
}

// MemoryDatabase is an in-memory implementation of Database
type MemoryDatabase struct {
	mu sync.RWMutex

	// Sones
	sones map[string]*Sone

	// Posts
	posts       map[string]*Post
	sonePosts   map[string]map[string]*Post // soneID -> postID -> Post
	knownPosts  map[string]bool

	// Replies
	replies       map[string]*PostReply
	postReplies   map[string]map[string]*PostReply // postID -> replyID -> Reply
	soneReplies   map[string]map[string]*PostReply // soneID -> replyID -> Reply
	knownReplies  map[string]bool

	// Albums
	albums     map[string]*Album
	soneAlbums map[string]map[string]*Album

	// Images
	images      map[string]*Image
	albumImages map[string]map[string]*Image

	// Friends (soneID -> set of friend IDs)
	friends map[string]map[string]bool

	// Bookmarks
	bookmarks map[string]bool

	// Persistence
	dataDir string
}

// NewMemoryDatabase creates a new in-memory database
func NewMemoryDatabase(dataDir string) *MemoryDatabase {
	return &MemoryDatabase{
		sones:        make(map[string]*Sone),
		posts:        make(map[string]*Post),
		sonePosts:    make(map[string]map[string]*Post),
		knownPosts:   make(map[string]bool),
		replies:      make(map[string]*PostReply),
		postReplies:  make(map[string]map[string]*PostReply),
		soneReplies:  make(map[string]map[string]*PostReply),
		knownReplies: make(map[string]bool),
		albums:       make(map[string]*Album),
		soneAlbums:   make(map[string]map[string]*Album),
		images:       make(map[string]*Image),
		albumImages:  make(map[string]map[string]*Image),
		friends:      make(map[string]map[string]bool),
		bookmarks:    make(map[string]bool),
		dataDir:      dataDir,
	}
}

// Sone operations

func (db *MemoryDatabase) GetSone(id string) *Sone {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.sones[id]
}

func (db *MemoryDatabase) GetAllSones() []*Sone {
	db.mu.RLock()
	defer db.mu.RUnlock()

	sones := make([]*Sone, 0, len(db.sones))
	for _, s := range db.sones {
		sones = append(sones, s)
	}
	return sones
}

func (db *MemoryDatabase) GetLocalSones() []*Sone {
	db.mu.RLock()
	defer db.mu.RUnlock()

	sones := make([]*Sone, 0)
	for _, s := range db.sones {
		if s.IsLocal {
			sones = append(sones, s)
		}
	}
	return sones
}

func (db *MemoryDatabase) StoreSone(sone *Sone) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.sones[sone.ID] = sone

	// Also store posts and replies
	for _, post := range sone.Posts {
		db.storePostLocked(post)
	}
	for _, reply := range sone.Replies {
		db.storeReplyLocked(reply)
	}

	return nil
}

func (db *MemoryDatabase) RemoveSone(id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	delete(db.sones, id)

	// Remove associated posts
	if posts, ok := db.sonePosts[id]; ok {
		for postID := range posts {
			delete(db.posts, postID)
		}
		delete(db.sonePosts, id)
	}

	// Remove associated replies
	if replies, ok := db.soneReplies[id]; ok {
		for replyID := range replies {
			delete(db.replies, replyID)
		}
		delete(db.soneReplies, id)
	}

	return nil
}

// Post operations

func (db *MemoryDatabase) GetPost(id string) *Post {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.posts[id]
}

func (db *MemoryDatabase) GetPostsBySone(soneID string) []*Post {
	db.mu.RLock()
	defer db.mu.RUnlock()

	posts := make([]*Post, 0)
	if sonePosts, ok := db.sonePosts[soneID]; ok {
		for _, p := range sonePosts {
			posts = append(posts, p)
		}
	}

	// Sort by time, newest first
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Time > posts[j].Time
	})

	return posts
}

func (db *MemoryDatabase) GetAllPosts() []*Post {
	db.mu.RLock()
	defer db.mu.RUnlock()

	posts := make([]*Post, 0, len(db.posts))
	for _, p := range db.posts {
		posts = append(posts, p)
	}

	// Sort by time, newest first
	sort.Slice(posts, func(i, j int) bool {
		return posts[i].Time > posts[j].Time
	})

	return posts
}

func (db *MemoryDatabase) StorePost(post *Post) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.storePostLocked(post)
	return nil
}

func (db *MemoryDatabase) storePostLocked(post *Post) {
	db.posts[post.ID] = post

	if db.sonePosts[post.SoneID] == nil {
		db.sonePosts[post.SoneID] = make(map[string]*Post)
	}
	db.sonePosts[post.SoneID][post.ID] = post
}

func (db *MemoryDatabase) RemovePost(id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if post, ok := db.posts[id]; ok {
		delete(db.posts, id)
		if sonePosts, ok := db.sonePosts[post.SoneID]; ok {
			delete(sonePosts, id)
		}
	}

	return nil
}

func (db *MemoryDatabase) IsPostKnown(id string) bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.knownPosts[id]
}

func (db *MemoryDatabase) SetPostKnown(id string, known bool) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if known {
		db.knownPosts[id] = true
	} else {
		delete(db.knownPosts, id)
	}

	if post, ok := db.posts[id]; ok {
		post.IsKnown = known
	}

	return nil
}

// Reply operations

func (db *MemoryDatabase) GetReply(id string) *PostReply {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.replies[id]
}

func (db *MemoryDatabase) GetRepliesByPost(postID string) []*PostReply {
	db.mu.RLock()
	defer db.mu.RUnlock()

	replies := make([]*PostReply, 0)
	if postReplies, ok := db.postReplies[postID]; ok {
		for _, r := range postReplies {
			replies = append(replies, r)
		}
	}

	// Sort by time, oldest first (chronological)
	sort.Slice(replies, func(i, j int) bool {
		return replies[i].Time < replies[j].Time
	})

	return replies
}

func (db *MemoryDatabase) GetRepliesBySone(soneID string) []*PostReply {
	db.mu.RLock()
	defer db.mu.RUnlock()

	replies := make([]*PostReply, 0)
	if soneReplies, ok := db.soneReplies[soneID]; ok {
		for _, r := range soneReplies {
			replies = append(replies, r)
		}
	}

	return replies
}

func (db *MemoryDatabase) StoreReply(reply *PostReply) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.storeReplyLocked(reply)
	return nil
}

func (db *MemoryDatabase) storeReplyLocked(reply *PostReply) {
	db.replies[reply.ID] = reply

	if db.postReplies[reply.PostID] == nil {
		db.postReplies[reply.PostID] = make(map[string]*PostReply)
	}
	db.postReplies[reply.PostID][reply.ID] = reply

	if db.soneReplies[reply.SoneID] == nil {
		db.soneReplies[reply.SoneID] = make(map[string]*PostReply)
	}
	db.soneReplies[reply.SoneID][reply.ID] = reply
}

func (db *MemoryDatabase) RemoveReply(id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if reply, ok := db.replies[id]; ok {
		delete(db.replies, id)
		if postReplies, ok := db.postReplies[reply.PostID]; ok {
			delete(postReplies, id)
		}
		if soneReplies, ok := db.soneReplies[reply.SoneID]; ok {
			delete(soneReplies, id)
		}
	}

	return nil
}

func (db *MemoryDatabase) IsReplyKnown(id string) bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.knownReplies[id]
}

func (db *MemoryDatabase) SetReplyKnown(id string, known bool) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if known {
		db.knownReplies[id] = true
	} else {
		delete(db.knownReplies, id)
	}

	if reply, ok := db.replies[id]; ok {
		reply.IsKnown = known
	}

	return nil
}

// Album operations

func (db *MemoryDatabase) GetAlbum(id string) *Album {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.albums[id]
}

func (db *MemoryDatabase) GetAlbumsBySone(soneID string) []*Album {
	db.mu.RLock()
	defer db.mu.RUnlock()

	albums := make([]*Album, 0)
	if soneAlbums, ok := db.soneAlbums[soneID]; ok {
		for _, a := range soneAlbums {
			albums = append(albums, a)
		}
	}

	return albums
}

func (db *MemoryDatabase) StoreAlbum(album *Album) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.albums[album.ID] = album

	if db.soneAlbums[album.SoneID] == nil {
		db.soneAlbums[album.SoneID] = make(map[string]*Album)
	}
	db.soneAlbums[album.SoneID][album.ID] = album

	return nil
}

func (db *MemoryDatabase) RemoveAlbum(id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if album, ok := db.albums[id]; ok {
		delete(db.albums, id)
		if soneAlbums, ok := db.soneAlbums[album.SoneID]; ok {
			delete(soneAlbums, id)
		}
	}

	return nil
}

// Image operations

func (db *MemoryDatabase) GetImage(id string) *Image {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.images[id]
}

func (db *MemoryDatabase) GetImagesByAlbum(albumID string) []*Image {
	db.mu.RLock()
	defer db.mu.RUnlock()

	images := make([]*Image, 0)
	if albumImages, ok := db.albumImages[albumID]; ok {
		for _, img := range albumImages {
			images = append(images, img)
		}
	}

	return images
}

func (db *MemoryDatabase) StoreImage(image *Image) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	db.images[image.ID] = image

	if db.albumImages[image.AlbumID] == nil {
		db.albumImages[image.AlbumID] = make(map[string]*Image)
	}
	db.albumImages[image.AlbumID][image.ID] = image

	return nil
}

func (db *MemoryDatabase) RemoveImage(id string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if image, ok := db.images[id]; ok {
		delete(db.images, id)
		if albumImages, ok := db.albumImages[image.AlbumID]; ok {
			delete(albumImages, id)
		}
	}

	return nil
}

// Friends operations

func (db *MemoryDatabase) GetFriends(soneID string) []string {
	db.mu.RLock()
	defer db.mu.RUnlock()

	friends := make([]string, 0)
	if soneFriends, ok := db.friends[soneID]; ok {
		for friendID := range soneFriends {
			friends = append(friends, friendID)
		}
	}

	return friends
}

func (db *MemoryDatabase) AddFriend(soneID, friendID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if db.friends[soneID] == nil {
		db.friends[soneID] = make(map[string]bool)
	}
	db.friends[soneID][friendID] = true

	return nil
}

func (db *MemoryDatabase) RemoveFriend(soneID, friendID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()

	if soneFriends, ok := db.friends[soneID]; ok {
		delete(soneFriends, friendID)
	}

	return nil
}

func (db *MemoryDatabase) IsFriend(soneID, friendID string) bool {
	db.mu.RLock()
	defer db.mu.RUnlock()

	if soneFriends, ok := db.friends[soneID]; ok {
		return soneFriends[friendID]
	}

	return false
}

// Bookmarks operations

func (db *MemoryDatabase) GetBookmarkedPosts() []string {
	db.mu.RLock()
	defer db.mu.RUnlock()

	bookmarks := make([]string, 0, len(db.bookmarks))
	for postID := range db.bookmarks {
		bookmarks = append(bookmarks, postID)
	}

	return bookmarks
}

func (db *MemoryDatabase) AddBookmark(postID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	db.bookmarks[postID] = true
	return nil
}

func (db *MemoryDatabase) RemoveBookmark(postID string) error {
	db.mu.Lock()
	defer db.mu.Unlock()
	delete(db.bookmarks, postID)
	return nil
}

func (db *MemoryDatabase) IsBookmarked(postID string) bool {
	db.mu.RLock()
	defer db.mu.RUnlock()
	return db.bookmarks[postID]
}

// Persistence

// DatabaseState represents the serializable state of the database
type DatabaseState struct {
	KnownPosts   []string          `json:"known_posts"`
	KnownReplies []string          `json:"known_replies"`
	Bookmarks    []string          `json:"bookmarks"`
	Friends      map[string][]string `json:"friends"`
}

func (db *MemoryDatabase) Save() error {
	if db.dataDir == "" {
		return nil // No persistence configured
	}

	db.mu.RLock()
	defer db.mu.RUnlock()

	state := &DatabaseState{
		KnownPosts:   make([]string, 0, len(db.knownPosts)),
		KnownReplies: make([]string, 0, len(db.knownReplies)),
		Bookmarks:    make([]string, 0, len(db.bookmarks)),
		Friends:      make(map[string][]string),
	}

	for id := range db.knownPosts {
		state.KnownPosts = append(state.KnownPosts, id)
	}

	for id := range db.knownReplies {
		state.KnownReplies = append(state.KnownReplies, id)
	}

	for id := range db.bookmarks {
		state.Bookmarks = append(state.Bookmarks, id)
	}

	for soneID, friends := range db.friends {
		friendList := make([]string, 0, len(friends))
		for friendID := range friends {
			friendList = append(friendList, friendID)
		}
		state.Friends[soneID] = friendList
	}

	// Ensure directory exists
	if err := os.MkdirAll(db.dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Write to file
	filePath := filepath.Join(db.dataDir, "sone_state.json")
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

func (db *MemoryDatabase) Load() error {
	if db.dataDir == "" {
		return nil // No persistence configured
	}

	filePath := filepath.Join(db.dataDir, "sone_state.json")

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No state file yet
		}
		return fmt.Errorf("failed to read state file: %w", err)
	}

	var state DatabaseState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to unmarshal state: %w", err)
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	for _, id := range state.KnownPosts {
		db.knownPosts[id] = true
	}

	for _, id := range state.KnownReplies {
		db.knownReplies[id] = true
	}

	for _, id := range state.Bookmarks {
		db.bookmarks[id] = true
	}

	for soneID, friendList := range state.Friends {
		db.friends[soneID] = make(map[string]bool)
		for _, friendID := range friendList {
			db.friends[soneID][friendID] = true
		}
	}

	return nil
}
