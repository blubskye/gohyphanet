// GoHyphanet - Freenet/Hyphanet FCP Library and Tools
// Copyright (C) 2025 GoHyphanet Contributors
// Licensed under GNU AGPLv3 - see LICENSE file for details
// Source: https://github.com/blubskye/gohyphanet

package web

import (
	"encoding/json"
	"net/http"
)

// AjaxResponse is the standard AJAX response format
type AjaxResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// writeJSON writes a JSON response
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

// writeSuccess writes a successful JSON response
func writeSuccess(w http.ResponseWriter, message string, data interface{}) {
	writeJSON(w, http.StatusOK, AjaxResponse{
		Success: true,
		Message: message,
		Data:    data,
	})
}

// writeError writes an error JSON response
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, AjaxResponse{
		Success: false,
		Error:   message,
	})
}

// handleAjaxCreatePost handles creating a new post
func (s *Server) handleAjaxCreatePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	currentSone := s.getCurrentSone(r)
	if currentSone == nil {
		writeError(w, http.StatusUnauthorized, "Not logged in")
		return
	}

	text := r.FormValue("text")
	if text == "" {
		writeError(w, http.StatusBadRequest, "Post text is required")
		return
	}

	var recipientID *string
	if r.FormValue("recipient") != "" {
		rid := r.FormValue("recipient")
		recipientID = &rid
	}

	post, err := s.core.CreatePost(currentSone.ID, text, recipientID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, "Post created", map[string]string{
		"postId": post.ID,
	})
}

// handleAjaxCreateReply handles creating a reply
func (s *Server) handleAjaxCreateReply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	currentSone := s.getCurrentSone(r)
	if currentSone == nil {
		writeError(w, http.StatusUnauthorized, "Not logged in")
		return
	}

	postID := r.FormValue("postId")
	text := r.FormValue("text")

	if postID == "" || text == "" {
		writeError(w, http.StatusBadRequest, "Post ID and text are required")
		return
	}

	reply, err := s.core.CreateReply(currentSone.ID, postID, text)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, "Reply created", map[string]string{
		"replyId": reply.ID,
	})
}

// handleAjaxDeletePost handles deleting a post
func (s *Server) handleAjaxDeletePost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	currentSone := s.getCurrentSone(r)
	if currentSone == nil {
		writeError(w, http.StatusUnauthorized, "Not logged in")
		return
	}

	postID := r.FormValue("postId")
	if postID == "" {
		writeError(w, http.StatusBadRequest, "Post ID is required")
		return
	}

	// Verify ownership
	post := s.core.Database().GetPost(postID)
	if post == nil {
		writeError(w, http.StatusNotFound, "Post not found")
		return
	}
	if post.SoneID != currentSone.ID {
		writeError(w, http.StatusForbidden, "Cannot delete another Sone's post")
		return
	}

	err := s.core.DeletePost(currentSone.ID, postID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, "Post deleted", nil)
}

// handleAjaxDeleteReply handles deleting a reply
func (s *Server) handleAjaxDeleteReply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	currentSone := s.getCurrentSone(r)
	if currentSone == nil {
		writeError(w, http.StatusUnauthorized, "Not logged in")
		return
	}

	replyID := r.FormValue("replyId")
	if replyID == "" {
		writeError(w, http.StatusBadRequest, "Reply ID is required")
		return
	}

	// Verify ownership
	reply := s.core.Database().GetReply(replyID)
	if reply == nil {
		writeError(w, http.StatusNotFound, "Reply not found")
		return
	}
	if reply.SoneID != currentSone.ID {
		writeError(w, http.StatusForbidden, "Cannot delete another Sone's reply")
		return
	}

	err := s.core.DeleteReply(currentSone.ID, replyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, "Reply deleted", nil)
}

