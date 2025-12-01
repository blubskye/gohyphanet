// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

// Package web provides a web interface for Freemail.
package web

import (
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/freemail"
)

//go:embed templates/*.html
var templatesFS embed.FS

//go:embed static/*
var staticFS embed.FS

// Server configuration
const (
	DefaultPort        = 3080
	ReadTimeout        = 30 * time.Second
	WriteTimeout       = 30 * time.Second
	SessionCookieName  = "freemail_session"
	SessionExpiration  = 24 * time.Hour
)

// Server represents the web server
type Server struct {
	mu sync.RWMutex

	// Configuration
	port    int
	address string

	// HTTP server
	httpServer *http.Server

	// Templates
	templates *template.Template

	// Dependencies
	accountManager   *freemail.AccountManager
	storage          *freemail.Storage
	transportManager *freemail.TransportManager

	// Sessions
	sessions map[string]*Session

	// State
	running bool
}

// Session represents a user session
type Session struct {
	ID        string
	AccountID string
	Account   *freemail.Account
	CreatedAt time.Time
	ExpiresAt time.Time
}

// NewServer creates a new web server
func NewServer(port int) *Server {
	if port <= 0 {
		port = DefaultPort
	}

	return &Server{
		port:     port,
		address:  fmt.Sprintf(":%d", port),
		sessions: make(map[string]*Session),
	}
}

// SetAccountManager sets the account manager
func (s *Server) SetAccountManager(am *freemail.AccountManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.accountManager = am
}

// SetStorage sets the storage
func (s *Server) SetStorage(storage *freemail.Storage) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storage = storage
}

// SetTransportManager sets the transport manager
func (s *Server) SetTransportManager(tm *freemail.TransportManager) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.transportManager = tm
}

// Start starts the web server
func (s *Server) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("server already running")
	}

	// Load templates
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to parse templates: %w", err)
	}
	s.templates = tmpl

	// Set up routes
	mux := http.NewServeMux()

	// Static files
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		s.mu.Unlock()
		return fmt.Errorf("failed to get static fs: %w", err)
	}
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// Pages
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/login", s.handleLogin)
	mux.HandleFunc("/logout", s.handleLogout)
	mux.HandleFunc("/inbox", s.handleInbox)
	mux.HandleFunc("/sent", s.handleSent)
	mux.HandleFunc("/trash", s.handleTrash)
	mux.HandleFunc("/drafts", s.handleDrafts)
	mux.HandleFunc("/folder/", s.handleFolder)
	mux.HandleFunc("/message/", s.handleMessage)
	mux.HandleFunc("/compose", s.handleCompose)
	mux.HandleFunc("/settings", s.handleSettings)

	// AJAX endpoints
	mux.HandleFunc("/ajax/send", s.handleAjaxSend)
	mux.HandleFunc("/ajax/delete", s.handleAjaxDelete)
	mux.HandleFunc("/ajax/mark-read", s.handleAjaxMarkRead)
	mux.HandleFunc("/ajax/move", s.handleAjaxMove)
	mux.HandleFunc("/ajax/status", s.handleAjaxStatus)

	// Create HTTP server
	s.httpServer = &http.Server{
		Addr:         s.address,
		Handler:      mux,
		ReadTimeout:  ReadTimeout,
		WriteTimeout: WriteTimeout,
	}

	s.running = true
	s.mu.Unlock()

	log.Printf("Web server listening on %s", s.address)

	go func() {
		if err := s.httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("Web server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the web server
func (s *Server) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	s.running = false
	s.mu.Unlock()

	if s.httpServer != nil {
		return s.httpServer.Close()
	}
	return nil
}

// getSession returns the session for a request
func (s *Server) getSession(r *http.Request) *Session {
	cookie, err := r.Cookie(SessionCookieName)
	if err != nil {
		return nil
	}

	s.mu.RLock()
	session := s.sessions[cookie.Value]
	s.mu.RUnlock()

	if session == nil || time.Now().After(session.ExpiresAt) {
		return nil
	}

	return session
}

// createSession creates a new session
func (s *Server) createSession(account *freemail.Account) *Session {
	sessionID := fmt.Sprintf("sess-%d", time.Now().UnixNano())

	session := &Session{
		ID:        sessionID,
		AccountID: account.ID,
		Account:   account,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(SessionExpiration),
	}

	s.mu.Lock()
	s.sessions[sessionID] = session
	s.mu.Unlock()

	return session
}

// deleteSession removes a session
func (s *Server) deleteSession(sessionID string) {
	s.mu.Lock()
	delete(s.sessions, sessionID)
	s.mu.Unlock()
}

// requireAuth checks for authentication and redirects if not authenticated
func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) *Session {
	session := s.getSession(r)
	if session == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return nil
	}
	return session
}

