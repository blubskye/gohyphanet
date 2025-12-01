// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package sone

import (
	"html"
	"regexp"
	"strings"
	"unicode"
)

// PartType represents the type of parsed text part
type PartType int

const (
	PartPlainText PartType = iota
	PartFreenetLink
	PartSoneLink
	PartPostLink
	PartHTTPLink
	PartFreemail
)

// Part represents a parsed piece of text
type Part interface {
	Type() PartType
	Text() string
	ToHTML() string
	ToPlainText() string
}

// PlainTextPart represents plain text
type PlainTextPart struct {
	Content string
}

func (p *PlainTextPart) Type() PartType     { return PartPlainText }
func (p *PlainTextPart) Text() string       { return p.Content }
func (p *PlainTextPart) ToPlainText() string { return p.Content }
func (p *PlainTextPart) ToHTML() string     { return html.EscapeString(p.Content) }

// FreenetLinkPart represents a Freenet URI (KSK@, SSK@, USK@, CHK@)
type FreenetLinkPart struct {
	Link        string // Full URI
	DisplayText string // Shortened display text
	Trusted     bool   // True if from trusted Sone
}

func (p *FreenetLinkPart) Type() PartType     { return PartFreenetLink }
func (p *FreenetLinkPart) Text() string       { return p.Link }
func (p *FreenetLinkPart) ToPlainText() string { return p.Link }
func (p *FreenetLinkPart) ToHTML() string {
	class := "freenet-link"
	if p.Trusted {
		class += " trusted"
	}
	return `<a class="` + class + `" href="/` + html.EscapeString(p.Link) + `">` + html.EscapeString(p.DisplayText) + `</a>`
}

// SoneLinkPart represents a sone:// link
type SoneLinkPart struct {
	SoneID   string
	SoneName string // Resolved name if available
}

func (p *SoneLinkPart) Type() PartType     { return PartSoneLink }
func (p *SoneLinkPart) Text() string       { return "sone://" + p.SoneID }
func (p *SoneLinkPart) ToPlainText() string {
	if p.SoneName != "" {
		return p.SoneName
	}
	return p.SoneID
}
func (p *SoneLinkPart) ToHTML() string {
	name := p.SoneName
	if name == "" {
		name = p.SoneID[:8] + "..."
	}
	return `<a class="sone-link" href="/sone/` + html.EscapeString(p.SoneID) + `">` + html.EscapeString(name) + `</a>`
}

// PostLinkPart represents a post:// link
type PostLinkPart struct {
	PostID     string
	PostAuthor string // Resolved author name if available
}

func (p *PostLinkPart) Type() PartType     { return PartPostLink }
func (p *PostLinkPart) Text() string       { return "post://" + p.PostID }
func (p *PostLinkPart) ToPlainText() string { return "[post]" }
func (p *PostLinkPart) ToHTML() string {
	text := "post"
	if p.PostAuthor != "" {
		text = p.PostAuthor + "'s post"
	}
	return `<a class="post-link" href="/post/` + html.EscapeString(p.PostID) + `">` + html.EscapeString(text) + `</a>`
}

// HTTPLinkPart represents an HTTP/HTTPS URL
type HTTPLinkPart struct {
	URL         string
	DisplayText string
}

func (p *HTTPLinkPart) Type() PartType     { return PartHTTPLink }
func (p *HTTPLinkPart) Text() string       { return p.URL }
func (p *HTTPLinkPart) ToPlainText() string { return p.URL }
func (p *HTTPLinkPart) ToHTML() string {
	return `<a class="http-link" href="` + html.EscapeString(p.URL) + `" rel="nofollow noopener" target="_blank">` + html.EscapeString(p.DisplayText) + `</a>`
}

// FreemailPart represents a Freemail address
type FreemailPart struct {
	LocalPart  string // Part before @
	FreemailID string // Base32 identity
	IdentityID string // Decoded identity ID
}

func (p *FreemailPart) Type() PartType     { return PartFreemail }
func (p *FreemailPart) Text() string       { return p.LocalPart + "@" + p.FreemailID + ".freemail" }
func (p *FreemailPart) ToPlainText() string { return p.Text() }
func (p *FreemailPart) ToHTML() string {
	return `<a class="freemail-link" href="/Freemail/NewMessage?to=` + html.EscapeString(p.IdentityID) + `">` + html.EscapeString(p.Text()) + `</a>`
}

// TextParser parses Sone text into parts
type TextParser struct {
	soneResolver func(id string) *Sone
	postResolver func(id string) *Post
}

