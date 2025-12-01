// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package sone

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// ProtocolVersion is the Sone XML protocol version
const ProtocolVersion = 0

// SoneXML represents the XML structure for Sone data
type SoneXML struct {
	XMLName         xml.Name         `xml:"sone"`
	Time            int64            `xml:"time"`
	ProtocolVersion int              `xml:"protocol-version"`
	Client          *ClientXML       `xml:"client"`
	Profile         *ProfileXML      `xml:"profile"`
	Posts           *PostsXML        `xml:"posts"`
	Replies         *RepliesXML      `xml:"replies"`
	PostLikes       *PostLikesXML    `xml:"post-likes"`
	ReplyLikes      *ReplyLikesXML   `xml:"reply-likes"`
	Albums          *AlbumsXML       `xml:"albums,omitempty"`
}

// ClientXML represents client info in XML
type ClientXML struct {
	Name    string `xml:"name"`
	Version string `xml:"version"`
}

// ProfileXML represents the profile section in XML
type ProfileXML struct {
	FirstName  string      `xml:"first-name"`
	MiddleName string      `xml:"middle-name"`
	LastName   string      `xml:"last-name"`
	BirthDay   string      `xml:"birth-day"`
	BirthMonth string      `xml:"birth-month"`
	BirthYear  string      `xml:"birth-year"`
	Avatar     string      `xml:"avatar"`
	Fields     *FieldsXML  `xml:"fields"`
}

// FieldsXML represents profile fields
type FieldsXML struct {
	Fields []FieldXML `xml:"field"`
}

// FieldXML represents a single profile field
type FieldXML struct {
	Name  string `xml:"field-name"`
	Value string `xml:"field-value"`
}

// PostsXML represents the posts section
type PostsXML struct {
	Posts []PostXML `xml:"post"`
}

// PostXML represents a single post
type PostXML struct {
	ID        string `xml:"id"`
	Recipient string `xml:"recipient"`
	Time      int64  `xml:"time"`
	Text      string `xml:"text"`
}

// RepliesXML represents the replies section
type RepliesXML struct {
	Replies []ReplyXML `xml:"reply"`
}

// ReplyXML represents a single reply
type ReplyXML struct {
	ID     string `xml:"id"`
	PostID string `xml:"post-id"`
	Time   int64  `xml:"time"`
	Text   string `xml:"text"`
}

// PostLikesXML represents liked posts
type PostLikesXML struct {
	PostLikes []string `xml:"post-like"`
}

// ReplyLikesXML represents liked replies
type ReplyLikesXML struct {
	ReplyLikes []string `xml:"reply-like"`
}

// AlbumsXML represents the albums section
type AlbumsXML struct {
	Albums []AlbumXML `xml:"album"`
}

// AlbumXML represents a single album
type AlbumXML struct {
	ID          string     `xml:"id"`
	Parent      string     `xml:"parent,omitempty"`
	Title       string     `xml:"title"`
	Description string     `xml:"description"`
	AlbumImage  string     `xml:"album-image"`
	Images      *ImagesXML `xml:"images,omitempty"`
}

// ImagesXML represents images in an album
type ImagesXML struct {
	Images []ImageXML `xml:"image"`
}

// ImageXML represents a single image
type ImageXML struct {
	ID           string `xml:"id"`
	CreationTime int64  `xml:"creation-time"`
	Key          string `xml:"key"`
	Title        string `xml:"title"`
	Description  string `xml:"description"`
	Width        int    `xml:"width"`
	Height       int    `xml:"height"`
}

