// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package sone

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMemoryDatabaseBasic(t *testing.T) {
	db := NewMemoryDatabase("")

	// Test storing a Sone
	sone := NewSone("test-sone-1")
	sone.Name = "Test User"

	db.StoreSone(sone)

	// Retrieve
	retrieved := db.GetSone("test-sone-1")
	if retrieved == nil {
		t.Fatal("Expected to retrieve stored Sone")
	}

	if retrieved.Name != "Test User" {
		t.Errorf("Expected name 'Test User', got '%s'", retrieved.Name)
	}
}

func TestMemoryDatabasePosts(t *testing.T) {
	db := NewMemoryDatabase("")

	// Store a post
	post := NewPost("sone-1", "Hello, world!")
	db.StorePost(post)

	// Retrieve
	retrieved := db.GetPost(post.ID)
	if retrieved == nil {
		t.Fatal("Expected to retrieve stored post")
	}

	if retrieved.Text != "Hello, world!" {
		t.Errorf("Expected text 'Hello, world!', got '%s'", retrieved.Text)
	}

	// Check if known
	if !db.IsPostKnown(post.ID) {
		t.Error("Expected post to be known")
	}

	if db.IsPostKnown("non-existent") {
		t.Error("Expected non-existent post to be unknown")
	}

	// Remove
	db.RemovePost(post.ID)
	if db.GetPost(post.ID) != nil {
		t.Error("Expected post to be removed")
	}
}

func TestMemoryDatabaseReplies(t *testing.T) {
	db := NewMemoryDatabase("")

	// Store a reply
	reply := NewPostReply("sone-1", "post-1", "This is a reply")
	db.StoreReply(reply)

	// Retrieve
	retrieved := db.GetReply(reply.ID)
	if retrieved == nil {
		t.Fatal("Expected to retrieve stored reply")
	}

	if retrieved.Text != "This is a reply" {
		t.Errorf("Expected text 'This is a reply', got '%s'", retrieved.Text)
	}

	// Check if known
	if !db.IsReplyKnown(reply.ID) {
		t.Error("Expected reply to be known")
	}

	// Remove
	db.RemoveReply(reply.ID)
	if db.GetReply(reply.ID) != nil {
		t.Error("Expected reply to be removed")
	}
}

func TestMemoryDatabaseFriends(t *testing.T) {
	db := NewMemoryDatabase("")

	// Add friend
	db.AddFriend("sone-1", "friend-1")
	db.AddFriend("sone-1", "friend-2")

	friends := db.GetFriends("sone-1")
	if len(friends) != 2 {
		t.Errorf("Expected 2 friends, got %d", len(friends))
	}

	// Check if friend
	if !db.IsFriend("sone-1", "friend-1") {
		t.Error("Expected friend-1 to be a friend")
	}

	if db.IsFriend("sone-1", "friend-3") {
		t.Error("Expected friend-3 to not be a friend")
	}

	// Remove friend
	db.RemoveFriend("sone-1", "friend-1")
	if db.IsFriend("sone-1", "friend-1") {
		t.Error("Expected friend-1 to be removed")
	}
}

func TestMemoryDatabaseBookmarks(t *testing.T) {
	db := NewMemoryDatabase("")

	// Add bookmarks
	db.AddBookmark("post-1")
	db.AddBookmark("post-2")

	bookmarks := db.GetBookmarks()
	if len(bookmarks) != 2 {
		t.Errorf("Expected 2 bookmarks, got %d", len(bookmarks))
	}

	// Check if bookmarked
	if !db.IsBookmarked("post-1") {
		t.Error("Expected post-1 to be bookmarked")
	}

	if db.IsBookmarked("post-3") {
		t.Error("Expected post-3 to not be bookmarked")
	}

	// Remove bookmark
	db.RemoveBookmark("post-1")
	if db.IsBookmarked("post-1") {
		t.Error("Expected post-1 bookmark to be removed")
	}
}

func TestMemoryDatabaseGetAllSones(t *testing.T) {
	db := NewMemoryDatabase("")

	// Store multiple Sones
	for i := 1; i <= 5; i++ {
		sone := NewSone("sone-" + string(rune('0'+i)))
		db.StoreSone(sone)
	}

	all := db.GetAllSones()
	if len(all) != 5 {
		t.Errorf("Expected 5 Sones, got %d", len(all))
	}
}

func TestMemoryDatabasePersistence(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "gosone-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := filepath.Join(tmpDir, "test.json")

	// Create database and add data
	db1 := NewMemoryDatabase(tmpDir)

	sone := NewSone("persist-sone")
	sone.Name = "Persistent User"
	db1.StoreSone(sone)

	post := NewPost("persist-sone", "Persistent post")
	db1.StorePost(post)

	db1.AddBookmark(post.ID)

	// Save
	if err := db1.Save(); err != nil {
		t.Fatalf("Failed to save database: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatal("Expected database file to be created")
	}

	// Load into new database
	db2 := NewMemoryDatabase(tmpDir)
	if err := db2.Load(); err != nil {
		t.Fatalf("Failed to load database: %v", err)
	}

	// Verify data
	loadedSone := db2.GetSone("persist-sone")
	if loadedSone == nil {
		t.Fatal("Expected to load persisted Sone")
	}

	if loadedSone.Name != "Persistent User" {
		t.Errorf("Expected name 'Persistent User', got '%s'", loadedSone.Name)
	}

	loadedPost := db2.GetPost(post.ID)
	if loadedPost == nil {
		t.Fatal("Expected to load persisted post")
	}

	if !db2.IsBookmarked(post.ID) {
		t.Error("Expected bookmark to be persisted")
	}
}

func TestMemoryDatabaseLoadNonExistent(t *testing.T) {
	db := NewMemoryDatabase("/non/existent/path")

	// Should not error, just return empty
	err := db.Load()
	if err != nil && !os.IsNotExist(err) {
		t.Errorf("Expected nil or not-exist error, got: %v", err)
	}
}

func TestMemoryDatabaseConcurrency(t *testing.T) {
	db := NewMemoryDatabase("")

	// Run concurrent operations
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func(id int) {
			sone := NewSone("concurrent-sone-" + string(rune('0'+id)))
			db.StoreSone(sone)
			db.GetSone(sone.ID)
			db.GetAllSones()
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should have all Sones
	all := db.GetAllSones()
	if len(all) != 10 {
		t.Errorf("Expected 10 Sones after concurrent operations, got %d", len(all))
	}
}

func TestMemoryDatabaseUpdateSone(t *testing.T) {
	db := NewMemoryDatabase("")

	// Store initial
	sone := NewSone("update-sone")
	sone.Name = "Initial Name"
	db.StoreSone(sone)

	// Update
	sone.Name = "Updated Name"
	db.StoreSone(sone)

	// Retrieve
	retrieved := db.GetSone("update-sone")
	if retrieved.Name != "Updated Name" {
		t.Errorf("Expected updated name 'Updated Name', got '%s'", retrieved.Name)
	}

	// Should still be only 1 Sone
	all := db.GetAllSones()
	if len(all) != 1 {
		t.Errorf("Expected 1 Sone after update, got %d", len(all))
	}
}
