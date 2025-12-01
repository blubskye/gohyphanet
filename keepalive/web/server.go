// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/keepalive"
)

//go:embed templates/*.html static/*
var embeddedFiles embed.FS

// Server represents the web server
type Server struct {
	mu sync.RWMutex

	port        int
	siteManager *keepalive.SiteManager
	reinserter  *keepalive.Reinserter
	config      *keepalive.Config
	stats       *keepalive.StatsCollector

	templates *template.Template
	server    *http.Server
}

// NewServer creates a new web server
func NewServer(port int) *Server {
	return &Server{
		port:  port,
		stats: keepalive.NewStatsCollector(),
	}
}

// SetSiteManager sets the site manager
func (s *Server) SetSiteManager(sm *keepalive.SiteManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.siteManager = sm
}

// SetReinserter sets the reinserter
func (s *Server) SetReinserter(r *keepalive.Reinserter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reinserter = r
}

// SetConfig sets the config
func (s *Server) SetConfig(cfg *keepalive.Config) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.config = cfg
}

// Start starts the web server
func (s *Server) Start() error {
	// Parse templates
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"formatTime": func(t time.Time) string {
			if t.IsZero() {
				return "-"
			}
			return t.Format("2006-01-02 15:04:05")
		},
		"formatDuration": func(d time.Duration) string {
			return d.Round(time.Second).String()
		},
		"formatPercent": func(f float64) string {
			return fmt.Sprintf("%.1f%%", f)
		},
		"stateClass": func(state string) string {
			switch state {
			case "complete", "skipped":
				return "success"
			case "failed", "error":
				return "danger"
			case "running", "healing", "testing":
				return "info"
			default:
				return "secondary"
			}
		},
	}).ParseFS(embeddedFiles, "templates/*.html")
	if err != nil {
		return fmt.Errorf("failed to parse templates: %w", err)
	}
	s.templates = tmpl

	// Create router
	mux := http.NewServeMux()

	// Static files
	staticFS, err := fs.Sub(embeddedFiles, "static")
	if err != nil {
		return fmt.Errorf("failed to get static fs: %w", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticFS))))

	// Pages
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/sites", s.handleSites)
	mux.HandleFunc("/site/", s.handleSiteDetail)
	mux.HandleFunc("/add", s.handleAddSite)
	mux.HandleFunc("/settings", s.handleSettings)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/about", s.handleAbout)

	// Actions
	mux.HandleFunc("/action/start", s.handleStart)
	mux.HandleFunc("/action/stop", s.handleStop)
	mux.HandleFunc("/action/pause", s.handlePause)
	mux.HandleFunc("/action/resume", s.handleResume)
	mux.HandleFunc("/action/delete", s.handleDelete)

	// AJAX endpoints
	mux.HandleFunc("/ajax/status", s.handleAjaxStatus)
	mux.HandleFunc("/ajax/sites", s.handleAjaxSites)
	mux.HandleFunc("/ajax/progress", s.handleAjaxProgress)

	// Create server
	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
	}

	// Start in background
	go func() {
		log.Printf("Keepalive Web UI starting on http://localhost:%d", s.port)
		if err := s.server.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("Web server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the web server
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// renderTemplate renders a template
func (s *Server) renderTemplate(w http.ResponseWriter, name string, data interface{}) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.templates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleIndex handles the main page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	s.mu.RLock()
	sm := s.siteManager
	reinserter := s.reinserter
	s.mu.RUnlock()

	data := struct {
		Title       string
		Sites       []*keepalive.Site
		ActiveSite  *keepalive.Site
		State       string
		Stats       keepalive.SessionStats
	}{
		Title: "GoKeepalive",
	}

	if sm != nil {
		data.Sites = sm.GetAllSites()
	}

	if reinserter != nil {
		data.State = reinserter.GetState().String()
		data.ActiveSite = reinserter.GetActiveSite()
	}

	data.Stats = s.stats.GetSessionStats()

	s.renderTemplate(w, "index.html", data)
}

// handleSites handles the sites list page
func (s *Server) handleSites(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	sm := s.siteManager
	s.mu.RUnlock()

	data := struct {
		Title string
		Sites []*keepalive.Site
	}{
		Title: "Sites - GoKeepalive",
	}

	if sm != nil {
		data.Sites = sm.GetAllSites()
	}

	s.renderTemplate(w, "sites.html", data)
}

// handleSiteDetail handles the site detail page
func (s *Server) handleSiteDetail(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/site/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	s.mu.RLock()
	sm := s.siteManager
	s.mu.RUnlock()

	if sm == nil {
		http.Error(w, "Site manager not initialized", http.StatusInternalServerError)
		return
	}

	site := sm.GetSite(id)
	if site == nil {
		http.NotFound(w, r)
		return
	}

	data := struct {
		Title string
		Site  *keepalive.Site
		Logs  []keepalive.LogEntry
	}{
		Title: fmt.Sprintf("%s - GoKeepalive", site.Name),
		Site:  site,
		Logs:  site.GetRecentLogs(50),
	}

	s.renderTemplate(w, "site.html", data)
}

// handleAddSite handles adding a new site
func (s *Server) handleAddSite(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		data := struct {
			Title string
		}{
			Title: "Add Site - GoKeepalive",
		}
		s.renderTemplate(w, "add.html", data)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uri := r.FormValue("uri")
	name := r.FormValue("name")

	if uri == "" {
		http.Error(w, "URI is required", http.StatusBadRequest)
		return
	}

	if name == "" {
		name = "Site"
	}

	s.mu.RLock()
	sm := s.siteManager
	s.mu.RUnlock()

	if sm == nil {
		http.Error(w, "Site manager not initialized", http.StatusInternalServerError)
		return
	}

	site, err := sm.AddSite(uri, name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/site/%d", site.ID), http.StatusSeeOther)
}

// handleSettings handles the settings page
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	cfg := s.config
	s.mu.RUnlock()

	if r.Method == "POST" {
		if cfg != nil {
			if power := r.FormValue("power"); power != "" {
				if p, err := strconv.Atoi(power); err == nil && p > 0 && p <= 20 {
					cfg.SetPower(p)
				}
			}
			// Save config
			// TODO: Save to storage
		}
		http.Redirect(w, r, "/settings", http.StatusSeeOther)
		return
	}

	data := struct {
		Title  string
		Config *keepalive.Config
	}{
		Title:  "Settings - GoKeepalive",
		Config: cfg,
	}

	s.renderTemplate(w, "settings.html", data)
}

// handleStats handles the stats page
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title   string
		Stats   keepalive.SessionStats
		History []keepalive.SessionRecord
	}{
		Title:   "Statistics - GoKeepalive",
		Stats:   s.stats.GetSessionStats(),
		History: s.stats.GetHistory(),
	}

	s.renderTemplate(w, "stats.html", data)
}

