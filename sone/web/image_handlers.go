// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/blubskye/gohyphanet/sone"
)

// RegisterImageHandlers registers image-related HTTP handlers
func (s *Server) RegisterImageHandlers() {
	// Image upload
	s.mux.HandleFunc("/ajax/upload-image", s.handleUploadImage)

	// Image fetch/proxy
	s.mux.HandleFunc("/image/", s.handleGetImage)

	// Album management
	s.mux.HandleFunc("/ajax/create-album", s.handleCreateAlbum)
	s.mux.HandleFunc("/ajax/delete-album", s.handleDeleteAlbum)
	s.mux.HandleFunc("/ajax/delete-image", s.handleDeleteImage)
	s.mux.HandleFunc("/ajax/move-image", s.handleMoveImage)

	// Album/image pages
	s.mux.HandleFunc("/albums/", s.handleAlbumsPage)
}

// handleUploadImage handles image upload requests
func (s *Server) handleUploadImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	currentSone := s.getCurrentSone(r)
	if currentSone == nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   "Not logged in",
		})
		return
	}

	// Parse multipart form (max 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   "Failed to parse form: " + err.Error(),
		})
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   "No image file provided",
		})
		return
	}
	defer file.Close()

	// Read file data
	data, err := io.ReadAll(file)
	if err != nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   "Failed to read image",
		})
		return
	}

	// Get album ID if provided
	albumID := r.FormValue("album")
	if albumID == "" {
		albumID = "root"
	}

	// Get image manager
	imgMgr := sone.NewImageManager(s.core)

	// Upload image
	upload, err := imgMgr.UploadImage(currentSone.ID, albumID, header.Filename, data, nil)
	if err != nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	jsonResponse(w, map[string]interface{}{
		"success":  true,
		"uploadId": upload.ID,
		"message":  "Image upload started",
	})
}

// handleGetImage proxies image requests through fproxy
func (s *Server) handleGetImage(w http.ResponseWriter, r *http.Request) {
	// Extract URI from path: /image/USK@.../path
	uri := strings.TrimPrefix(r.URL.Path, "/image/")
	if uri == "" {
		http.Error(w, "No URI provided", http.StatusBadRequest)
		return
	}

	// Get image manager
	imgMgr := sone.NewImageManager(s.core)

	// Fetch image
	img, err := imgMgr.GetImage(uri)
	if err != nil {
		http.Error(w, "Image not found: "+err.Error(), http.StatusNotFound)
		return
	}

	// Set content type and serve
	w.Header().Set("Content-Type", img.MimeType)
	w.Header().Set("Cache-Control", "public, max-age=86400") // Cache for 24 hours
	w.Write(img.Data)
}

// handleCreateAlbum creates a new album
func (s *Server) handleCreateAlbum(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	currentSone := s.getCurrentSone(r)
	if currentSone == nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   "Not logged in",
		})
		return
	}

	var req struct {
		Title    string `json:"title"`
		ParentID string `json:"parentId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   "Invalid request",
		})
		return
	}

	if req.Title == "" {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   "Album title is required",
		})
		return
	}

	imgMgr := sone.NewImageManager(s.core)
	album, err := imgMgr.CreateAlbum(currentSone.ID, req.Title, req.ParentID)
	if err != nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	jsonResponse(w, map[string]interface{}{
		"success": true,
		"albumId": album.ID,
	})
}

// handleDeleteAlbum deletes an album
func (s *Server) handleDeleteAlbum(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	currentSone := s.getCurrentSone(r)
	if currentSone == nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   "Not logged in",
		})
		return
	}

	var req struct {
		AlbumID string `json:"albumId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   "Invalid request",
		})
		return
	}

	imgMgr := sone.NewImageManager(s.core)
	if err := imgMgr.DeleteAlbum(currentSone.ID, req.AlbumID); err != nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	jsonResponse(w, map[string]interface{}{
		"success": true,
	})
}

// handleDeleteImage deletes an image
func (s *Server) handleDeleteImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	currentSone := s.getCurrentSone(r)
	if currentSone == nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   "Not logged in",
		})
		return
	}

	var req struct {
		ImageID string `json:"imageId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   "Invalid request",
		})
		return
	}

	imgMgr := sone.NewImageManager(s.core)
	if err := imgMgr.DeleteImage(currentSone.ID, req.ImageID); err != nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	jsonResponse(w, map[string]interface{}{
		"success": true,
	})
}

// handleMoveImage moves an image to a different album
func (s *Server) handleMoveImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	currentSone := s.getCurrentSone(r)
	if currentSone == nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   "Not logged in",
		})
		return
	}

	var req struct {
		ImageID string `json:"imageId"`
		AlbumID string `json:"albumId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   "Invalid request",
		})
		return
	}

	imgMgr := sone.NewImageManager(s.core)
	if err := imgMgr.MoveImage(currentSone.ID, req.ImageID, req.AlbumID); err != nil {
		jsonResponse(w, map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	jsonResponse(w, map[string]interface{}{
		"success": true,
	})
}

// handleAlbumsPage shows a Sone's albums
func (s *Server) handleAlbumsPage(w http.ResponseWriter, r *http.Request) {
	// Extract Sone ID from path: /albums/SONE_ID
	soneID := strings.TrimPrefix(r.URL.Path, "/albums/")
	if soneID == "" {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	viewedSone := s.core.GetSone(soneID)
	if viewedSone == nil {
		http.Error(w, "Sone not found", http.StatusNotFound)
		return
	}

	imgMgr := sone.NewImageManager(s.core)
	albums := imgMgr.GetAlbums(soneID)

	data := s.baseData(r)
	data["Title"] = viewedSone.Name + " - Albums"
	data["ViewedSone"] = viewedSone
	data["Albums"] = albums
	data["IsOwnSone"] = s.getCurrentSone(r) != nil && s.getCurrentSone(r).ID == soneID

	s.render(w, "albums.html", data)
}