// NewTextParser creates a new text parser
func NewTextParser(soneResolver func(id string) *Sone, postResolver func(id string) *Post) *TextParser {
	return &TextParser{
		soneResolver: soneResolver,
		postResolver: postResolver,
	}
}

// Parse parses text into a slice of parts
func (tp *TextParser) Parse(text string) []Part {
	// Split into lines, trim empty lines at start/end
	lines := strings.Split(text, "\n")
	lines = trimEmptyLines(lines)
	lines = mergeMultipleEmptyLines(lines)

	// Parse each line
	var allParts []Part
	for i, line := range lines {
		if i > 0 {
			allParts = append(allParts, &PlainTextPart{Content: "\n"})
		}
		parts := tp.parseLine(line)
		allParts = append(allParts, parts...)
	}

	// Clean up
	allParts = removeEmptyPlainTextParts(allParts)
	allParts = mergeAdjacentPlainTextParts(allParts)

	return allParts
}

// parseLine parses a single line into parts
func (tp *TextParser) parseLine(line string) []Part {
	var parts []Part
	remaining := line

	for remaining != "" {
		// Find next link of any type
		nextLink := tp.findNextLink(remaining)

		if nextLink == nil {
			// No more links, rest is plain text
			parts = append(parts, &PlainTextPart{Content: remaining})
			break
		}

		if nextLink.position > 0 {
			// Add plain text before the link
			parts = append(parts, &PlainTextPart{Content: remaining[:nextLink.position]})
		}

		// Add the link part
		parts = append(parts, nextLink.part)

		// Continue with remainder
		remaining = nextLink.remainder
	}

	return parts
}

// linkMatch represents a found link
type linkMatch struct {
	position  int
	part      Part
	remainder string
}

// findNextLink finds the next link in the text
func (tp *TextParser) findNextLink(text string) *linkMatch {
	var best *linkMatch

	// Check each link type
	checkers := []func(string) *linkMatch{
		tp.findKSK,
		tp.findCHK,
		tp.findSSK,
		tp.findUSK,
		tp.findSoneLink,
		tp.findPostLink,
		tp.findHTTP,
		tp.findHTTPS,
		tp.findFreemail,
	}

	for _, checker := range checkers {
		match := checker(text)
		if match != nil && (best == nil || match.position < best.position) {
			best = match
		}
	}

	return best
}

// Freenet link patterns
var (
	kskPattern  = regexp.MustCompile(`(?:freenet:)?KSK@[^\s]+`)
	chkPattern  = regexp.MustCompile(`(?:freenet:)?CHK@[^\s]+`)
	sskPattern  = regexp.MustCompile(`(?:freenet:)?SSK@[^\s]+`)
	uskPattern  = regexp.MustCompile(`(?:freenet:)?USK@[^\s]+`)
	sonePattern = regexp.MustCompile(`sone://([A-Za-z0-9~-]{43})`)
	postPattern = regexp.MustCompile(`post://([A-Za-z0-9-]{36})`)
	httpPattern = regexp.MustCompile(`https?://[^\s]+`)
	freemailPattern = regexp.MustCompile(`[A-Za-z0-9._-]+@[a-z2-7]{52}\.freemail`)
)

func (tp *TextParser) findKSK(text string) *linkMatch {
	return tp.findFreenetLink(text, kskPattern, "KSK@")
}

func (tp *TextParser) findCHK(text string) *linkMatch {
	return tp.findFreenetLink(text, chkPattern, "CHK@")
}

func (tp *TextParser) findSSK(text string) *linkMatch {
	return tp.findFreenetLink(text, sskPattern, "SSK@")
}

func (tp *TextParser) findUSK(text string) *linkMatch {
	return tp.findFreenetLink(text, uskPattern, "USK@")
}

func (tp *TextParser) findFreenetLink(text string, pattern *regexp.Regexp, keyType string) *linkMatch {
	loc := pattern.FindStringIndex(text)
	if loc == nil {
		return nil
	}

	link := text[loc[0]:loc[1]]
	link = cleanLinkEnd(link)

	// Remove freenet: prefix for display
	displayLink := link
	if strings.HasPrefix(strings.ToLower(displayLink), "freenet:") {
		displayLink = displayLink[8:]
	}

	// Extract display name
	displayText := extractFreenetDisplayText(displayLink, keyType)

	return &linkMatch{
		position: loc[0],
		part: &FreenetLinkPart{
			Link:        displayLink,
			DisplayText: displayText,
		},
		remainder: text[loc[0]+len(link):],
	}
}

