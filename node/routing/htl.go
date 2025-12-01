package routing

import "math/rand"

const (
	// MaxHTL is the maximum hops to live
	MaxHTL = 18

	// SwapMaxHTL is the maximum HTL for location swap requests
	SwapMaxHTL = 10

	// MaxHighHTLFailures is the max failures allowed in high-HTL (no-cache) zone
	MaxHighHTLFailures = 5
)

// HTLManager handles HTL (Hops To Live) decrement logic
type HTLManager struct {
	disableProbabilisticHTLs bool
	decrementHTLAtMaximum    bool
	decrementHTLAtMinimum    bool
}

// NewHTLManager creates a new HTL manager
func NewHTLManager(disableProbabilisticHTLs bool) *HTLManager {
	return &HTLManager{
		disableProbabilisticHTLs: disableProbabilisticHTLs,
		decrementHTLAtMaximum:    rand.Float64() < 0.5,
		decrementHTLAtMinimum:    rand.Float64() < 0.5,
	}
}

// DecrementHTL decrements HTL according to Freenet's probabilistic algorithm
func (hm *HTLManager) DecrementHTL(htl int16) int16 {
	// Clamp to max
	if htl > MaxHTL {
		htl = MaxHTL
	}

	// Already zero
	if htl <= 0 {
		return 0
	}

	// At maximum HTL
	if htl == MaxHTL {
		// Probabilistic: sometimes don't decrement
		// This creates "no-cache" zone near originator for anonymity
		if hm.decrementHTLAtMaximum || hm.disableProbabilisticHTLs {
			htl--
		}
		return htl
	}

	// At minimum HTL (1)
	if htl == 1 {
		// Probabilistic: sometimes don't decrement
		// This extends reach slightly
		if hm.decrementHTLAtMinimum || hm.disableProbabilisticHTLs {
			htl--
		}
		return htl
	}

	// Middle range: always decrement
	htl--
	return htl
}

// CanWriteDatastore checks if HTL is low enough to write to datastore
// High HTL means we're in the "no-cache" zone near the originator
func CanWriteDatastore(htl int16) bool {
	// Can write if HTL is less than max (not in no-cache zone)
	return htl < MaxHTL
}

// IsHighHTL checks if this is a high HTL (no-cache zone)
func IsHighHTL(htl int16) bool {
	return htl >= MaxHTL
}

// ClampHTL ensures HTL is in valid range
func ClampHTL(htl int16) int16 {
	if htl < 0 {
		return 0
	}
	if htl > MaxHTL {
		return MaxHTL
	}
	return htl
}