// PageData holds data for page templates
type PageData struct {
	Title       string
	Session     *Session
	Account     *freemail.Account
	Folder      *freemail.Folder
	FolderName  string
	Folders     []string
	Messages    []*freemail.Message
	Message     *freemail.Message
	Error       string
	Success     string
	UnreadCount int
}

// handleIndex handles the index page
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	session := s.getSession(r)
	if session == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/inbox", http.StatusSeeOther)
}

// handleLogin handles the login page
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		username := r.FormValue("username")
		password := r.FormValue("password")

		s.mu.RLock()
		am := s.accountManager
		s.mu.RUnlock()

		if am == nil {
			s.renderError(w, "Service unavailable")
			return
		}

		account, valid := am.Authenticate(username, password)
		if !valid {
			s.renderTemplate(w, "login.html", &PageData{
				Title: "Login",
				Error: "Invalid username or password",
			})
			return
		}

		session := s.createSession(account)
		http.SetCookie(w, &http.Cookie{
			Name:     SessionCookieName,
			Value:    session.ID,
			Path:     "/",
			HttpOnly: true,
			Expires:  session.ExpiresAt,
		})

		http.Redirect(w, r, "/inbox", http.StatusSeeOther)
		return
	}

	s.renderTemplate(w, "login.html", &PageData{
		Title: "Login",
	})
}

// handleLogout handles logout
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	session := s.getSession(r)
	if session != nil {
		s.deleteSession(session.ID)
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// handleInbox handles the inbox page
func (s *Server) handleInbox(w http.ResponseWriter, r *http.Request) {
	session := s.requireAuth(w, r)
	if session == nil {
		return
	}

	s.renderFolder(w, session, "INBOX")
}

// handleSent handles the sent folder
func (s *Server) handleSent(w http.ResponseWriter, r *http.Request) {
	session := s.requireAuth(w, r)
	if session == nil {
		return
	}

	s.renderFolder(w, session, "Sent")
}

// handleTrash handles the trash folder
func (s *Server) handleTrash(w http.ResponseWriter, r *http.Request) {
	session := s.requireAuth(w, r)
	if session == nil {
		return
	}

	s.renderFolder(w, session, "Trash")
}

// handleDrafts handles the drafts folder
func (s *Server) handleDrafts(w http.ResponseWriter, r *http.Request) {
	session := s.requireAuth(w, r)
	if session == nil {
		return
	}

	s.renderFolder(w, session, "Drafts")
}

// handleFolder handles custom folder pages
func (s *Server) handleFolder(w http.ResponseWriter, r *http.Request) {
	session := s.requireAuth(w, r)
	if session == nil {
		return
	}

	folderName := r.URL.Path[len("/folder/"):]
	if folderName == "" {
		http.Redirect(w, r, "/inbox", http.StatusSeeOther)
		return
	}

	s.renderFolder(w, session, folderName)
}

// renderFolder renders a folder view
func (s *Server) renderFolder(w http.ResponseWriter, session *Session, folderName string) {
	folder := session.Account.GetFolder(folderName)
	if folder == nil {
		s.renderError(w, "Folder not found")
		return
	}

	s.renderTemplate(w, "folder.html", &PageData{
		Title:       folderName,
		Session:     session,
		Account:     session.Account,
		Folder:      folder,
		FolderName:  folderName,
		Folders:     session.Account.ListFolders(),
		Messages:    folder.Messages,
		UnreadCount: countUnread(folder),
	})
}

// handleMessage handles message view
func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	session := s.requireAuth(w, r)
	if session == nil {
		return
	}

	// Parse: /message/{folder}/{uid}
	path := r.URL.Path[len("/message/"):]
	var folderName string
	var uid uint32
	fmt.Sscanf(path, "%s/%d", &folderName, &uid)

	folder := session.Account.GetFolder(folderName)
	if folder == nil {
		s.renderError(w, "Folder not found")
		return
	}

	msg := folder.GetMessage(uid)
	if msg == nil {
		s.renderError(w, "Message not found")
		return
	}

	// Mark as read
	msg.SetFlag(freemail.FlagSeen)

	s.renderTemplate(w, "message.html", &PageData{
		Title:      msg.Subject,
		Session:    session,
		Account:    session.Account,
		Folder:     folder,
		FolderName: folderName,
		Folders:    session.Account.ListFolders(),
		Message:    msg,
	})
}