// ParseSoneXML parses XML data into a Sone object
func ParseSoneXML(data []byte, soneID string) (*Sone, error) {
	var soneXML SoneXML
	if err := xml.Unmarshal(data, &soneXML); err != nil {
		return nil, fmt.Errorf("failed to parse Sone XML: %w", err)
	}

	// Check protocol version
	if soneXML.ProtocolVersion > ProtocolVersion {
		return nil, fmt.Errorf("unsupported protocol version: %d (max: %d)",
			soneXML.ProtocolVersion, ProtocolVersion)
	}

	sone := NewSone(soneID)
	sone.Time = soneXML.Time
	sone.Status = StatusIdle

	// Parse client
	if soneXML.Client != nil {
		sone.Client = &Client{
			Name:    soneXML.Client.Name,
			Version: soneXML.Client.Version,
		}
	}

	// Parse profile
	if soneXML.Profile != nil {
		parseProfile(sone.Profile, soneXML.Profile)
	}

	// Parse posts
	if soneXML.Posts != nil {
		for _, postXML := range soneXML.Posts.Posts {
			post := &Post{
				ID:       postXML.ID,
				SoneID:   soneID,
				Time:     postXML.Time,
				Text:     postXML.Text,
				IsLoaded: true,
			}
			if postXML.Recipient != "" {
				post.RecipientID = &postXML.Recipient
			}
			sone.Posts = append(sone.Posts, post)
		}
	}

	// Parse replies
	if soneXML.Replies != nil {
		for _, replyXML := range soneXML.Replies.Replies {
			reply := &PostReply{
				ID:       replyXML.ID,
				SoneID:   soneID,
				PostID:   replyXML.PostID,
				Time:     replyXML.Time,
				Text:     replyXML.Text,
				IsLoaded: true,
			}
			sone.Replies = append(sone.Replies, reply)
		}
	}

	// Parse liked posts
	if soneXML.PostLikes != nil {
		for _, postID := range soneXML.PostLikes.PostLikes {
			sone.LikedPostIDs[postID] = true
		}
	}

	// Parse liked replies
	if soneXML.ReplyLikes != nil {
		for _, replyID := range soneXML.ReplyLikes.ReplyLikes {
			sone.LikedReplyIDs[replyID] = true
		}
	}

	// Parse albums
	if soneXML.Albums != nil {
		parseAlbums(sone, soneXML.Albums)
	}

	return sone, nil
}

// parseProfile parses profile XML into a Profile object
func parseProfile(profile *Profile, profileXML *ProfileXML) {
	profile.FirstName = profileXML.FirstName
	profile.MiddleName = profileXML.MiddleName
	profile.LastName = profileXML.LastName
	profile.Avatar = profileXML.Avatar

	if profileXML.BirthDay != "" {
		if day, err := strconv.Atoi(profileXML.BirthDay); err == nil {
			profile.BirthDay = &day
		}
	}
	if profileXML.BirthMonth != "" {
		if month, err := strconv.Atoi(profileXML.BirthMonth); err == nil {
			profile.BirthMonth = &month
		}
	}
	if profileXML.BirthYear != "" {
		if year, err := strconv.Atoi(profileXML.BirthYear); err == nil {
			profile.BirthYear = &year
		}
	}

	if profileXML.Fields != nil {
		for _, fieldXML := range profileXML.Fields.Fields {
			field := profile.AddField(fieldXML.Name)
			field.Value = fieldXML.Value
		}
	}
}

// parseAlbums parses albums XML into the Sone
func parseAlbums(sone *Sone, albumsXML *AlbumsXML) {
	// First pass: create all albums
	albumMap := make(map[string]*Album)
	for _, albumXML := range albumsXML.Albums {
		album := &Album{
			ID:          albumXML.ID,
			SoneID:      sone.ID,
			Title:       albumXML.Title,
			Description: albumXML.Description,
			AlbumImage:  albumXML.AlbumImage,
			Albums:      make([]*Album, 0),
			Images:      make([]*Image, 0),
		}

		if albumXML.Parent != "" {
			album.ParentID = &albumXML.Parent
		}

		// Parse images
		if albumXML.Images != nil {
			for _, imgXML := range albumXML.Images.Images {
				img := &Image{
					ID:           imgXML.ID,
					SoneID:       sone.ID,
					AlbumID:      album.ID,
					Key:          imgXML.Key,
					CreationTime: imgXML.CreationTime,
					Title:        imgXML.Title,
					Description:  imgXML.Description,
					Width:        imgXML.Width,
					Height:       imgXML.Height,
				}
				album.Images = append(album.Images, img)
			}
		}

		albumMap[album.ID] = album
	}

	// Second pass: build hierarchy
	for _, album := range albumMap {
		if album.ParentID == nil {
			// Top-level album, add to root
			sone.RootAlbum.Albums = append(sone.RootAlbum.Albums, album)
		} else {
			// Find parent and add
			if parent, ok := albumMap[*album.ParentID]; ok {
				parent.Albums = append(parent.Albums, album)
			} else {
				// Parent not found, add to root
				sone.RootAlbum.Albums = append(sone.RootAlbum.Albums, album)
			}
		}
	}
}

