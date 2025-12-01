// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package web

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/blubskye/gohyphanet/sone"
)

//go:embed templates/*.html static/*
var content embed.FS

// Server is the Sone web interface server
type Server struct {
	core       *sone.Core
	textParser *sone.TextParser
	templates  *template.Template
	mux        *http.ServeMux
	server     *http.Server
	addr       string
}

// NewServer creates a new web server
func NewServer(core *sone.Core, addr string) *Server {
	s := &Server{
		core: core,
		addr: addr,
		mux:  http.NewServeMux(),
	}

	// Create text parser with resolvers
	s.textParser = sone.NewTextParser(
		func(id string) *sone.Sone { return core.GetSone(id) },
		func(id string) *sone.Post { return core.Database().GetPost(id) },
	)

	// Load templates
	s.loadTemplates()

	// Register routes
	s.registerRoutes()

	return s
}

// loadTemplates loads HTML templates
func (s *Server) loadTemplates() {
	funcMap := template.FuncMap{
		"parseText":    s.parseTextToHTML,
		"formatTime":   formatTime,
		"shortID":      shortID,
		"pluralize":    pluralize,
		"safeHTML":     safeHTML,
		"add":          add,
		"sub":          sub,
	}

	var err error
	s.templates, err = template.New("").Funcs(funcMap).ParseFS(content, "templates/*.html")
	if err != nil {
		log.Printf("Warning: failed to parse embedded templates: %v", err)
		// Create empty template set
		s.templates = template.New("").Funcs(funcMap)
	}
}

// parseTextToHTML parses Sone text and returns HTML
func (s *Server) parseTextToHTML(text string) template.HTML {
	parts := s.textParser.Parse(text)
	return template.HTML(sone.RenderPartsToHTML(parts))
}

// registerRoutes sets up HTTP routes
func (s *Server) registerRoutes() {
	// Static files
	s.mux.HandleFunc("/static/", s.handleStatic)

	// Pages
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/sone/", s.handleViewSone)
	s.mux.HandleFunc("/post/", s.handleViewPost)
	s.mux.HandleFunc("/known-sones", s.handleKnownSones)
	s.mux.HandleFunc("/bookmarks", s.handleBookmarks)
	s.mux.HandleFunc("/options", s.handleOptions)
	s.mux.HandleFunc("/login", s.handleLogin)
	s.mux.HandleFunc("/create-post", s.handleCreatePostPage)
	s.mux.HandleFunc("/search", s.handleSearch)

	// AJAX endpoints
	s.mux.HandleFunc("/ajax/create-post", s.handleAjaxCreatePost)
	s.mux.HandleFunc("/ajax/create-reply", s.handleAjaxCreateReply)
	s.mux.HandleFunc("/ajax/delete-post", s.handleAjaxDeletePost)
	s.mux.HandleFunc("/ajax/delete-reply", s.handleAjaxDeleteReply)
	s.mux.HandleFunc("/ajax/like", s.handleAjaxLike)
	s.mux.HandleFunc("/ajax/unlike", s.handleAjaxUnlike)
	s.mux.HandleFunc("/ajax/follow", s.handleAjaxFollow)
	s.mux.HandleFunc("/ajax/unfollow", s.handleAjaxUnfollow)
	s.mux.HandleFunc("/ajax/bookmark", s.handleAjaxBookmark)
	s.mux.HandleFunc("/ajax/unbookmark", s.handleAjaxUnbookmark)
	s.mux.HandleFunc("/ajax/status", s.handleAjaxStatus)
	s.mux.HandleFunc("/ajax/notifications", s.handleAjaxNotifications)
	s.mux.HandleFunc("/ajax/dismiss-notification", s.handleAjaxDismissNotification)

	// Register image handlers
	s.RegisterImageHandlers()
}

// Start starts the web server
func (s *Server) Start() error {
	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      s.mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	log.Printf("Starting Sone web interface on %s", s.addr)
	return s.server.ListenAndServe()
}

// Stop stops the web server
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// handleStatic serves static files
func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	// Remove /static/ prefix
	path := strings.TrimPrefix(r.URL.Path, "/static/")

	data, err := content.ReadFile("static/" + path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Set content type based on extension
	ext := filepath.Ext(path)
	switch ext {
	case ".css":
		w.Header().Set("Content-Type", "text/css")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	}

	w.Write(data)
}

// Template helper functions

func formatTime(timestamp int64) string {
	t := time.UnixMilli(timestamp)
	now := time.Now()
	diff := now.Sub(t)

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		return pluralize(mins, "minute") + " ago"
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		return pluralize(hours, "hour") + " ago"
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		return pluralize(days, "day") + " ago"
	default:
		return t.Format("Jan 2, 2006")
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8] + "..."
	}
	return id
}

func pluralize(count int, singular string) string {
	if count == 1 {
		return "1 " + singular
	}
	return string(rune(count+'0')) + " " + singular + "s"
}

func safeHTML(s string) template.HTML {
	return template.HTML(s)
}

func add(a, b int) int {
	return a + b
}

func sub(a, b int) int {
	return a - b
}

// PageData contains common data for all pages
type PageData struct {
	Title         string
	CurrentSone   *sone.Sone
	LocalSones    []*sone.Sone
	Notifications []*sone.Notification
	Error         string
	Success       string
}

// getPageData creates common page data
func (s *Server) getPageData(r *http.Request, title string) *PageData {
	// Get current sone from session/cookie
	currentSone := s.getCurrentSone(r)

	return &PageData{
		Title:         title,
		CurrentSone:   currentSone,
		LocalSones:    s.core.GetLocalSones(),
		Notifications: s.core.GetNotifications(),
	}
}

// getCurrentSone gets the currently selected Sone from session
func (s *Server) getCurrentSone(r *http.Request) *sone.Sone {
	// Check for sone cookie
	cookie, err := r.Cookie("current_sone")
	if err != nil {
		// Return first local sone if available
		localSones := s.core.GetLocalSones()
		if len(localSones) > 0 {
			return localSones[0]
		}
		return nil
	}

	return s.core.GetLocalSone(cookie.Value)
}

// setCurrentSone sets the currently selected Sone
func (s *Server) setCurrentSone(w http.ResponseWriter, soneID string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "current_sone",
		Value:    soneID,
		Path:     "/",
		MaxAge:   86400 * 365, // 1 year
		HttpOnly: true,
	})
}

// renderTemplate renders a template with data
func (s *Server) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	err := s.templates.ExecuteTemplate(w, name, data)
	if err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

// requireLocalSone ensures a local sone is selected
func (s *Server) requireLocalSone(w http.ResponseWriter, r *http.Request) *sone.Sone {
	currentSone := s.getCurrentSone(r)
	if currentSone == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return nil
	}
	return currentSone
}
