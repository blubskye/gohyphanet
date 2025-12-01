// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package sone

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"  // GIF support
	_ "image/jpeg" // JPEG support
	_ "image/png"  // PNG support
	"log"
	"strings"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/fcp"
)

// ImageManager handles image uploads and retrieval
type ImageManager struct {
	core *Core
	ops  *fcp.Operations

	// Track ongoing uploads
	uploads     map[string]*ImageUpload
	uploadsLock sync.RWMutex

	// Cache for retrieved images
	cache     map[string]*CachedImage
	cacheLock sync.RWMutex
	cacheSize int64
	maxCache  int64 // Maximum cache size in bytes
}

// ImageUpload represents an ongoing image upload
type ImageUpload struct {
	ID          string
	AlbumID     string
	Filename    string
	MimeType    string
	Width       int
	Height      int
	Data        []byte
	Status      UploadStatus
	Progress    float64
	Error       string
	ResultURI   string
	StartTime   time.Time
	Callbacks   []ImageUploadCallback
	callbacksMu sync.Mutex
}

// UploadStatus represents the status of an image upload
type UploadStatus int

const (
	UploadPending UploadStatus = iota
	UploadInProgress
	UploadComplete
	UploadFailed
)

// ImageUploadCallback is called when upload status changes
type ImageUploadCallback func(upload *ImageUpload)

// CachedImage represents a cached image
type CachedImage struct {
	URI       string
	Data      []byte
	MimeType  string
	Width     int
	Height    int
	FetchTime time.Time
}

// NewImageManager creates a new image manager
func NewImageManager(core *Core) *ImageManager {
	return &ImageManager{
		core:     core,
		ops:      fcp.NewOperations(core.fcpClient),
		uploads:  make(map[string]*ImageUpload),
		cache:    make(map[string]*CachedImage),
		maxCache: 50 * 1024 * 1024, // 50MB default cache
	}
}

// UploadImage uploads an image to Hyphanet
func (m *ImageManager) UploadImage(soneID string, albumID string, filename string, data []byte, callback ImageUploadCallback) (*ImageUpload, error) {
	// Detect image format and dimensions
	mimeType, width, height, err := m.analyzeImage(data)
	if err != nil {
		return nil, fmt.Errorf("invalid image: %w", err)
	}

	// Generate unique ID
	uploadID := fmt.Sprintf("img-%s-%d", soneID[:8], time.Now().UnixNano())

	upload := &ImageUpload{
		ID:        uploadID,
		AlbumID:   albumID,
		Filename:  filename,
		MimeType:  mimeType,
		Width:     width,
		Height:    height,
		Data:      data,
		Status:    UploadPending,
		StartTime: time.Now(),
	}

	if callback != nil {
		upload.Callbacks = append(upload.Callbacks, callback)
	}

	m.uploadsLock.Lock()
	m.uploads[uploadID] = upload
	m.uploadsLock.Unlock()

	// Start upload in background
	go m.processUpload(soneID, upload)

	return upload, nil
}

// GetUpload returns an upload by ID
func (m *ImageManager) GetUpload(uploadID string) *ImageUpload {
	m.uploadsLock.RLock()
	defer m.uploadsLock.RUnlock()
	return m.uploads[uploadID]
}

// GetImage retrieves an image from Hyphanet (with caching)
func (m *ImageManager) GetImage(uri string) (*CachedImage, error) {
	// Check cache first
	m.cacheLock.RLock()
	cached, exists := m.cache[uri]
	m.cacheLock.RUnlock()

	if exists {
		return cached, nil
	}

	// Fetch from Hyphanet
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	result, err := m.ops.Get(ctx, uri)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch image: %w", err)
	}

	if !result.Success {
		return nil, fmt.Errorf("image fetch failed: %s", result.Error)
	}

	// Analyze the fetched image
	mimeType, width, height, err := m.analyzeImage(result.Data)
	if err != nil {
		return nil, fmt.Errorf("invalid image data: %w", err)
	}

	img := &CachedImage{
		URI:       uri,
		Data:      result.Data,
		MimeType:  mimeType,
		Width:     width,
		Height:    height,
		FetchTime: time.Now(),
	}

	// Add to cache
	m.addToCache(img)

	return img, nil
}

// processUpload handles the actual upload to Hyphanet
func (m *ImageManager) processUpload(soneID string, upload *ImageUpload) {
	upload.Status = UploadInProgress
	m.notifyCallbacks(upload)

	// Get the local Sone for insert key
	sone := m.core.GetLocalSone(soneID)
	if sone == nil {
		upload.Status = UploadFailed
		upload.Error = "sone not found"
		m.notifyCallbacks(upload)
		return
	}

	// Build insert URI
	// Images go into: InsertURI/images/UPLOAD_ID
	insertURI := fmt.Sprintf("%s/images/%s", sone.InsertURI, upload.ID)

	// Upload with progress tracking
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	result, err := m.ops.PutWithProgress(ctx, insertURI, upload.Data, func(succeeded, total int) {
		if total > 0 {
			upload.Progress = float64(succeeded) / float64(total)
			m.notifyCallbacks(upload)
		}
	})

	if err != nil {
		upload.Status = UploadFailed
		upload.Error = err.Error()
		m.notifyCallbacks(upload)
		return
	}

	if !result.Success {
		upload.Status = UploadFailed
		upload.Error = result.Error
		m.notifyCallbacks(upload)
		return
	}

	// Upload successful
	upload.Status = UploadComplete
	upload.ResultURI = result.URI
	upload.Progress = 1.0
	m.notifyCallbacks(upload)

	// Create Image object and add to album
	m.addImageToAlbum(soneID, upload)

	log.Printf("Image uploaded successfully: %s", upload.ResultURI)
}

