// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package sone

import (
	"strings"
	"testing"
)

func TestTextParserPlainText(t *testing.T) {
	parser := NewTextParser(nil, nil)

	text := "Hello, world!"
	parts := parser.Parse(text)

	if len(parts) != 1 {
		t.Errorf("Expected 1 part, got %d", len(parts))
	}

	if parts[0].Type() != PartPlainText {
		t.Errorf("Expected PartPlainText, got %d", parts[0].Type())
	}

	if parts[0].Text() != text {
		t.Errorf("Expected '%s', got '%s'", text, parts[0].Text())
	}
}

func TestTextParserFreenetLinks(t *testing.T) {
	parser := NewTextParser(nil, nil)

	testCases := []struct {
		input    string
		expected PartType
		linkType string
	}{
		{"Check out USK@abc123/site/0/", PartFreenetLink, "USK"},
		{"SSK@abc123/doc", PartFreenetLink, "SSK"},
		{"CHK@abc123", PartFreenetLink, "CHK"},
		{"KSK@mykey", PartFreenetLink, "KSK"},
	}

	for _, tc := range testCases {
		parts := parser.Parse(tc.input)

		found := false
		for _, part := range parts {
			if part.Type() == tc.expected {
				found = true
				if !strings.Contains(part.Text(), tc.linkType+"@") {
					t.Errorf("Expected link type %s in '%s'", tc.linkType, part.Text())
				}
				break
			}
		}

		if !found {
			t.Errorf("Expected to find %s link in '%s'", tc.linkType, tc.input)
		}
	}
}

func TestTextParserSoneLinks(t *testing.T) {
	// Create parser with resolver
	testSone := NewSone("test-sone-123")
	testSone.Name = "TestUser"

	parser := NewTextParser(
		func(id string) *Sone {
			if id == "test-sone-123" {
				return testSone
			}
			return nil
		},
		nil,
	)

	text := "Check out sone://test-sone-123 for more info"
	parts := parser.Parse(text)

	found := false
	for _, part := range parts {
		if part.Type() == PartSoneLink {
			found = true
			if sl, ok := part.(*SoneLinkPart); ok {
				if sl.SoneID != "test-sone-123" {
					t.Errorf("Expected SoneID 'test-sone-123', got '%s'", sl.SoneID)
				}
				if sl.Sone == nil {
					t.Error("Expected Sone to be resolved")
				} else if sl.Sone.Name != "TestUser" {
					t.Errorf("Expected Sone name 'TestUser', got '%s'", sl.Sone.Name)
				}
			}
			break
		}
	}

	if !found {
		t.Error("Expected to find SoneLink")
	}
}

func TestTextParserPostLinks(t *testing.T) {
	testPost := &Post{
		ID:     "test-post-456",
		SoneID: "author-sone",
		Text:   "Original post",
	}

	parser := NewTextParser(
		nil,
		func(id string) *Post {
			if id == "test-post-456" {
				return testPost
			}
			return nil
		},
	)

	text := "See post://test-post-456 for details"
	parts := parser.Parse(text)

	found := false
	for _, part := range parts {
		if part.Type() == PartPostLink {
			found = true
			if pl, ok := part.(*PostLinkPart); ok {
				if pl.PostID != "test-post-456" {
					t.Errorf("Expected PostID 'test-post-456', got '%s'", pl.PostID)
				}
				if pl.Post == nil {
					t.Error("Expected Post to be resolved")
				}
			}
			break
		}
	}

	if !found {
		t.Error("Expected to find PostLink")
	}
}

