// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package sone

import (
	"strings"
	"testing"
)

// Sample Sone XML for testing
const sampleSoneXML = `<?xml version="1.0" encoding="UTF-8"?>
<sone>
    <time>1700000000000</time>
    <protocol-version>0</protocol-version>
    <client>
        <name>TestClient</name>
        <version>1.0</version>
    </client>
    <profile>
        <first-name>John</first-name>
        <middle-name>Q</middle-name>
        <last-name>Public</last-name>
        <birth-day>15</birth-day>
        <birth-month>6</birth-month>
        <birth-year>1990</birth-year>
        <avatar>CHK@test-avatar-key</avatar>
        <fields>
            <field>
                <field-name>Website</field-name>
                <field-value>http://example.com</field-value>
            </field>
        </fields>
    </profile>
    <posts>
        <post>
            <id>post-123</id>
            <time>1699999000000</time>
            <recipient>recipient-sone-id</recipient>
            <text>Hello, world!</text>
        </post>
        <post>
            <id>post-456</id>
            <time>1699998000000</time>
            <text>Another post without recipient</text>
        </post>
    </posts>
    <replies>
        <reply>
            <id>reply-789</id>
            <post-id>post-123</post-id>
            <time>1699999500000</time>
            <text>This is a reply</text>
        </reply>
    </replies>
    <post-likes>
        <post-like>liked-post-1</post-like>
        <post-like>liked-post-2</post-like>
    </post-likes>
    <reply-likes>
        <reply-like>liked-reply-1</reply-like>
    </reply-likes>
    <albums>
        <album>
            <id>album-1</id>
            <title>My Album</title>
            <description>Album description</description>
            <images>
                <image>
                    <id>img-1</id>
                    <creation-time>1699990000000</creation-time>
                    <key>CHK@image-key</key>
                    <title>Test Image</title>
                    <description>Image description</description>
                    <width>800</width>
                    <height>600</height>
                </image>
            </images>
        </album>
    </albums>
</sone>`

func TestParseSoneXML(t *testing.T) {
	sone, err := ParseSoneXML([]byte(sampleSoneXML), "test-sone-id")
	if err != nil {
		t.Fatalf("Failed to parse Sone XML: %v", err)
	}

	// Check basic fields
	if sone.ID != "test-sone-id" {
		t.Errorf("Expected ID 'test-sone-id', got '%s'", sone.ID)
	}

	if sone.Time != 1700000000000 {
		t.Errorf("Expected Time 1700000000000, got %d", sone.Time)
	}

	// Check client info
	if sone.Client == nil {
		t.Fatal("Expected Client to be set")
	}

	if sone.Client.Name != "TestClient" {
		t.Errorf("Expected Client.Name 'TestClient', got '%s'", sone.Client.Name)
	}

	// Check profile
	if sone.Profile == nil {
		t.Fatal("Expected Profile to be set")
	}

	if sone.Profile.FirstName != "John" {
		t.Errorf("Expected FirstName 'John', got '%s'", sone.Profile.FirstName)
	}

	if sone.Profile.LastName != "Public" {
		t.Errorf("Expected LastName 'Public', got '%s'", sone.Profile.LastName)
	}

	if sone.Profile.BirthYear != 1990 {
		t.Errorf("Expected BirthYear 1990, got %d", sone.Profile.BirthYear)
	}

	// Check profile fields
	if len(sone.Profile.Fields) != 1 {
		t.Errorf("Expected 1 profile field, got %d", len(sone.Profile.Fields))
	} else {
		if sone.Profile.Fields[0].Name != "Website" {
			t.Errorf("Expected field name 'Website', got '%s'", sone.Profile.Fields[0].Name)
		}
	}

	// Check posts
	if len(sone.Posts) != 2 {
		t.Errorf("Expected 2 posts, got %d", len(sone.Posts))
	} else {
		if sone.Posts[0].ID != "post-123" {
			t.Errorf("Expected first post ID 'post-123', got '%s'", sone.Posts[0].ID)
		}
		if sone.Posts[0].Text != "Hello, world!" {
			t.Errorf("Expected first post text 'Hello, world!', got '%s'", sone.Posts[0].Text)
		}
		if sone.Posts[0].RecipientID == nil || *sone.Posts[0].RecipientID != "recipient-sone-id" {
			t.Error("Expected first post to have recipient 'recipient-sone-id'")
		}
		if sone.Posts[1].RecipientID != nil {
			t.Error("Expected second post to have no recipient")
		}
	}

	// Check replies
	if len(sone.Replies) != 1 {
		t.Errorf("Expected 1 reply, got %d", len(sone.Replies))
	} else {
		if sone.Replies[0].ID != "reply-789" {
			t.Errorf("Expected reply ID 'reply-789', got '%s'", sone.Replies[0].ID)
		}
		if sone.Replies[0].PostID != "post-123" {
			t.Errorf("Expected reply PostID 'post-123', got '%s'", sone.Replies[0].PostID)
		}
	}

	// Check likes
	if len(sone.LikedPostIDs) != 2 {
		t.Errorf("Expected 2 liked posts, got %d", len(sone.LikedPostIDs))
	}
	if _, ok := sone.LikedPostIDs["liked-post-1"]; !ok {
		t.Error("Expected liked-post-1 to be in LikedPostIDs")
	}

	if len(sone.LikedReplyIDs) != 1 {
		t.Errorf("Expected 1 liked reply, got %d", len(sone.LikedReplyIDs))
	}

	// Check albums
	if sone.RootAlbum == nil {
		t.Fatal("Expected RootAlbum to be set")
	}
	if len(sone.RootAlbum.Albums) != 1 {
		t.Errorf("Expected 1 album, got %d", len(sone.RootAlbum.Albums))
	} else {
		album := sone.RootAlbum.Albums[0]
		if album.Title != "My Album" {
			t.Errorf("Expected album title 'My Album', got '%s'", album.Title)
		}
		if len(album.Images) != 1 {
			t.Errorf("Expected 1 image in album, got %d", len(album.Images))
		}
	}
}

