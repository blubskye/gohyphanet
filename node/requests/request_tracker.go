package requests

import (
	"fmt"
	"sync"
	"time"
)

// RequestTracker manages active requests
type RequestTracker struct {
	mu sync.RWMutex

	// Active requests by UID
	handlers map[uint64]*RequestHandler

	// Statistics
	totalRequests     int64
	completedRequests int64
	successfulRequests int64
	failedRequests    int64

	// Limits
	maxActiveRequests int
}

// NewRequestTracker creates a new request tracker
func NewRequestTracker(maxActiveRequests int) *RequestTracker {
	if maxActiveRequests <= 0 {
		maxActiveRequests = 1000 // Default limit
	}

	return &RequestTracker{
		handlers:          make(map[uint64]*RequestHandler),
		maxActiveRequests: maxActiveRequests,
	}
}

// RegisterRequest registers a new request handler
func (rt *RequestTracker) RegisterRequest(handler *RequestHandler) error {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	// Check limit
	if len(rt.handlers) >= rt.maxActiveRequests {
		return fmt.Errorf("too many active requests (%d/%d)", len(rt.handlers), rt.maxActiveRequests)
	}

	// Check for UID collision
	if _, exists := rt.handlers[handler.uid]; exists {
		return fmt.Errorf("request with UID %d already exists", handler.uid)
	}

	rt.handlers[handler.uid] = handler
	rt.totalRequests++

	return nil
}

// UnregisterRequest removes a request handler
func (rt *RequestTracker) UnregisterRequest(uid uint64) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	handler, exists := rt.handlers[uid]
	if !exists {
		return
	}

	delete(rt.handlers, uid)

	// Update statistics
	rt.completedRequests++
	if handler.GetStatus().IsSuccess() {
		rt.successfulRequests++
	} else if handler.GetStatus().IsFailure() {
		rt.failedRequests++
	}
}

// GetRequest retrieves a request handler by UID
func (rt *RequestTracker) GetRequest(uid uint64) *RequestHandler {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	return rt.handlers[uid]
}

// GetActiveRequests returns all active request handlers
func (rt *RequestTracker) GetActiveRequests() []*RequestHandler {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	handlers := make([]*RequestHandler, 0, len(rt.handlers))
	for _, h := range rt.handlers {
		handlers = append(handlers, h)
	}

	return handlers
}

// GetActiveRequestCount returns the number of active requests
func (rt *RequestTracker) GetActiveRequestCount() int {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	return len(rt.handlers)
}

// CleanupTimedOut removes requests that have exceeded their deadlines
func (rt *RequestTracker) CleanupTimedOut() int {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	now := time.Now()
	toRemove := []uint64{}

	for uid, handler := range rt.handlers {
		handler.mu.RLock()
		deadline := handler.responseDeadline
		status := handler.status
		handler.mu.RUnlock()

		// Remove if past deadline and not finished
		if now.After(deadline) && !status.IsTerminal() {
			toRemove = append(toRemove, uid)
		}
	}

	for _, uid := range toRemove {
		handler := rt.handlers[uid]
		handler.setStatus(StatusTimedOut)
		delete(rt.handlers, uid)
		rt.completedRequests++
		rt.failedRequests++
	}

	return len(toRemove)
}

// GetStats returns tracker statistics
func (rt *RequestTracker) GetStats() RequestTrackerStats {
	rt.mu.RLock()
	defer rt.mu.RUnlock()

	return RequestTrackerStats{
		ActiveRequests:     len(rt.handlers),
		MaxActiveRequests:  rt.maxActiveRequests,
		TotalRequests:      rt.totalRequests,
		CompletedRequests:  rt.completedRequests,
		SuccessfulRequests: rt.successfulRequests,
		FailedRequests:     rt.failedRequests,
	}
}

// RequestTrackerStats contains statistics about the request tracker
type RequestTrackerStats struct {
	ActiveRequests     int
	MaxActiveRequests  int
	TotalRequests      int64
	CompletedRequests  int64
	SuccessfulRequests int64
	FailedRequests     int64
}

// String returns a formatted string of tracker statistics
func (rts RequestTrackerStats) String() string {
	successRate := float64(0)
	if rts.CompletedRequests > 0 {
		successRate = float64(rts.SuccessfulRequests) / float64(rts.CompletedRequests) * 100
	}

	return fmt.Sprintf("Requests: %d/%d active, %d total, %d completed (%.1f%% success)",
		rts.ActiveRequests, rts.MaxActiveRequests, rts.TotalRequests,
		rts.CompletedRequests, successRate)
}

// StartCleanupRoutine starts a goroutine that periodically cleans up timed out requests
func (rt *RequestTracker) StartCleanupRoutine(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			cleaned := rt.CleanupTimedOut()
			if cleaned > 0 {
				// Log cleanup if desired
				_ = cleaned
			}
		}
	}()
}
