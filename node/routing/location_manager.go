package routing

import (
	"container/list"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"
)

const (
	// Swap constants
	SwapResetInterval  = 16000 // Swaps before random location reset
	SwapTimeout        = 60 * time.Second
	MinSwapTime        = 8 * time.Second
	MaxSwapTime        = 60 * time.Second
	SendSwapInterval   = 8 * time.Second
	SwapLockTimeout    = 10 * time.Second

	// Recently forwarded cleanup
	SwapChainCleanupInterval = 5 * time.Minute
	SwapChainMaxAge          = 2 * SwapTimeout
)

// LocationManager manages node location in the keyspace and location swapping
type LocationManager struct {
	mu sync.RWMutex

	// Current location
	location          float64
	timeLocationSet   time.Time
	locationSetCount  int64
	locationChangeDist float64 // Total distance moved this session

	// Swapping
	averageSwapTime       *BootstrappingDecayingRunningAverage
	swapsSinceReset       int
	locked                bool
	lockedTime            time.Time
	incomingSwapQueue     *list.List
	recentlyForwarded     map[uint64]*RecentlyForwardedItem
	recentlyForwardedLock sync.Mutex

	// Configuration
	enableSwapping     bool
	randomizeLocation  bool
}

// RecentlyForwardedItem tracks recently forwarded swap requests to prevent loops
type RecentlyForwardedItem struct {
	incomingID              uint64
	outgoingID              uint64
	addedTime               time.Time
	lastMessageTime         time.Time
	successfullyForwarded   bool
}

// NewLocationManager creates a new location manager
func NewLocationManager(initialLocation float64, enableSwapping bool) *LocationManager {
	if initialLocation < 0 || initialLocation >= 1.0 {
		// Generate random location
		initialLocation = generateRandomLocation()
	}

	lm := &LocationManager{
		location:            initialLocation,
		timeLocationSet:     time.Now(),
		locationSetCount:    0,
		locationChangeDist:  0,
		enableSwapping:      enableSwapping,
		randomizeLocation:   true,
		incomingSwapQueue:   list.New(),
		recentlyForwarded:   make(map[uint64]*RecentlyForwardedItem),
	}

	// Initialize average swap time (default 30s, range 8-60s, max reports 20)
	lm.averageSwapTime = NewBootstrappingDecayingRunningAverage(30.0, 8.0, 60.0, 20)

	// Start cleanup goroutine
	go lm.cleanupRoutine()

	return lm
}

// GetLocation returns the current location
func (lm *LocationManager) GetLocation() float64 {
	lm.mu.RLock()
	defer lm.mu.RUnlock()
	return lm.location
}

// SetLocation sets a new location
func (lm *LocationManager) SetLocation(newLocation float64) error {
	newLocation = Normalize(newLocation)

	lm.mu.Lock()
	defer lm.mu.Unlock()

	oldLocation := lm.location

	// Calculate distance moved
	dist := Distance(oldLocation, newLocation)
	lm.locationChangeDist += dist

	lm.location = newLocation
	lm.timeLocationSet = time.Now()
	lm.locationSetCount++

	return nil
}

// ShouldSwap implements the Metropolis-Hastings swap decision algorithm
func (lm *LocationManager) ShouldSwap(
	myLoc float64,
	myFriendLocs []float64,
	hisLoc float64,
	hisFriendLocs []float64,
	sharedRandom uint64,
) bool {
	// Prevent swapping with self
	if math.Abs(hisLoc-myLoc) <= 2*math.SmallestNonzeroFloat64 {
		return false
	}

	// Calculate current edge distances (A)
	A := 1.0
	for _, loc := range myFriendLocs {
		if math.Abs(loc-myLoc) > 2*math.SmallestNonzeroFloat64 {
			A *= Distance(loc, myLoc)
		}
	}
	for _, loc := range hisFriendLocs {
		if math.Abs(loc-hisLoc) > 2*math.SmallestNonzeroFloat64 {
			A *= Distance(loc, hisLoc)
		}
	}

	// Calculate swapped edge distances (B)
	B := 1.0
	for _, loc := range myFriendLocs {
		if math.Abs(loc-hisLoc) > 2*math.SmallestNonzeroFloat64 {
			B *= Distance(loc, hisLoc)
		}
	}
	for _, loc := range hisFriendLocs {
		if math.Abs(loc-myLoc) > 2*math.SmallestNonzeroFloat64 {
			B *= Distance(loc, myLoc)
		}
	}

	// If A > B, always swap (improves network)
	if A > B {
		return true
	}

	// Otherwise swap with probability p = A/B
	p := A / B

	// Use shared random for deterministic decision
	randProb := float64(sharedRandom&0x7FFFFFFFFFFFFFFF) / float64(0x7FFFFFFFFFFFFFFF)

	return randProb < p
}