// handleAbout handles the about page
func (s *Server) handleAbout(w http.ResponseWriter, r *http.Request) {
	data := struct {
		Title string
	}{
		Title: "About - GoKeepalive",
	}

	s.renderTemplate(w, "about.html", data)
}

// handleStart handles starting reinsertion
func (s *Server) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.FormValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid site ID", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	sm := s.siteManager
	reinserter := s.reinserter
	s.mu.RUnlock()

	if sm == nil || reinserter == nil {
		http.Error(w, "Not initialized", http.StatusInternalServerError)
		return
	}

	site := sm.GetSite(id)
	if site == nil {
		http.Error(w, "Site not found", http.StatusNotFound)
		return
	}

	if err := reinserter.Start(site); err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleStop handles stopping reinsertion
func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	reinserter := s.reinserter
	s.mu.RUnlock()

	if reinserter != nil {
		reinserter.Stop()
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handlePause handles pausing reinsertion
func (s *Server) handlePause(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	reinserter := s.reinserter
	s.mu.RUnlock()

	if reinserter != nil {
		reinserter.Pause()
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleResume handles resuming reinsertion
func (s *Server) handleResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.mu.RLock()
	reinserter := s.reinserter
	s.mu.RUnlock()

	if reinserter != nil {
		reinserter.Resume()
	}

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// handleDelete handles deleting a site
func (s *Server) handleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := r.FormValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid site ID", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	sm := s.siteManager
	s.mu.RUnlock()

	if sm != nil {
		sm.RemoveSite(id)
	}

	http.Redirect(w, r, "/sites", http.StatusSeeOther)
}

// AJAX handlers

type statusResponse struct {
	State      string  `json:"state"`
	SiteID     int     `json:"site_id"`
	SiteName   string  `json:"site_name"`
	Segment    int     `json:"segment"`
	TotalSegs  int     `json:"total_segments"`
	Percent    float64 `json:"percent"`
	Message    string  `json:"message"`
}

func (s *Server) handleAjaxStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	reinserter := s.reinserter
	s.mu.RUnlock()

	resp := statusResponse{
		State: "idle",
	}

	if reinserter != nil {
		resp.State = reinserter.GetState().String()
		if site := reinserter.GetActiveSite(); site != nil {
			resp.SiteID = site.ID
			resp.SiteName = site.Name
			resp.Segment = site.GetCurrentSegment()
			resp.TotalSegs = site.SegmentCount()
			if resp.TotalSegs > 0 {
				resp.Percent = float64(resp.Segment+1) / float64(resp.TotalSegs) * 100
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleAjaxSites(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	sm := s.siteManager
	s.mu.RUnlock()

	var sites []map[string]interface{}
	if sm != nil {
		for _, site := range sm.GetAllSites() {
			sites = append(sites, map[string]interface{}{
				"id":           site.ID,
				"name":         site.Name,
				"uri":          site.URI,
				"state":        site.State.String(),
				"availability": site.Availability,
				"segments":     site.SegmentCount(),
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sites)
}

func (s *Server) handleAjaxProgress(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	reinserter := s.reinserter
	s.mu.RUnlock()

	resp := map[string]interface{}{
		"running": false,
	}

	if reinserter != nil && reinserter.GetState() == keepalive.ReinserterRunning {
		resp["running"] = true
		if site := reinserter.GetActiveSite(); site != nil {
			ft, fs, ff, it, is, if_ := reinserter.GetStats()
			resp["fetch_total"] = ft
			resp["fetch_success"] = fs
			resp["fetch_failed"] = ff
			resp["insert_total"] = it
			resp["insert_success"] = is
			resp["insert_failed"] = if_
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