// handleCompose handles the compose page
func (s *Server) handleCompose(w http.ResponseWriter, r *http.Request) {
	session := s.requireAuth(w, r)
	if session == nil {
		return
	}

	if r.Method == "POST" {
		to := r.FormValue("to")
		subject := r.FormValue("subject")
		body := r.FormValue("body")

		// Create message
		msg := freemail.NewMessage()
		msg.From = session.Account.GetEmailAddress()
		toAddr, err := freemail.ParseEmailAddress(to)
		if err != nil {
			s.renderTemplate(w, "compose.html", &PageData{
				Title:   "Compose",
				Session: session,
				Account: session.Account,
				Folders: session.Account.ListFolders(),
				Error:   "Invalid recipient address",
			})
			return
		}
		msg.To = []*freemail.EmailAddress{toAddr}
		msg.Subject = subject
		msg.Body = []byte(body)
		msg.MessageID = fmt.Sprintf("<%d@gofreemail>", time.Now().UnixNano())

		// Add to sent folder
		session.Account.Sent.AddMessage(msg)

		// TODO: Queue for delivery via transport

		s.renderTemplate(w, "compose.html", &PageData{
			Title:   "Compose",
			Session: session,
			Account: session.Account,
			Folders: session.Account.ListFolders(),
			Success: "Message sent",
		})
		return
	}

	// Check for reply
	replyTo := r.URL.Query().Get("reply")
	var replyMsg *freemail.Message
	if replyTo != "" {
		var folder string
		var uid uint32
		fmt.Sscanf(replyTo, "%s/%d", &folder, &uid)
		if f := session.Account.GetFolder(folder); f != nil {
			replyMsg = f.GetMessage(uid)
		}
	}

	s.renderTemplate(w, "compose.html", &PageData{
		Title:   "Compose",
		Session: session,
		Account: session.Account,
		Folders: session.Account.ListFolders(),
		Message: replyMsg,
	})
}

// handleSettings handles the settings page
func (s *Server) handleSettings(w http.ResponseWriter, r *http.Request) {
	session := s.requireAuth(w, r)
	if session == nil {
		return
	}

	s.renderTemplate(w, "settings.html", &PageData{
		Title:   "Settings",
		Session: session,
		Account: session.Account,
		Folders: session.Account.ListFolders(),
	})
}

// AJAX handlers