// handleAjaxLike handles liking a post or reply
func (s *Server) handleAjaxLike(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	currentSone := s.getCurrentSone(r)
	if currentSone == nil {
		writeError(w, http.StatusUnauthorized, "Not logged in")
		return
	}

	postID := r.FormValue("postId")
	replyID := r.FormValue("replyId")

	if postID != "" {
		err := s.core.LikePost(currentSone.ID, postID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeSuccess(w, "Post liked", nil)
	} else if replyID != "" {
		currentSone.LikeReply(replyID)
		writeSuccess(w, "Reply liked", nil)
	} else {
		writeError(w, http.StatusBadRequest, "Post ID or Reply ID required")
	}
}

// handleAjaxUnlike handles unliking a post or reply
func (s *Server) handleAjaxUnlike(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	currentSone := s.getCurrentSone(r)
	if currentSone == nil {
		writeError(w, http.StatusUnauthorized, "Not logged in")
		return
	}

	postID := r.FormValue("postId")
	replyID := r.FormValue("replyId")

	if postID != "" {
		err := s.core.UnlikePost(currentSone.ID, postID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeSuccess(w, "Post unliked", nil)
	} else if replyID != "" {
		currentSone.UnlikeReply(replyID)
		writeSuccess(w, "Reply unliked", nil)
	} else {
		writeError(w, http.StatusBadRequest, "Post ID or Reply ID required")
	}
}

// handleAjaxFollow handles following a Sone
func (s *Server) handleAjaxFollow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	currentSone := s.getCurrentSone(r)
	if currentSone == nil {
		writeError(w, http.StatusUnauthorized, "Not logged in")
		return
	}

	soneID := r.FormValue("soneId")
	if soneID == "" {
		writeError(w, http.StatusBadRequest, "Sone ID is required")
		return
	}

	err := s.core.FollowSone(currentSone.ID, soneID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, "Now following", nil)
}

// handleAjaxUnfollow handles unfollowing a Sone
func (s *Server) handleAjaxUnfollow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	currentSone := s.getCurrentSone(r)
	if currentSone == nil {
		writeError(w, http.StatusUnauthorized, "Not logged in")
		return
	}

	soneID := r.FormValue("soneId")
	if soneID == "" {
		writeError(w, http.StatusBadRequest, "Sone ID is required")
		return
	}

	err := s.core.UnfollowSone(currentSone.ID, soneID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, "Unfollowed", nil)
}

// handleAjaxBookmark handles bookmarking a post
func (s *Server) handleAjaxBookmark(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	postID := r.FormValue("postId")
	if postID == "" {
		writeError(w, http.StatusBadRequest, "Post ID is required")
		return
	}

	err := s.core.Database().AddBookmark(postID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, "Post bookmarked", nil)
}

// handleAjaxUnbookmark handles removing a bookmark
func (s *Server) handleAjaxUnbookmark(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	postID := r.FormValue("postId")
	if postID == "" {
		writeError(w, http.StatusBadRequest, "Post ID is required")
		return
	}

	err := s.core.Database().RemoveBookmark(postID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeSuccess(w, "Bookmark removed", nil)
}

// StatusResponse contains status information
type StatusResponse struct {
	LocalSones    int    `json:"localSones"`
	KnownSones    int    `json:"knownSones"`
	Posts         int    `json:"posts"`
	Notifications int    `json:"notifications"`
	Status        string `json:"status"`
}

// handleAjaxStatus handles status requests
func (s *Server) handleAjaxStatus(w http.ResponseWriter, r *http.Request) {
	status := &StatusResponse{
		LocalSones:    len(s.core.GetLocalSones()),
		KnownSones:    len(s.core.GetAllSones()),
		Posts:         len(s.core.Database().GetAllPosts()),
		Notifications: len(s.core.GetNotifications()),
		Status:        "running",
	}

	writeSuccess(w, "", status)
}

// handleAjaxNotifications handles notification requests
func (s *Server) handleAjaxNotifications(w http.ResponseWriter, r *http.Request) {
	notifications := s.core.GetNotifications()
	writeSuccess(w, "", notifications)
}

// handleAjaxDismissNotification handles dismissing a notification
func (s *Server) handleAjaxDismissNotification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	notificationID := r.FormValue("notificationId")
	if notificationID == "" {
		writeError(w, http.StatusBadRequest, "Notification ID is required")
		return
	}

	s.core.DismissNotification(notificationID)
	writeSuccess(w, "Notification dismissed", nil)
}