// addImageToAlbum adds the uploaded image to the Sone's album
func (m *ImageManager) addImageToAlbum(soneID string, upload *ImageUpload) {
	sone := m.core.GetLocalSone(soneID)
	if sone == nil {
		return
	}

	// Create Image object
	img := &Image{
		ID:       upload.ID,
		SoneID:   soneID,
		AlbumID:  upload.AlbumID,
		Key:      upload.ResultURI,
		Title:    upload.Filename,
		Width:    upload.Width,
		Height:   upload.Height,
		MimeType: upload.MimeType,
	}

	sone.mu.Lock()
	defer sone.mu.Unlock()

	// Find or create the album
	album := m.findOrCreateAlbum(sone, upload.AlbumID)
	album.Images = append(album.Images, img)

	// Update Sone timestamp
	sone.Time = time.Now().UnixMilli()

	// Trigger re-insertion
	if inserter, ok := m.core.soneInserters[soneID]; ok {
		inserter.TriggerInsert()
	}
}

// findOrCreateAlbum finds an existing album or creates a new one
func (m *ImageManager) findOrCreateAlbum(sone *Sone, albumID string) *Album {
	// Search in root album
	if sone.RootAlbum == nil {
		sone.RootAlbum = &Album{
			ID:     "root",
			SoneID: sone.ID,
			Title:  "Root Album",
		}
	}

	// If no album ID specified, use root
	if albumID == "" || albumID == "root" {
		return sone.RootAlbum
	}

	// Search for existing album
	album := m.findAlbumByID(sone.RootAlbum, albumID)
	if album != nil {
		return album
	}

	// Create new album
	newAlbum := &Album{
		ID:     albumID,
		SoneID: sone.ID,
		Title:  albumID,
	}
	sone.RootAlbum.Albums = append(sone.RootAlbum.Albums, newAlbum)

	return newAlbum
}

// findAlbumByID recursively searches for an album
func (m *ImageManager) findAlbumByID(album *Album, id string) *Album {
	if album.ID == id {
		return album
	}

	for _, child := range album.Albums {
		if found := m.findAlbumByID(child, id); found != nil {
			return found
		}
	}

	return nil
}

// notifyCallbacks notifies all callbacks of a status change
func (m *ImageManager) notifyCallbacks(upload *ImageUpload) {
	upload.callbacksMu.Lock()
	callbacks := make([]ImageUploadCallback, len(upload.Callbacks))
	copy(callbacks, upload.Callbacks)
	upload.callbacksMu.Unlock()

	for _, cb := range callbacks {
		cb(upload)
	}
}

// analyzeImage detects the format and dimensions of an image
func (m *ImageManager) analyzeImage(data []byte) (mimeType string, width, height int, err error) {
	reader := bytes.NewReader(data)
	config, format, err := image.DecodeConfig(reader)
	if err != nil {
		return "", 0, 0, err
	}

	// Map format to MIME type
	switch strings.ToLower(format) {
	case "jpeg", "jpg":
		mimeType = "image/jpeg"
	case "png":
		mimeType = "image/png"
	case "gif":
		mimeType = "image/gif"
	default:
		mimeType = "application/octet-stream"
	}

	return mimeType, config.Width, config.Height, nil
}

// addToCache adds an image to the cache
func (m *ImageManager) addToCache(img *CachedImage) {
	m.cacheLock.Lock()
	defer m.cacheLock.Unlock()

	// Evict old entries if cache is too large
	newSize := m.cacheSize + int64(len(img.Data))
	for newSize > m.maxCache && len(m.cache) > 0 {
		// Find oldest entry
		var oldest string
		var oldestTime time.Time
		for uri, cached := range m.cache {
			if oldest == "" || cached.FetchTime.Before(oldestTime) {
				oldest = uri
				oldestTime = cached.FetchTime
			}
		}
		if oldest != "" {
			m.cacheSize -= int64(len(m.cache[oldest].Data))
			delete(m.cache, oldest)
			newSize = m.cacheSize + int64(len(img.Data))
		}
	}

	m.cache[img.URI] = img
	m.cacheSize += int64(len(img.Data))
}

// ClearCache clears the image cache
func (m *ImageManager) ClearCache() {
	m.cacheLock.Lock()
	defer m.cacheLock.Unlock()

	m.cache = make(map[string]*CachedImage)
	m.cacheSize = 0
}

