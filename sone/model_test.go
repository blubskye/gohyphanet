// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package sone

import (
	"testing"
	"time"
)

func TestNewSone(t *testing.T) {
	id := "test-sone-id-12345"
	s := NewSone(id)

	if s.ID != id {
		t.Errorf("Expected ID %s, got %s", id, s.ID)
	}

	if s.Status != StatusUnknown {
		t.Errorf("Expected status Unknown, got %d", s.Status)
	}

	if s.Friends == nil {
		t.Error("Expected Friends map to be initialized")
	}

	if s.LikedPostIDs == nil {
		t.Error("Expected LikedPostIDs map to be initialized")
	}

	if s.LikedReplyIDs == nil {
		t.Error("Expected LikedReplyIDs map to be initialized")
	}
}

func TestNewPost(t *testing.T) {
	soneID := "sone-123"
	text := "Hello, world!"

	post := NewPost(soneID, text)

	if post.SoneID != soneID {
		t.Errorf("Expected SoneID %s, got %s", soneID, post.SoneID)
	}

	if post.Text != text {
		t.Errorf("Expected Text %s, got %s", text, post.Text)
	}

	if post.ID == "" {
		t.Error("Expected post ID to be generated")
	}

	if post.Time == 0 {
		t.Error("Expected Time to be set")
	}
}

func TestNewPostReply(t *testing.T) {
	soneID := "sone-123"
	postID := "post-456"
	text := "This is a reply"

	reply := NewPostReply(soneID, postID, text)

	if reply.SoneID != soneID {
		t.Errorf("Expected SoneID %s, got %s", soneID, reply.SoneID)
	}

	if reply.PostID != postID {
		t.Errorf("Expected PostID %s, got %s", postID, reply.PostID)
	}

	if reply.Text != text {
		t.Errorf("Expected Text %s, got %s", text, reply.Text)
	}

	if reply.ID == "" {
		t.Error("Expected reply ID to be generated")
	}
}

func TestSoneFriends(t *testing.T) {
	s := NewSone("test-sone")

	// Add friend
	s.AddFriend("friend-1")

	if !s.IsFriend("friend-1") {
		t.Error("Expected friend-1 to be a friend")
	}

	if s.IsFriend("friend-2") {
		t.Error("Expected friend-2 to not be a friend")
	}

	// Add another friend
	s.AddFriend("friend-2")

	if !s.IsFriend("friend-2") {
		t.Error("Expected friend-2 to be a friend after adding")
	}

	// Remove friend
	s.RemoveFriend("friend-1")

	if s.IsFriend("friend-1") {
		t.Error("Expected friend-1 to not be a friend after removal")
	}
}

func TestSoneLikes(t *testing.T) {
	s := NewSone("test-sone")

	// Like a post
	s.LikePost("post-1")

	if !s.HasLikedPost("post-1") {
		t.Error("Expected post-1 to be liked")
	}

	if s.HasLikedPost("post-2") {
		t.Error("Expected post-2 to not be liked")
	}

	// Unlike post
	s.UnlikePost("post-1")

	if s.HasLikedPost("post-1") {
		t.Error("Expected post-1 to not be liked after unliking")
	}

	// Like a reply
	s.LikeReply("reply-1")

	if !s.HasLikedReply("reply-1") {
		t.Error("Expected reply-1 to be liked")
	}

	s.UnlikeReply("reply-1")

	if s.HasLikedReply("reply-1") {
		t.Error("Expected reply-1 to not be liked after unliking")
	}
}

func TestSoneFingerprint(t *testing.T) {
	s := NewSone("test-sone")
	s.Name = "Test User"

	// Get initial fingerprint
	fp1 := s.GetFingerprint()

	if fp1 == "" {
		t.Error("Expected fingerprint to not be empty")
	}

	// Same state should produce same fingerprint
	fp2 := s.GetFingerprint()

	if fp1 != fp2 {
		t.Error("Expected same fingerprint for same state")
	}

	// Change state
	s.Posts = append(s.Posts, NewPost(s.ID, "Hello"))
	fp3 := s.GetFingerprint()

	if fp1 == fp3 {
		t.Error("Expected different fingerprint after adding post")
	}
}

func TestProfile(t *testing.T) {
	s := NewSone("test-sone")
	s.Profile = &Profile{
		FirstName:  "John",
		LastName:   "Doe",
		BirthYear:  1990,
		BirthMonth: 5,
		BirthDay:   15,
	}

	// Add custom field
	s.Profile.Fields = append(s.Profile.Fields, &Field{
		Name:  "Website",
		Value: "https://example.com",
	})

	if s.Profile.FirstName != "John" {
		t.Errorf("Expected FirstName John, got %s", s.Profile.FirstName)
	}

	if len(s.Profile.Fields) != 1 {
		t.Errorf("Expected 1 field, got %d", len(s.Profile.Fields))
	}

	if s.Profile.Fields[0].Name != "Website" {
		t.Errorf("Expected field name Website, got %s", s.Profile.Fields[0].Name)
	}
}

func TestAlbumAndImage(t *testing.T) {
	album := &Album{
		ID:     "album-1",
		SoneID: "test-sone",
		Title:  "Test Album",
	}

	img := &Image{
		ID:       "img-1",
		SoneID:   "test-sone",
		AlbumID:  "album-1",
		Key:      "CHK@...",
		Title:    "Test Image",
		Width:    800,
		Height:   600,
		MimeType: "image/jpeg",
	}

	album.Images = append(album.Images, img)

	if len(album.Images) != 1 {
		t.Errorf("Expected 1 image, got %d", len(album.Images))
	}

	if album.Images[0].Title != "Test Image" {
		t.Errorf("Expected image title 'Test Image', got %s", album.Images[0].Title)
	}

	// Test nested albums
	childAlbum := &Album{
		ID:     "album-2",
		SoneID: "test-sone",
		Title:  "Child Album",
	}

	album.Albums = append(album.Albums, childAlbum)

	if len(album.Albums) != 1 {
		t.Errorf("Expected 1 child album, got %d", len(album.Albums))
	}
}

func TestSoneStatus(t *testing.T) {
	testCases := []struct {
		status   SoneStatus
		expected string
	}{
		{StatusUnknown, "Unknown"},
		{StatusIdle, "Idle"},
		{StatusInserting, "Inserting"},
		{StatusDownloading, "Downloading"},
	}

	for _, tc := range testCases {
		if tc.status.String() != tc.expected {
			t.Errorf("Expected status string %s, got %s", tc.expected, tc.status.String())
		}
	}
}

func TestPostReplies(t *testing.T) {
	post := NewPost("sone-1", "Original post")

	// Add replies
	reply1 := NewPostReply("sone-2", post.ID, "Reply 1")
	reply2 := NewPostReply("sone-3", post.ID, "Reply 2")

	post.Replies = append(post.Replies, reply1, reply2)

	if len(post.Replies) != 2 {
		t.Errorf("Expected 2 replies, got %d", len(post.Replies))
	}

	if post.Replies[0].Text != "Reply 1" {
		t.Errorf("Expected first reply text 'Reply 1', got %s", post.Replies[0].Text)
	}
}

func TestPostTime(t *testing.T) {
	before := time.Now().UnixMilli()
	post := NewPost("sone-1", "Test")
	after := time.Now().UnixMilli()

	if post.Time < before || post.Time > after {
		t.Errorf("Post time %d should be between %d and %d", post.Time, before, after)
	}
}