// TrackRecentlyForwarded records a recently forwarded swap request
func (lm *LocationManager) TrackRecentlyForwarded(incomingID, outgoingID uint64) {
	lm.recentlyForwardedLock.Lock()
	defer lm.recentlyForwardedLock.Unlock()

	item := &RecentlyForwardedItem{
		incomingID:  incomingID,
		outgoingID:  outgoingID,
		addedTime:   time.Now(),
		lastMessageTime: time.Now(),
	}

	// Track both IDs to catch loops
	lm.recentlyForwarded[incomingID] = item
	lm.recentlyForwarded[outgoingID] = item
}

// IsRecentlyForwarded checks if a swap UID was recently seen
func (lm *LocationManager) IsRecentlyForwarded(uid uint64) bool {
	lm.recentlyForwardedLock.Lock()
	defer lm.recentlyForwardedLock.Unlock()

	_, exists := lm.recentlyForwarded[uid]
	return exists
}

// RecordSwapTime records the time taken for a swap
func (lm *LocationManager) RecordSwapTime(duration time.Duration) {
	seconds := duration.Seconds()
	lm.averageSwapTime.Report(seconds)
}

// GetAverageSwapTime returns the current average swap time
func (lm *LocationManager) GetAverageSwapTime() time.Duration {
	seconds := lm.averageSwapTime.CurrentValue()
	return time.Duration(seconds * float64(time.Second))
}

// IncrementSwapCount increments the swap counter and checks for reset
func (lm *LocationManager) IncrementSwapCount() bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.swapsSinceReset++

	// Check if we should randomize location
	if lm.randomizeLocation && lm.swapsSinceReset >= SwapResetInterval {
		lm.swapsSinceReset = 0
		newLoc := generateRandomLocation()
		oldLoc := lm.location

		lm.location = newLoc
		lm.timeLocationSet = time.Now()
		lm.locationSetCount++

		// Track distance
		dist := Distance(oldLoc, newLoc)
		lm.locationChangeDist += dist

		return true // Reset occurred
	}

	return false
}

// Lock acquires the swap lock
func (lm *LocationManager) Lock() bool {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if lm.locked {
		// Check if lock has timed out
		if time.Since(lm.lockedTime) > SwapLockTimeout {
			lm.locked = false
		} else {
			return false
		}
	}

	lm.locked = true
	lm.lockedTime = time.Now()
	return true
}

// Unlock releases the swap lock
func (lm *LocationManager) Unlock() {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	lm.locked = false
}

// IsLocked checks if the swap lock is held
func (lm *LocationManager) IsLocked() bool {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	if !lm.locked {
		return false
	}

	// Check timeout
	if time.Since(lm.lockedTime) > SwapLockTimeout {
		return false
	}

	return true
}

// GetStats returns location manager statistics
func (lm *LocationManager) GetStats() LocationStats {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	return LocationStats{
		Location:           lm.location,
		TimeSet:            lm.timeLocationSet,
		SetCount:           lm.locationSetCount,
		TotalDistanceMoved: lm.locationChangeDist,
		SwapsSinceReset:    lm.swapsSinceReset,
		AverageSwapTime:    lm.GetAverageSwapTime(),
		IsLocked:           lm.locked,
	}
}

// LocationStats contains statistics about location management
type LocationStats struct {
	Location           float64
	TimeSet            time.Time
	SetCount           int64
	TotalDistanceMoved float64
	SwapsSinceReset    int
	AverageSwapTime    time.Duration
	IsLocked           bool
}

// String returns a formatted string of location statistics
func (ls LocationStats) String() string {
	return fmt.Sprintf("Location: %.6f, Set %d times, Moved %.6f total, %d swaps since reset, Avg swap: %v",
		ls.Location, ls.SetCount, ls.TotalDistanceMoved, ls.SwapsSinceReset, ls.AverageSwapTime)
}

// cleanupRoutine periodically cleans up old swap tracking data
func (lm *LocationManager) cleanupRoutine() {
	ticker := time.NewTicker(SwapChainCleanupInterval)
	defer ticker.Stop()

	for range ticker.C {
		lm.cleanupOldSwapChains()
	}
}

// cleanupOldSwapChains removes old recently forwarded entries
func (lm *LocationManager) cleanupOldSwapChains() {
	lm.recentlyForwardedLock.Lock()
	defer lm.recentlyForwardedLock.Unlock()

	cutoff := time.Now().Add(-SwapChainMaxAge)

	for _, item := range lm.recentlyForwarded {
		if item.addedTime.Before(cutoff) {
			// Remove both incoming and outgoing IDs
			delete(lm.recentlyForwarded, item.incomingID)
			delete(lm.recentlyForwarded, item.outgoingID)
		}
	}
}

// generateRandomLocation generates a random location in [0.0, 1.0)
func generateRandomLocation() float64 {
	var buf [8]byte
	rand.Read(buf[:])
	u := binary.BigEndian.Uint64(buf[:])

	// Convert to [0.0, 1.0)
	return float64(u&0x7FFFFFFFFFFFFFFF) / float64(0x7FFFFFFFFFFFFFFF)
}