// ToXML serializes a Sone to XML for insertion
func (s *Sone) ToXML(clientName, clientVersion string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	soneXML := &SoneXML{
		Time:            s.Time,
		ProtocolVersion: ProtocolVersion,
		Client: &ClientXML{
			Name:    clientName,
			Version: clientVersion,
		},
		Profile:    profileToXML(s.Profile),
		Posts:      postsToXML(s.Posts),
		Replies:    repliesToXML(s.Replies),
		PostLikes:  postLikesToXML(s.LikedPostIDs),
		ReplyLikes: replyLikesToXML(s.LikedReplyIDs),
	}

	// Only include albums if there are any
	if len(s.RootAlbum.Albums) > 0 {
		soneXML.Albums = albumsToXML(s.RootAlbum)
	}

	// Marshal with XML header
	output, err := xml.MarshalIndent(soneXML, "", "\t")
	if err != nil {
		return nil, err
	}

	// Add XML header
	header := []byte(`<?xml version="1.0" encoding="utf-8" ?>` + "\n")
	return append(header, output...), nil
}

func profileToXML(p *Profile) *ProfileXML {
	profile := &ProfileXML{
		FirstName:  p.FirstName,
		MiddleName: p.MiddleName,
		LastName:   p.LastName,
		Avatar:     p.Avatar,
	}

	if p.BirthDay != nil {
		profile.BirthDay = strconv.Itoa(*p.BirthDay)
	}
	if p.BirthMonth != nil {
		profile.BirthMonth = strconv.Itoa(*p.BirthMonth)
	}
	if p.BirthYear != nil {
		profile.BirthYear = strconv.Itoa(*p.BirthYear)
	}

	if len(p.Fields) > 0 {
		profile.Fields = &FieldsXML{
			Fields: make([]FieldXML, 0, len(p.Fields)),
		}
		for _, f := range p.Fields {
			profile.Fields.Fields = append(profile.Fields.Fields, FieldXML{
				Name:  f.Name,
				Value: f.Value,
			})
		}
	}

	return profile
}

func postsToXML(posts []*Post) *PostsXML {
	postsXML := &PostsXML{
		Posts: make([]PostXML, 0, len(posts)),
	}

	for _, p := range posts {
		postXML := PostXML{
			ID:   p.ID,
			Time: p.Time,
			Text: p.Text,
		}
		if p.RecipientID != nil {
			postXML.Recipient = *p.RecipientID
		}
		postsXML.Posts = append(postsXML.Posts, postXML)
	}

	return postsXML
}

func repliesToXML(replies []*PostReply) *RepliesXML {
	repliesXML := &RepliesXML{
		Replies: make([]ReplyXML, 0, len(replies)),
	}

	for _, r := range replies {
		repliesXML.Replies = append(repliesXML.Replies, ReplyXML{
			ID:     r.ID,
			PostID: r.PostID,
			Time:   r.Time,
			Text:   r.Text,
		})
	}

	return repliesXML
}

func postLikesToXML(likes map[string]bool) *PostLikesXML {
	likesXML := &PostLikesXML{
		PostLikes: make([]string, 0, len(likes)),
	}
	for id := range likes {
		likesXML.PostLikes = append(likesXML.PostLikes, id)
	}
	return likesXML
}

func replyLikesToXML(likes map[string]bool) *ReplyLikesXML {
	likesXML := &ReplyLikesXML{
		ReplyLikes: make([]string, 0, len(likes)),
	}
	for id := range likes {
		likesXML.ReplyLikes = append(likesXML.ReplyLikes, id)
	}
	return likesXML
}

func albumsToXML(rootAlbum *Album) *AlbumsXML {
	albumsXML := &AlbumsXML{
		Albums: make([]AlbumXML, 0),
	}

	// Flatten album hierarchy
	var flattenAlbums func(albums []*Album, parentID *string)
	flattenAlbums = func(albums []*Album, parentID *string) {
		for _, album := range albums {
			albumXML := AlbumXML{
				ID:          album.ID,
				Title:       album.Title,
				Description: album.Description,
				AlbumImage:  album.AlbumImage,
			}

			if parentID != nil {
				albumXML.Parent = *parentID
			}

			// Add images
			if len(album.Images) > 0 {
				albumXML.Images = &ImagesXML{
					Images: make([]ImageXML, 0, len(album.Images)),
				}
				for _, img := range album.Images {
					albumXML.Images.Images = append(albumXML.Images.Images, ImageXML{
						ID:           img.ID,
						CreationTime: img.CreationTime,
						Key:          img.Key,
						Title:        img.Title,
						Description:  img.Description,
						Width:        img.Width,
						Height:       img.Height,
					})
				}
			}

			albumsXML.Albums = append(albumsXML.Albums, albumXML)

			// Recurse into nested albums
			if len(album.Albums) > 0 {
				flattenAlbums(album.Albums, &album.ID)
			}
		}
	}

	flattenAlbums(rootAlbum.Albums, nil)
	return albumsXML
}

// EscapeXML escapes special characters for XML
func EscapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}
