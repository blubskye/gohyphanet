// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package web

import (
	"net/http"
	"sort"
	"strings"

	"github.com/blubskye/gohyphanet/sone"
)

// IndexData contains data for the index page
type IndexData struct {
	*PageData
	Posts []*PostView
}

// PostView represents a post with additional view data
type PostView struct {
	*sone.Post
	Author      *sone.Sone
	Recipient   *sone.Sone
	Replies     []*ReplyView
	ReplyCount  int
	LikeCount   int
	IsLiked     bool
	IsBookmarked bool
	TextHTML    string
}

// ReplyView represents a reply with additional view data
type ReplyView struct {
	*sone.PostReply
	Author   *sone.Sone
	TextHTML string
}

// handleIndex handles the main feed page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	data := &IndexData{
		PageData: s.getPageData(r, "Sone - Home"),
		Posts:    make([]*PostView, 0),
	}

	currentSone := data.CurrentSone
	if currentSone == nil {
		// Show all posts if not logged in
		allPosts := s.core.Database().GetAllPosts()
		for _, post := range allPosts {
			data.Posts = append(data.Posts, s.createPostView(post, nil))
		}
	} else {
		// Show feed for current sone
		posts := s.core.GetPostFeed(currentSone.ID)
		for _, post := range posts {
			data.Posts = append(data.Posts, s.createPostView(post, currentSone))
		}
	}

	// Sort by time, newest first
	sort.Slice(data.Posts, func(i, j int) bool {
		return data.Posts[i].Time > data.Posts[j].Time
	})

	// Limit to 50 posts
	if len(data.Posts) > 50 {
		data.Posts = data.Posts[:50]
	}

	s.renderTemplate(w, "index.html", data)
}

// createPostView creates a PostView from a Post
func (s *Server) createPostView(post *sone.Post, currentSone *sone.Sone) *PostView {
	pv := &PostView{
		Post:   post,
		Author: s.core.GetSone(post.SoneID),
	}

	// Parse text to HTML
	parts := s.textParser.Parse(post.Text)
	pv.TextHTML = sone.RenderPartsToHTML(parts)

	// Get recipient
	if post.RecipientID != nil {
		pv.Recipient = s.core.GetSone(*post.RecipientID)
	}

	// Get replies
	replies := s.core.Database().GetRepliesByPost(post.ID)
	pv.ReplyCount = len(replies)
	for _, reply := range replies {
		rv := &ReplyView{
			PostReply: reply,
			Author:    s.core.GetSone(reply.SoneID),
		}
		replyParts := s.textParser.Parse(reply.Text)
		rv.TextHTML = sone.RenderPartsToHTML(replyParts)
		pv.Replies = append(pv.Replies, rv)
	}

	// Count likes (simplified - would need to track this properly)
	pv.LikeCount = 0

	// Check if current sone liked this post
	if currentSone != nil {
		pv.IsLiked = currentSone.IsPostLiked(post.ID)
		pv.IsBookmarked = s.core.Database().IsBookmarked(post.ID)
	}

	return pv
}

// SoneViewData contains data for viewing a Sone profile
type SoneViewData struct {
	*PageData
	Sone       *sone.Sone
	Posts      []*PostView
	IsFollowed bool
	IsSelf     bool
}

// handleViewSone handles viewing a Sone profile
func (s *Server) handleViewSone(w http.ResponseWriter, r *http.Request) {
	// Extract Sone ID from URL
	soneID := strings.TrimPrefix(r.URL.Path, "/sone/")
	if soneID == "" {
		http.NotFound(w, r)
		return
	}

	viewSone := s.core.GetSone(soneID)
	if viewSone == nil {
		http.NotFound(w, r)
		return
	}

	currentSone := s.getCurrentSone(r)

	data := &SoneViewData{
		PageData: s.getPageData(r, "Sone - "+viewSone.Name),
		Sone:     viewSone,
		Posts:    make([]*PostView, 0),
	}

	// Check relationships
	if currentSone != nil {
		data.IsSelf = currentSone.ID == viewSone.ID
		data.IsFollowed = currentSone.HasFriend(viewSone.ID)
	}

	// Get posts by this Sone
	posts := s.core.Database().GetPostsBySone(soneID)
	for _, post := range posts {
		data.Posts = append(data.Posts, s.createPostView(post, currentSone))
	}

	s.renderTemplate(w, "sone.html", data)
}

// PostViewData contains data for viewing a single post
type PostViewData struct {
	*PageData
	Post *PostView
}

// handleViewPost handles viewing a single post
func (s *Server) handleViewPost(w http.ResponseWriter, r *http.Request) {
	postID := strings.TrimPrefix(r.URL.Path, "/post/")
	if postID == "" {
		http.NotFound(w, r)
		return
	}

	post := s.core.Database().GetPost(postID)
	if post == nil {
		http.NotFound(w, r)
		return
	}

	currentSone := s.getCurrentSone(r)

	data := &PostViewData{
		PageData: s.getPageData(r, "Sone - Post"),
		Post:     s.createPostView(post, currentSone),
	}

	// Mark as known
	s.core.Database().SetPostKnown(postID, true)

	s.renderTemplate(w, "post.html", data)
}

