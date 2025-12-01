package routing

import (
	"math"
	"time"

	"github.com/blubskye/gohyphanet/node/keys"
)

// PeerInterface defines the minimal interface required for routing
type PeerInterface interface {
	GetLocation() float64
	IsRoutable() bool
	IsDisconnecting() bool
	GetClosestPeerLocation(target float64, exclude map[float64]bool) float64
	ShallWeRouteAccordingToOurPeersLocation(htl int16) bool
	IsInMandatoryBackoff(now time.Time, realTime bool) bool
	IsRoutingBackedOff(realTime bool) bool
}

// FailureTableInterface defines the interface for failure tracking
type FailureTableInterface interface {
	GetTimeoutTime(peer PeerInterface, key keys.Key, htl int16, now time.Time, excludeFT bool) time.Time
	HadAnyOffers(key keys.Key) bool
}

// PeerSelector implements the core peer selection algorithm for routing
type PeerSelector struct {
	myLocation    float64
	peers         []PeerInterface
	failureTable  FailureTableInterface
}

// NewPeerSelector creates a new peer selector
func NewPeerSelector(myLocation float64, peers []PeerInterface, failureTable FailureTableInterface) *PeerSelector {
	return &PeerSelector{
		myLocation:   myLocation,
		peers:        peers,
		failureTable: failureTable,
	}
}

// CloserPeer selects the best peer to forward a request to
// This is the CRITICAL routing algorithm
func (ps *PeerSelector) CloserPeer(
	source PeerInterface,
	routedTo map[PeerInterface]bool,
	targetLocation float64,
	key keys.Key,
	htl int16,
	realTime bool,
	now time.Time,
) PeerInterface {

	// Max distance we're willing to consider
	maxDiff := math.MaxFloat64
	if source != nil {
		maxDiff = Distance(ps.myLocation, targetLocation)
	}

	var (
		closestNotBackedOff     PeerInterface
		closestNotBackedOffDist = math.MaxFloat64

		closestBackedOff     PeerInterface
		closestBackedOffDist = math.MaxFloat64

		leastRecentlyTimedOut         PeerInterface
		timeLeastRecentlyTimedOut     = time.Time{}

		leastRecentlyTimedOutBackedOff         PeerInterface
		timeLeastRecentlyTimedOutBackedOff     = time.Time{}
	)

	// Locations to exclude (already visited)
	excludeLocs := make(map[float64]bool)
	excludeLocs[ps.myLocation] = true
	if source != nil {
		excludeLocs[source.GetLocation()] = true
	}
	for peer := range routedTo {
		excludeLocs[peer.GetLocation()] = true
	}

	// Iterate through all peers
	for _, p := range ps.peers {
		// Skip if already routed to
		if routedTo[p] {
			continue
		}

		// Skip source peer
		if p == source {
			continue
		}

		// Skip if not routable
		if !p.IsRoutable() {
			continue
		}

		// Skip if disconnecting
		if p.IsDisconnecting() {
			continue
		}

		// Check if peer is in mandatory backoff
		if p.IsInMandatoryBackoff(now, realTime) {
			continue
		}

		// Get peer location
		peerLoc := p.GetLocation()
		realDiff := Distance(peerLoc, targetLocation)
		diff := realDiff

		// FOAF (Friend-of-a-Friend) routing optimization
		// Route through peer's peers if they published locations
		if p.ShallWeRouteAccordingToOurPeersLocation(htl) {
			closestFOAFLoc := p.GetClosestPeerLocation(targetLocation, excludeLocs)
			if !math.IsNaN(closestFOAFLoc) {
				foafDiff := Distance(closestFOAFLoc, targetLocation)
				if foafDiff < diff {
					peerLoc = closestFOAFLoc
					diff = foafDiff
				}
			}
		}

		// Skip if further than us
		if diff > maxDiff {
			continue
		}

		// Check failure table timeout
		var timeoutTime time.Time
		timedOut := false
		if ps.failureTable != nil && key != nil {
			timeoutTime = ps.failureTable.GetTimeoutTime(p, key, htl, now, true)
			timedOut = now.Before(timeoutTime)
		}

		// Check if peer is backed off
		backedOff := p.IsRoutingBackedOff(realTime)

		// Update closest peers based on backoff/timeout status
		if !backedOff && !timedOut && diff < closestNotBackedOffDist {
			closestNotBackedOff = p
			closestNotBackedOffDist = diff
		} else if backedOff && !timedOut && diff < closestBackedOffDist {
			closestBackedOff = p
			closestBackedOffDist = diff
		} else if timedOut {
			if !backedOff && (timeLeastRecentlyTimedOut.IsZero() || timeoutTime.Before(timeLeastRecentlyTimedOut)) {
				leastRecentlyTimedOut = p
				timeLeastRecentlyTimedOut = timeoutTime
			} else if backedOff && (timeLeastRecentlyTimedOutBackedOff.IsZero() || timeoutTime.Before(timeLeastRecentlyTimedOutBackedOff)) {
				leastRecentlyTimedOutBackedOff = p
				timeLeastRecentlyTimedOutBackedOff = timeoutTime
			}
		}
	}

	// Priority order for peer selection:
	// 1. Closest peer not backed off and not timed out
	// 2. Least recently timed out peer (not backed off)
	// 3. Closest backed-off peer (not timed out)
	// 4. Least recently timed out backed-off peer

	if closestNotBackedOff != nil {
		return closestNotBackedOff
	}
	if leastRecentlyTimedOut != nil {
		return leastRecentlyTimedOut
	}
	if closestBackedOff != nil {
		return closestBackedOff
	}
	if leastRecentlyTimedOutBackedOff != nil {
		return leastRecentlyTimedOutBackedOff
	}

	return nil // Route Not Found
}