// CreateAlbum creates a new album for a Sone
func (m *ImageManager) CreateAlbum(soneID string, title string, parentID string) (*Album, error) {
	sone := m.core.GetLocalSone(soneID)
	if sone == nil {
		return nil, fmt.Errorf("sone not found: %s", soneID)
	}

	albumID := fmt.Sprintf("album-%d", time.Now().UnixNano())

	album := &Album{
		ID:     albumID,
		SoneID: soneID,
		Title:  title,
	}

	sone.mu.Lock()
	defer sone.mu.Unlock()

	// Ensure root album exists
	if sone.RootAlbum == nil {
		sone.RootAlbum = &Album{
			ID:     "root",
			SoneID: soneID,
			Title:  "Root Album",
		}
	}

	// Find parent album
	parent := sone.RootAlbum
	if parentID != "" && parentID != "root" {
		parent = m.findAlbumByID(sone.RootAlbum, parentID)
		if parent == nil {
			return nil, fmt.Errorf("parent album not found: %s", parentID)
		}
	}

	parent.Albums = append(parent.Albums, album)

	// Update Sone timestamp
	sone.Time = time.Now().UnixMilli()

	return album, nil
}

// DeleteAlbum deletes an album and all its contents
func (m *ImageManager) DeleteAlbum(soneID string, albumID string) error {
	sone := m.core.GetLocalSone(soneID)
	if sone == nil {
		return fmt.Errorf("sone not found: %s", soneID)
	}

	if albumID == "root" {
		return fmt.Errorf("cannot delete root album")
	}

	sone.mu.Lock()
	defer sone.mu.Unlock()

	// Find and remove the album
	if sone.RootAlbum == nil {
		return fmt.Errorf("album not found: %s", albumID)
	}

	removed := m.removeAlbumByID(sone.RootAlbum, albumID)
	if !removed {
		return fmt.Errorf("album not found: %s", albumID)
	}

	// Update Sone timestamp
	sone.Time = time.Now().UnixMilli()

	return nil
}

// removeAlbumByID recursively removes an album
func (m *ImageManager) removeAlbumByID(album *Album, id string) bool {
	for i, child := range album.Albums {
		if child.ID == id {
			album.Albums = append(album.Albums[:i], album.Albums[i+1:]...)
			return true
		}
		if m.removeAlbumByID(child, id) {
			return true
		}
	}
	return false
}

// DeleteImage deletes an image from an album
func (m *ImageManager) DeleteImage(soneID string, imageID string) error {
	sone := m.core.GetLocalSone(soneID)
	if sone == nil {
		return fmt.Errorf("sone not found: %s", soneID)
	}

	sone.mu.Lock()
	defer sone.mu.Unlock()

	if sone.RootAlbum == nil {
		return fmt.Errorf("image not found: %s", imageID)
	}

	removed := m.removeImageByID(sone.RootAlbum, imageID)
	if !removed {
		return fmt.Errorf("image not found: %s", imageID)
	}

	// Update Sone timestamp
	sone.Time = time.Now().UnixMilli()

	return nil
}

// removeImageByID recursively removes an image from albums
func (m *ImageManager) removeImageByID(album *Album, id string) bool {
	for i, img := range album.Images {
		if img.ID == id {
			album.Images = append(album.Images[:i], album.Images[i+1:]...)
			return true
		}
	}

	for _, child := range album.Albums {
		if m.removeImageByID(child, id) {
			return true
		}
	}

	return false
}

// MoveImage moves an image to a different album
func (m *ImageManager) MoveImage(soneID string, imageID string, targetAlbumID string) error {
	sone := m.core.GetLocalSone(soneID)
	if sone == nil {
		return fmt.Errorf("sone not found: %s", soneID)
	}

	sone.mu.Lock()
	defer sone.mu.Unlock()

	if sone.RootAlbum == nil {
		return fmt.Errorf("image not found: %s", imageID)
	}

	// Find the image
	img := m.findImageByID(sone.RootAlbum, imageID)
	if img == nil {
		return fmt.Errorf("image not found: %s", imageID)
	}

	// Find target album
	targetAlbum := m.findAlbumByID(sone.RootAlbum, targetAlbumID)
	if targetAlbum == nil {
		return fmt.Errorf("album not found: %s", targetAlbumID)
	}

	// Remove from current album
	m.removeImageByID(sone.RootAlbum, imageID)

	// Add to target album
	img.AlbumID = targetAlbumID
	targetAlbum.Images = append(targetAlbum.Images, img)

	// Update Sone timestamp
	sone.Time = time.Now().UnixMilli()

	return nil
}

// findImageByID recursively searches for an image
func (m *ImageManager) findImageByID(album *Album, id string) *Image {
	for _, img := range album.Images {
		if img.ID == id {
			return img
		}
	}

	for _, child := range album.Albums {
		if found := m.findImageByID(child, id); found != nil {
			return found
		}
	}

	return nil
}

// GetAlbums returns all albums for a Sone
func (m *ImageManager) GetAlbums(soneID string) []*Album {
	sone := m.core.GetSone(soneID)
	if sone == nil || sone.RootAlbum == nil {
		return nil
	}

	// Return root album and all nested albums
	return []*Album{sone.RootAlbum}
}
