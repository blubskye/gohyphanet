package routing

import (
	"fmt"
	"math"
)

// Location represents a point in the circular keyspace [0.0, 1.0)
type Location float64

const (
	// LocationInvalid represents an invalid/unset location
	LocationInvalid Location = -1.0

	// MinLocation is the minimum valid location
	MinLocation Location = 0.0

	// MaxLocation is the maximum valid location (exclusive)
	MaxLocation Location = 1.0
)

// Distance calculates the distance between two locations in circular keyspace
// The keyspace wraps around, so distance from 0.9 to 0.1 is 0.2, not 0.8
func Distance(a, b float64) float64 {
	change := b - a

	// Handle wraparound
	if change > 0.5 {
		return change - 1.0
	}
	if change <= -0.5 {
		return change + 1.0
	}

	return math.Abs(change)
}

// Change calculates the signed distance from 'from' to 'to'
// Positive means clockwise, negative means counter-clockwise
func Change(from, to float64) float64 {
	change := to - from

	// Handle wraparound
	if change > 0.5 {
		return change - 1.0
	}
	if change <= -0.5 {
		return change + 1.0
	}

	return change
}

// Normalize ensures a location is in the valid range [0.0, 1.0)
func Normalize(rough float64) float64 {
	// Handle NaN and Inf
	if math.IsNaN(rough) || math.IsInf(rough, 0) {
		return 0.0
	}

	normal := math.Mod(rough, 1.0)
	if normal < 0 {
		return 1.0 + normal
	}
	return normal
}

// IsValid checks if a location is valid
func (loc Location) IsValid() bool {
	f := float64(loc)
	return f >= 0.0 && f < 1.0 && !math.IsNaN(f) && !math.IsInf(f, 0)
}

// DistanceTo calculates distance from this location to another
func (loc Location) DistanceTo(other Location) float64 {
	return Distance(float64(loc), float64(other))
}

// ChangeTo calculates signed change from this location to another
func (loc Location) ChangeTo(other Location) float64 {
	return Change(float64(loc), float64(other))
}

// String returns a formatted string representation
func (loc Location) String() string {
	if !loc.IsValid() {
		return "INVALID"
	}
	return fmt.Sprintf("%.6f", float64(loc))
}

// Equal checks if two locations are approximately equal
func (loc Location) Equal(other Location, epsilon float64) bool {
	return math.Abs(float64(loc)-float64(other)) < epsilon
}

// ClosestLocation returns the closest location from a list
func ClosestLocation(target Location, locations []Location) Location {
	if len(locations) == 0 {
		return LocationInvalid
	}

	closest := locations[0]
	closestDist := target.DistanceTo(closest)

	for _, loc := range locations[1:] {
		dist := target.DistanceTo(loc)
		if dist < closestDist {
			closest = loc
			closestDist = dist
		}
	}

	return closest
}

// BetweenLocations checks if 'middle' is between 'start' and 'end' in circular keyspace
// going clockwise from start to end
func BetweenLocations(start, middle, end Location) bool {
	startF := float64(start)
	middleF := float64(middle)
	endF := float64(end)

	if startF <= endF {
		// Normal case: no wraparound
		return middleF >= startF && middleF <= endF
	} else {
		// Wraparound case
		return middleF >= startF || middleF <= endF
	}
}