// CheckRecentlyFailed checks if we should return RecentlyFailed instead of routing
// This implements request quenching when multiple nodes have recently failed
func (ps *PeerSelector) CheckRecentlyFailed(
	source PeerInterface,
	routedTo map[PeerInterface]bool,
	targetLocation float64,
	key keys.Key,
	htl int16,
	now time.Time,
) *RecentlyFailedReturn {

	if ps.failureTable == nil || key == nil {
		return nil
	}

	// Need at least 3 peers OR 25% of total peers waiting
	peerCount := len(ps.peers)
	minCountWaiting := 3
	maxCountWaiting := peerCount / 4
	if maxCountWaiting < minCountWaiting {
		maxCountWaiting = minCountWaiting
	}

	// Count how many peers are in timeout for this key
	countWaiting := 0
	var soonestWakeup time.Time

	for _, p := range ps.peers {
		timeout := ps.failureTable.GetTimeoutTime(p, key, htl, now, false)
		if now.Before(timeout) {
			countWaiting++
			if soonestWakeup.IsZero() || timeout.Before(soonestWakeup) {
				soonestWakeup = timeout
			}
		}
	}

	if countWaiting < maxCountWaiting {
		return nil // Not enough waiting
	}

	// Find top 2 routing choices (ignoring timeouts)
	first := ps.CloserPeer(source, routedTo, targetLocation, key, htl, false, now)
	if first == nil {
		return nil
	}

	firstTimeout := ps.failureTable.GetTimeoutTime(first, key, htl, now, false)
	if !now.Before(firstTimeout) {
		return nil // First choice not in timeout
	}

	// Try second choice
	routedToCopy := make(map[PeerInterface]bool)
	for k, v := range routedTo {
		routedToCopy[k] = v
	}
	routedToCopy[first] = true

	second := ps.CloserPeer(source, routedToCopy, targetLocation, key, htl, false, now)
	if second == nil {
		return nil
	}

	secondTimeout := ps.failureTable.GetTimeoutTime(second, key, htl, now, false)
	if !now.Before(secondTimeout) {
		return nil // Second choice not in timeout
	}

	// Both top choices are in timeout - return RecentlyFailed!
	until := firstTimeout
	if secondTimeout.Before(until) {
		until = secondTimeout
	}
	if countWaiting == maxCountWaiting && !soonestWakeup.IsZero() && soonestWakeup.Before(until) {
		until = soonestWakeup
	}

	// Don't return RecentlyFailed if we have an offer for this key
	if ps.failureTable.HadAnyOffers(key) {
		return nil
	}

	return &RecentlyFailedReturn{
		CountWaiting:      countWaiting,
		RecentlyFailedTime: until,
	}
}

// RecentlyFailedReturn contains information about a recently failed check
type RecentlyFailedReturn struct {
	CountWaiting      int
	RecentlyFailedTime time.Time
}
