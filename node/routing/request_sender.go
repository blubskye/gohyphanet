package routing

import (
	"fmt"
	"sync"
	"time"

	"github.com/blubskye/gohyphanet/node/keys"
)

// RequestStatus represents the status of a request
type RequestStatus int

const (
	// NotFinished means the request is still in progress
	NotFinished RequestStatus = iota
	// Success means the request succeeded
	Success
	// RouteNotFound means no suitable peer could be found
	RouteNotFound
	// DataNotFound means the data doesn't exist in the network
	DataNotFound
	// TransferFailed means data transfer failed
	TransferFailed
	// VerifyFailure means data verification failed
	VerifyFailure
	// TimedOut means the request timed out
	TimedOut
	// RejectedOverload means the request was rejected due to overload
	RejectedOverload
	// InternalError means an internal error occurred
	InternalError
	// RecentlyFailed means multiple nodes recently failed for this key
	RecentlyFailed
)

// String returns a string representation of the status
func (rs RequestStatus) String() string {
	switch rs {
	case NotFinished:
		return "NotFinished"
	case Success:
		return "Success"
	case RouteNotFound:
		return "RouteNotFound"
	case DataNotFound:
		return "DataNotFound"
	case TransferFailed:
		return "TransferFailed"
	case VerifyFailure:
		return "VerifyFailure"
	case TimedOut:
		return "TimedOut"
	case RejectedOverload:
		return "RejectedOverload"
	case InternalError:
		return "InternalError"
	case RecentlyFailed:
		return "RecentlyFailed"
	default:
		return fmt.Sprintf("Unknown(%d)", rs)
	}
}

// RequestSender manages the state of an outgoing request
type RequestSender struct {
	mu sync.RWMutex

	// Request identification
	uid    uint64
	key    keys.Key
	target float64 // Target location derived from key

	// HTL management
	htl                 int16
	origHTL             int16
	highHTLFailureCount int

	// Routing state
	nodesRoutedTo map[PeerInterface]bool
	routeAttempts int

	// Source tracking
	source      PeerInterface
	successFrom PeerInterface

	// Timing
	searchTimeout time.Duration
	startTime     time.Time

	// Status
	status         RequestStatus
	receivingAsync bool
}

// NewRequestSender creates a new request sender
func NewRequestSender(uid uint64, key keys.Key, htl int16, source PeerInterface, timeout time.Duration) *RequestSender {
	// Calculate target location from key
	target := key.ToNormalizedDouble()

	return &RequestSender{
		uid:           uid,
		key:           key,
		target:        target,
		htl:           ClampHTL(htl),
		origHTL:       ClampHTL(htl),
		nodesRoutedTo: make(map[PeerInterface]bool),
		source:        source,
		searchTimeout: timeout,
		startTime:     time.Now(),
		status:        NotFinished,
	}
}

// GetUID returns the request UID
func (rs *RequestSender) GetUID() uint64 {
	return rs.uid
}

// GetKey returns the request key
func (rs *RequestSender) GetKey() keys.Key {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.key
}

// GetHTL returns the current HTL
func (rs *RequestSender) GetHTL() int16 {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.htl
}

// DecrementHTL decrements the HTL using the provided HTL manager
func (rs *RequestSender) DecrementHTL(htlMgr *HTLManager) int16 {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.htl = htlMgr.DecrementHTL(rs.htl)
	return rs.htl
}

// GetTargetLocation returns the target location
func (rs *RequestSender) GetTargetLocation() float64 {
	return rs.target
}

// AddRoutedTo adds a peer to the routing history
func (rs *RequestSender) AddRoutedTo(peer PeerInterface) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.nodesRoutedTo[peer] = true
	rs.routeAttempts++
}

// GetRoutedTo returns a copy of the routing history
func (rs *RequestSender) GetRoutedTo() map[PeerInterface]bool {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	copy := make(map[PeerInterface]bool)
	for k, v := range rs.nodesRoutedTo {
		copy[k] = v
	}
	return copy
}

// IncrementHighHTLFailureCount increments the high HTL failure counter
func (rs *RequestSender) IncrementHighHTLFailureCount() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.highHTLFailureCount++
	return rs.highHTLFailureCount
}

// GetHighHTLFailureCount returns the high HTL failure count
func (rs *RequestSender) GetHighHTLFailureCount() int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.highHTLFailureCount
}

// SetStatus sets the request status
func (rs *RequestSender) SetStatus(status RequestStatus) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.status = status
}

// GetStatus returns the current status
func (rs *RequestSender) GetStatus() RequestStatus {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.status
}

// IsFinished checks if the request is finished
func (rs *RequestSender) IsFinished() bool {
	status := rs.GetStatus()
	return status != NotFinished
}

// SetSuccessFrom records which peer the data came from
func (rs *RequestSender) SetSuccessFrom(peer PeerInterface) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.successFrom = peer
}

// GetSuccessFrom returns which peer the data came from
func (rs *RequestSender) GetSuccessFrom() PeerInterface {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.successFrom
}

// GetElapsedTime returns how long the request has been running
func (rs *RequestSender) GetElapsedTime() time.Duration {
	return time.Since(rs.startTime)
}

// IsTimedOut checks if the request has timed out
func (rs *RequestSender) IsTimedOut() bool {
	return time.Since(rs.startTime) > rs.searchTimeout
}

// GetRouteAttempts returns the number of routing attempts
func (rs *RequestSender) GetRouteAttempts() int {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	return rs.routeAttempts
}

// GetStats returns request statistics
func (rs *RequestSender) GetStats() RequestStats {
	rs.mu.RLock()
	defer rs.mu.RUnlock()

	return RequestStats{
		UID:                 rs.uid,
		Status:              rs.status,
		HTL:                 rs.htl,
		OrigHTL:             rs.origHTL,
		RouteAttempts:       rs.routeAttempts,
		HighHTLFailures:     rs.highHTLFailureCount,
		ElapsedTime:         time.Since(rs.startTime),
		NodesRoutedToCount:  len(rs.nodesRoutedTo),
	}
}

// RequestStats contains statistics about a request
type RequestStats struct {
	UID                uint64
	Status             RequestStatus
	HTL                int16
	OrigHTL            int16
	RouteAttempts      int
	HighHTLFailures    int
	ElapsedTime        time.Duration
	NodesRoutedToCount int
}

// String returns a formatted string of request statistics
func (rs RequestStats) String() string {
	return fmt.Sprintf("Request UID=%d, Status=%s, HTL=%d/%d, Attempts=%d, Time=%v",
		rs.UID, rs.Status, rs.HTL, rs.OrigHTL, rs.RouteAttempts, rs.ElapsedTime)
}