func (tp *TextParser) findSoneLink(text string) *linkMatch {
	loc := sonePattern.FindStringSubmatchIndex(text)
	if loc == nil {
		return nil
	}

	soneID := text[loc[2]:loc[3]]

	var soneName string
	if tp.soneResolver != nil {
		if sone := tp.soneResolver(soneID); sone != nil {
			soneName = sone.Name
		}
	}

	return &linkMatch{
		position: loc[0],
		part: &SoneLinkPart{
			SoneID:   soneID,
			SoneName: soneName,
		},
		remainder: text[loc[1]:],
	}
}

func (tp *TextParser) findPostLink(text string) *linkMatch {
	loc := postPattern.FindStringSubmatchIndex(text)
	if loc == nil {
		return nil
	}

	postID := text[loc[2]:loc[3]]

	var postAuthor string
	if tp.postResolver != nil {
		if post := tp.postResolver(postID); post != nil {
			if tp.soneResolver != nil {
				if sone := tp.soneResolver(post.SoneID); sone != nil {
					postAuthor = sone.Name
				}
			}
		}
	}

	return &linkMatch{
		position: loc[0],
		part: &PostLinkPart{
			PostID:     postID,
			PostAuthor: postAuthor,
		},
		remainder: text[loc[1]:],
	}
}

func (tp *TextParser) findHTTP(text string) *linkMatch {
	return tp.findHTTPLink(text)
}

func (tp *TextParser) findHTTPS(text string) *linkMatch {
	return tp.findHTTPLink(text)
}

func (tp *TextParser) findHTTPLink(text string) *linkMatch {
	loc := httpPattern.FindStringIndex(text)
	if loc == nil {
		return nil
	}

	link := text[loc[0]:loc[1]]
	link = cleanLinkEnd(link)

	displayText := simplifyURL(link)

	return &linkMatch{
		position: loc[0],
		part: &HTTPLinkPart{
			URL:         link,
			DisplayText: displayText,
		},
		remainder: text[loc[0]+len(link):],
	}
}

func (tp *TextParser) findFreemail(text string) *linkMatch {
	loc := freemailPattern.FindStringIndex(text)
	if loc == nil {
		return nil
	}

	email := text[loc[0]:loc[1]]
	atIndex := strings.Index(email, "@")
	localPart := email[:atIndex]
	freemailID := email[atIndex+1 : len(email)-9] // Remove ".freemail"

	return &linkMatch{
		position: loc[0],
		part: &FreemailPart{
			LocalPart:  localPart,
			FreemailID: freemailID,
			IdentityID: freemailID, // Would need base32 decoding for real ID
		},
		remainder: text[loc[1]:],
	}
}

// Helper functions

func trimEmptyLines(lines []string) []string {
	start := 0
	end := len(lines)

	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}

	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}

	return lines[start:end]
}

func mergeMultipleEmptyLines(lines []string) []string {
	var result []string
	prevEmpty := false

	for _, line := range lines {
		isEmpty := strings.TrimSpace(line) == ""
		if isEmpty && prevEmpty {
			continue
		}
		result = append(result, line)
		prevEmpty = isEmpty
	}

	return result
}

func removeEmptyPlainTextParts(parts []Part) []Part {
	var result []Part
	for _, p := range parts {
		if pt, ok := p.(*PlainTextPart); ok && pt.Content == "" {
			continue
		}
		result = append(result, p)
	}
	return result
}

func mergeAdjacentPlainTextParts(parts []Part) []Part {
	if len(parts) == 0 {
		return parts
	}

	var result []Part
	for _, p := range parts {
		if len(result) > 0 {
			if lastPT, ok := result[len(result)-1].(*PlainTextPart); ok {
				if currentPT, ok := p.(*PlainTextPart); ok {
					lastPT.Content += currentPT.Content
					continue
				}
			}
		}
		result = append(result, p)
	}
	return result
}

// cleanLinkEnd removes trailing punctuation and unmatched parens from a link
func cleanLinkEnd(link string) string {
	// Remove trailing punctuation
	for len(link) > 0 {
		last := rune(link[len(link)-1])
		if last == '.' || last == ',' || last == '?' || last == '!' || last == ')' || last == ']' {
			// Check for matched parens
			if last == ')' && strings.Count(link, "(") > strings.Count(link, ")")-1 {
				break
			}
			if last == ']' && strings.Count(link, "[") > strings.Count(link, "]")-1 {
				break
			}
			link = link[:len(link)-1]
		} else {
			break
		}
	}
	return link
}