func TestTextParserHTTPLinks(t *testing.T) {
	parser := NewTextParser(nil, nil)

	testCases := []string{
		"Visit https://example.com for more",
		"Check http://example.org/page",
		"HTTPS://UPPERCASE.COM works too",
	}

	for _, tc := range testCases {
		parts := parser.Parse(tc)

		found := false
		for _, part := range parts {
			if part.Type() == PartHTTPLink {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Expected to find HTTP link in '%s'", tc)
		}
	}
}

func TestTextParserFreemailLinks(t *testing.T) {
	parser := NewTextParser(nil, nil)

	text := "Email me at user@abc123.freemail"
	parts := parser.Parse(text)

	found := false
	for _, part := range parts {
		if part.Type() == PartFreemail {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find Freemail link")
	}
}

func TestTextParserMixedContent(t *testing.T) {
	parser := NewTextParser(nil, nil)

	text := "Hello! Check USK@key/site/0 and https://example.com for info."
	parts := parser.Parse(text)

	// Should have at least: plain, freenet link, plain, http link, plain
	if len(parts) < 3 {
		t.Errorf("Expected at least 3 parts for mixed content, got %d", len(parts))
	}

	hasPlain := false
	hasFreenet := false
	hasHTTP := false

	for _, part := range parts {
		switch part.Type() {
		case PartPlainText:
			hasPlain = true
		case PartFreenetLink:
			hasFreenet = true
		case PartHTTPLink:
			hasHTTP = true
		}
	}

	if !hasPlain {
		t.Error("Expected plain text parts")
	}
	if !hasFreenet {
		t.Error("Expected Freenet link part")
	}
	if !hasHTTP {
		t.Error("Expected HTTP link part")
	}
}

func TestRenderPartsToHTML(t *testing.T) {
	parser := NewTextParser(nil, nil)

	text := "Hello! Visit https://example.com"
	parts := parser.Parse(text)

	html := RenderPartsToHTML(parts)

	if !strings.Contains(html, "<a") {
		t.Error("Expected HTML link tag")
	}

	if !strings.Contains(html, "href=") {
		t.Error("Expected href attribute")
	}

	if !strings.Contains(html, "Hello!") {
		t.Error("Expected plain text to be preserved")
	}
}

func TestRenderPartsToPlainText(t *testing.T) {
	parser := NewTextParser(nil, nil)

	text := "Hello! Visit https://example.com"
	parts := parser.Parse(text)

	plain := RenderPartsToPlainText(parts)

	// Should just concatenate text
	if !strings.Contains(plain, "Hello!") {
		t.Error("Expected 'Hello!' in plain text")
	}

	if !strings.Contains(plain, "https://example.com") {
		t.Error("Expected URL in plain text")
	}
}

func TestMentionDetector(t *testing.T) {
	sone1 := NewSone("sone-1")
	sone1.Name = "Alice"

	sone2 := NewSone("sone-2")
	sone2.Name = "Bob"

	detector := NewMentionDetector([]*Sone{sone1, sone2})

	testCases := []struct {
		text     string
		expected []string
	}{
		{"Hello @Alice!", []string{"sone-1"}},
		{"@Bob and @Alice are here", []string{"sone-2", "sone-1"}},
		{"No mentions here", []string{}},
		{"@Unknown person", []string{}},
	}

	for _, tc := range testCases {
		mentions := detector.DetectMentions(tc.text)

		if len(mentions) != len(tc.expected) {
			t.Errorf("For '%s': expected %d mentions, got %d", tc.text, len(tc.expected), len(mentions))
			continue
		}

		for i, expectedID := range tc.expected {
			if mentions[i] != expectedID {
				t.Errorf("For '%s': expected mention %d to be '%s', got '%s'", tc.text, i, expectedID, mentions[i])
			}
		}
	}
}

func TestEmptyText(t *testing.T) {
	parser := NewTextParser(nil, nil)

	parts := parser.Parse("")

	if len(parts) != 0 {
		t.Errorf("Expected 0 parts for empty text, got %d", len(parts))
	}
}

func TestOnlyWhitespace(t *testing.T) {
	parser := NewTextParser(nil, nil)

	parts := parser.Parse("   \t\n   ")

	// Should return the whitespace as plain text
	if len(parts) != 1 {
		t.Errorf("Expected 1 part for whitespace, got %d", len(parts))
	}

	if parts[0].Type() != PartPlainText {
		t.Error("Expected whitespace to be plain text")
	}
}

func TestSpecialCharactersInText(t *testing.T) {
	parser := NewTextParser(nil, nil)

	text := "<script>alert('xss')</script>"
	parts := parser.Parse(text)

	html := RenderPartsToHTML(parts)

	// Should escape HTML characters
	if strings.Contains(html, "<script>") {
		t.Error("Expected script tag to be escaped")
	}

	if !strings.Contains(html, "&lt;script&gt;") {
		t.Error("Expected HTML-escaped text")
	}
}