// handleAjaxSend handles sending a message via AJAX
func (s *Server) handleAjaxSend(w http.ResponseWriter, r *http.Request) {
	session := s.requireAuth(w, r)
	if session == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse form
	to := r.FormValue("to")
	subject := r.FormValue("subject")
	body := r.FormValue("body")

	// Create message
	msg := freemail.NewMessage()
	msg.From = session.Account.GetEmailAddress()
	toAddr, err := freemail.ParseEmailAddress(to)
	if err != nil {
		http.Error(w, "Invalid recipient address", http.StatusBadRequest)
		return
	}
	msg.To = []*freemail.EmailAddress{toAddr}
	msg.Subject = subject
	msg.Body = []byte(body)
	msg.MessageID = fmt.Sprintf("<%d@gofreemail>", time.Now().UnixNano())

	// Add to sent folder
	session.Account.Sent.AddMessage(msg)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleAjaxDelete handles deleting a message
func (s *Server) handleAjaxDelete(w http.ResponseWriter, r *http.Request) {
	session := s.requireAuth(w, r)
	if session == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	folder := r.FormValue("folder")
	var uid uint32
	fmt.Sscanf(r.FormValue("uid"), "%d", &uid)

	f := session.Account.GetFolder(folder)
	if f == nil {
		http.Error(w, "Folder not found", http.StatusNotFound)
		return
	}

	msg := f.GetMessage(uid)
	if msg == nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	// Mark as deleted
	msg.SetFlag(freemail.FlagDeleted)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleAjaxMarkRead handles marking a message as read
func (s *Server) handleAjaxMarkRead(w http.ResponseWriter, r *http.Request) {
	session := s.requireAuth(w, r)
	if session == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	folder := r.FormValue("folder")
	var uid uint32
	fmt.Sscanf(r.FormValue("uid"), "%d", &uid)

	f := session.Account.GetFolder(folder)
	if f == nil {
		http.Error(w, "Folder not found", http.StatusNotFound)
		return
	}

	msg := f.GetMessage(uid)
	if msg == nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	msg.SetFlag(freemail.FlagSeen)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleAjaxMove handles moving a message to another folder
func (s *Server) handleAjaxMove(w http.ResponseWriter, r *http.Request) {
	session := s.requireAuth(w, r)
	if session == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	srcFolder := r.FormValue("src_folder")
	destFolder := r.FormValue("dest_folder")
	var uid uint32
	fmt.Sscanf(r.FormValue("uid"), "%d", &uid)

	src := session.Account.GetFolder(srcFolder)
	if src == nil {
		http.Error(w, "Source folder not found", http.StatusNotFound)
		return
	}

	dest := session.Account.GetFolder(destFolder)
	if dest == nil {
		http.Error(w, "Destination folder not found", http.StatusNotFound)
		return
	}

	msg := src.GetMessage(uid)
	if msg == nil {
		http.Error(w, "Message not found", http.StatusNotFound)
		return
	}

	// Copy to destination
	copyMsg := *msg
	copyMsg.UID = 0
	dest.AddMessage(&copyMsg)

	// Mark original as deleted
	msg.SetFlag(freemail.FlagDeleted)

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

// handleAjaxStatus returns status information
func (s *Server) handleAjaxStatus(w http.ResponseWriter, r *http.Request) {
	session := s.requireAuth(w, r)
	if session == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Return unread counts
	unread := countUnread(session.Account.Inbox)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"unread": %d}`, unread)
}

// renderTemplate renders a template
func (s *Server) renderTemplate(w http.ResponseWriter, name string, data *PageData) {
	if s.templates == nil {
		http.Error(w, "Templates not loaded", http.StatusInternalServerError)
		return
	}

	err := s.templates.ExecuteTemplate(w, name, data)
	if err != nil {
		log.Printf("Template error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// renderError renders an error page
func (s *Server) renderError(w http.ResponseWriter, message string) {
	s.renderTemplate(w, "error.html", &PageData{
		Title: "Error",
		Error: message,
	})
}

// countUnread counts unread messages in a folder
func countUnread(folder *freemail.Folder) int {
	count := 0
	for _, msg := range folder.Messages {
		if !msg.HasFlag(freemail.FlagSeen) {
			count++
		}
	}
	return count
}