// KnownSonesData contains data for the known Sones page
type KnownSonesData struct {
	*PageData
	Sones    []*sone.Sone
	Filter   string
	SortBy   string
}

// handleKnownSones handles the known Sones list
func (s *Server) handleKnownSones(w http.ResponseWriter, r *http.Request) {
	filter := r.URL.Query().Get("filter")
	sortBy := r.URL.Query().Get("sort")
	if sortBy == "" {
		sortBy = "name"
	}

	data := &KnownSonesData{
		PageData: s.getPageData(r, "Sone - Known Sones"),
		Sones:    s.core.GetAllSones(),
		Filter:   filter,
		SortBy:   sortBy,
	}

	// Filter
	if filter != "" {
		filtered := make([]*sone.Sone, 0)
		filterLower := strings.ToLower(filter)
		for _, sn := range data.Sones {
			if strings.Contains(strings.ToLower(sn.Name), filterLower) {
				filtered = append(filtered, sn)
			}
		}
		data.Sones = filtered
	}

	// Sort
	switch sortBy {
	case "name":
		sort.Slice(data.Sones, func(i, j int) bool {
			return strings.ToLower(data.Sones[i].Name) < strings.ToLower(data.Sones[j].Name)
		})
	case "posts":
		sort.Slice(data.Sones, func(i, j int) bool {
			return len(data.Sones[i].Posts) > len(data.Sones[j].Posts)
		})
	case "activity":
		sort.Slice(data.Sones, func(i, j int) bool {
			return data.Sones[i].Time > data.Sones[j].Time
		})
	}

	s.renderTemplate(w, "known-sones.html", data)
}

// BookmarksData contains data for the bookmarks page
type BookmarksData struct {
	*PageData
	Posts []*PostView
}

// handleBookmarks handles the bookmarks page
func (s *Server) handleBookmarks(w http.ResponseWriter, r *http.Request) {
	currentSone := s.getCurrentSone(r)

	data := &BookmarksData{
		PageData: s.getPageData(r, "Sone - Bookmarks"),
		Posts:    make([]*PostView, 0),
	}

	bookmarkedIDs := s.core.Database().GetBookmarkedPosts()
	for _, postID := range bookmarkedIDs {
		post := s.core.Database().GetPost(postID)
		if post != nil {
			data.Posts = append(data.Posts, s.createPostView(post, currentSone))
		}
	}

	s.renderTemplate(w, "bookmarks.html", data)
}

// OptionsData contains data for the options page
type OptionsData struct {
	*PageData
}

// handleOptions handles the options page
func (s *Server) handleOptions(w http.ResponseWriter, r *http.Request) {
	data := &OptionsData{
		PageData: s.getPageData(r, "Sone - Options"),
	}

	s.renderTemplate(w, "options.html", data)
}

// LoginData contains data for the login page
type LoginData struct {
	*PageData
}

// handleLogin handles the login/identity selection page
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		soneID := r.FormValue("sone")
		if soneID != "" {
			s.setCurrentSone(w, soneID)
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}

	data := &LoginData{
		PageData: s.getPageData(r, "Sone - Login"),
	}

	s.renderTemplate(w, "login.html", data)
}

// CreatePostData contains data for the create post page
type CreatePostData struct {
	*PageData
	RecipientID string
}

// handleCreatePostPage handles the create post page
func (s *Server) handleCreatePostPage(w http.ResponseWriter, r *http.Request) {
	currentSone := s.requireLocalSone(w, r)
	if currentSone == nil {
		return
	}

	data := &CreatePostData{
		PageData:    s.getPageData(r, "Sone - Create Post"),
		RecipientID: r.URL.Query().Get("recipient"),
	}

	s.renderTemplate(w, "create-post.html", data)
}

// SearchData contains data for the search page
type SearchData struct {
	*PageData
	Query   string
	Posts   []*PostView
	Sones   []*sone.Sone
}

// handleSearch handles the search page
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	currentSone := s.getCurrentSone(r)

	data := &SearchData{
		PageData: s.getPageData(r, "Sone - Search"),
		Query:    query,
		Posts:    make([]*PostView, 0),
		Sones:    make([]*sone.Sone, 0),
	}

	if query != "" {
		queryLower := strings.ToLower(query)

		// Search posts
		allPosts := s.core.Database().GetAllPosts()
		for _, post := range allPosts {
			if strings.Contains(strings.ToLower(post.Text), queryLower) {
				data.Posts = append(data.Posts, s.createPostView(post, currentSone))
			}
		}

		// Search Sones
		allSones := s.core.GetAllSones()
		for _, sn := range allSones {
			if strings.Contains(strings.ToLower(sn.Name), queryLower) {
				data.Sones = append(data.Sones, sn)
			}
			// Also search profile
			if sn.Profile != nil {
				if strings.Contains(strings.ToLower(sn.Profile.FirstName), queryLower) ||
					strings.Contains(strings.ToLower(sn.Profile.LastName), queryLower) {
					data.Sones = append(data.Sones, sn)
				}
			}
		}

		// Limit results
		if len(data.Posts) > 20 {
			data.Posts = data.Posts[:20]
		}
		if len(data.Sones) > 20 {
			data.Sones = data.Sones[:20]
		}
	}

	s.renderTemplate(w, "search.html", data)
}