func TestToXML(t *testing.T) {
	// Create a Sone
	sone := NewSone("test-sone-id")
	sone.Name = "Test User"
	sone.Time = 1700000000000
	sone.Profile = &Profile{
		FirstName: "Test",
		LastName:  "User",
	}
	sone.Posts = append(sone.Posts, &Post{
		ID:     "post-1",
		SoneID: sone.ID,
		Time:   1699999000000,
		Text:   "Test post",
	})

	// Generate XML
	xml, err := sone.ToXML("GoSone", "0.1.0")
	if err != nil {
		t.Fatalf("Failed to generate XML: %v", err)
	}

	// Verify XML structure
	xmlStr := string(xml)

	if !strings.Contains(xmlStr, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>") {
		t.Error("Expected XML declaration")
	}

	if !strings.Contains(xmlStr, "<sone>") {
		t.Error("Expected <sone> root element")
	}

	if !strings.Contains(xmlStr, "<protocol-version>0</protocol-version>") {
		t.Error("Expected protocol version 0")
	}

	if !strings.Contains(xmlStr, "<first-name>Test</first-name>") {
		t.Error("Expected profile first name")
	}

	if !strings.Contains(xmlStr, "<text>Test post</text>") {
		t.Error("Expected post text")
	}

	if !strings.Contains(xmlStr, "<name>GoSone</name>") {
		t.Error("Expected client name")
	}
}

func TestRoundTrip(t *testing.T) {
	// Create a Sone
	original := NewSone("roundtrip-sone")
	original.Time = 1700000000000
	original.Profile = &Profile{
		FirstName:  "Round",
		LastName:   "Trip",
		BirthYear:  1995,
		BirthMonth: 3,
		BirthDay:   20,
		Fields: []*Field{
			{Name: "Location", Value: "Earth"},
		},
	}
	original.Posts = append(original.Posts, &Post{
		ID:     "post-rt-1",
		SoneID: original.ID,
		Time:   1699999000000,
		Text:   "Round trip test",
	})
	original.Replies = append(original.Replies, &PostReply{
		ID:     "reply-rt-1",
		SoneID: original.ID,
		PostID: "post-rt-1",
		Time:   1699999500000,
		Text:   "Reply in round trip",
	})
	original.LikePost("external-post")
	original.AddFriend("friend-sone")

	// Convert to XML
	xml, err := original.ToXML("GoSone", "0.1.0")
	if err != nil {
		t.Fatalf("Failed to generate XML: %v", err)
	}

	// Parse back
	parsed, err := ParseSoneXML(xml, original.ID)
	if err != nil {
		t.Fatalf("Failed to parse XML: %v", err)
	}

	// Verify
	if parsed.Time != original.Time {
		t.Errorf("Time mismatch: expected %d, got %d", original.Time, parsed.Time)
	}

	if parsed.Profile.FirstName != original.Profile.FirstName {
		t.Errorf("FirstName mismatch: expected %s, got %s", original.Profile.FirstName, parsed.Profile.FirstName)
	}

	if len(parsed.Posts) != len(original.Posts) {
		t.Errorf("Posts count mismatch: expected %d, got %d", len(original.Posts), len(parsed.Posts))
	}

	if len(parsed.Replies) != len(original.Replies) {
		t.Errorf("Replies count mismatch: expected %d, got %d", len(original.Replies), len(parsed.Replies))
	}

	if !parsed.HasLikedPost("external-post") {
		t.Error("Expected external-post to be liked")
	}
}

func TestParseMalformedXML(t *testing.T) {
	malformed := []byte(`<?xml version="1.0"?><sone><unclosed>`)

	_, err := ParseSoneXML(malformed, "test")
	if err == nil {
		t.Error("Expected error for malformed XML")
	}
}

func TestParseEmptyXML(t *testing.T) {
	empty := []byte(`<?xml version="1.0"?><sone></sone>`)

	sone, err := ParseSoneXML(empty, "test-id")
	if err != nil {
		t.Fatalf("Failed to parse empty XML: %v", err)
	}

	if sone.ID != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", sone.ID)
	}
}