// extractFreenetDisplayText extracts a display name from a Freenet URI
func extractFreenetDisplayText(link string, keyType string) string {
	// Try to find a filename or doc name
	parts := strings.Split(link, "/")
	if len(parts) > 1 {
		// Return last non-empty path component
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] != "" && !strings.HasPrefix(parts[i], keyType) {
				// Remove query string
				name := strings.Split(parts[i], "?")[0]
				if name != "" {
					return name
				}
			}
		}
	}

	// Fall back to key type + first chars
	if len(link) > 12 {
		return link[:12] + "..."
	}
	return link
}

// simplifyURL creates a shortened display version of a URL
func simplifyURL(url string) string {
	// Remove protocol
	display := url
	if strings.HasPrefix(display, "https://") {
		display = display[8:]
	} else if strings.HasPrefix(display, "http://") {
		display = display[7:]
	}

	// Remove www prefix
	if strings.HasPrefix(display, "www.") {
		display = display[4:]
	}

	// Remove query parameters
	if qIdx := strings.Index(display, "?"); qIdx != -1 {
		display = display[:qIdx]
	}

	// Shorten middle path components
	parts := strings.Split(display, "/")
	if len(parts) > 3 {
		display = parts[0] + "/.../" + parts[len(parts)-1]
	}

	// Remove trailing slash
	display = strings.TrimSuffix(display, "/")

	return display
}

// RenderPartsToHTML converts parts to HTML
func RenderPartsToHTML(parts []Part) string {
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.ToHTML())
	}
	return sb.String()
}

// RenderPartsToPlainText converts parts to plain text
func RenderPartsToPlainText(parts []Part) string {
	var sb strings.Builder
	for _, p := range parts {
		sb.WriteString(p.ToPlainText())
	}
	return sb.String()
}

// FindMentionedSones finds all Sone IDs mentioned in parts
func FindMentionedSones(parts []Part) []string {
	var soneIDs []string
	seen := make(map[string]bool)

	for _, p := range parts {
		if sp, ok := p.(*SoneLinkPart); ok {
			if !seen[sp.SoneID] {
				seen[sp.SoneID] = true
				soneIDs = append(soneIDs, sp.SoneID)
			}
		}
	}

	return soneIDs
}

// HasMentionOf checks if parts contain a mention of the given Sone ID
func HasMentionOf(parts []Part, soneID string) bool {
	for _, p := range parts {
		if sp, ok := p.(*SoneLinkPart); ok {
			if sp.SoneID == soneID {
				return true
			}
		}
	}
	return false
}

// MentionDetector detects mentions of local Sones in posts/replies
type MentionDetector struct {
	textParser  *TextParser
	localSones  map[string]bool
	eventBus    *EventBus
}

// NewMentionDetector creates a new mention detector
func NewMentionDetector(textParser *TextParser, eventBus *EventBus) *MentionDetector {
	return &MentionDetector{
		textParser: textParser,
		localSones: make(map[string]bool),
		eventBus:   eventBus,
	}
}

// SetLocalSones updates the set of local Sone IDs
func (md *MentionDetector) SetLocalSones(soneIDs []string) {
	md.localSones = make(map[string]bool)
	for _, id := range soneIDs {
		md.localSones[id] = true
	}
}

// CheckPost checks a post for mentions of local Sones
func (md *MentionDetector) CheckPost(post *Post) bool {
	parts := md.textParser.Parse(post.Text)
	for _, soneID := range FindMentionedSones(parts) {
		if md.localSones[soneID] {
			md.eventBus.PublishMentionDetected(post, soneID)
			return true
		}
	}
	return false
}

// CheckReply checks a reply for mentions of local Sones
func (md *MentionDetector) CheckReply(reply *PostReply) bool {
	parts := md.textParser.Parse(reply.Text)
	for _, soneID := range FindMentionedSones(parts) {
		if md.localSones[soneID] {
			// Create a dummy post for the event
			post := &Post{ID: reply.PostID}
			md.eventBus.PublishMentionDetected(post, soneID)
			return true
		}
	}
	return false
}

// isWhitespace checks if a rune is whitespace (including Unicode whitespace)
func isWhitespace(r rune) bool {
	return unicode.IsSpace(r) || r == '\u00a0' || r == '\u200b' || r == '\u2060'
}
