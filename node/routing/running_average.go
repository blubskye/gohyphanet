package routing

import (
	"math"
	"sync"
)

// BootstrappingDecayingRunningAverage maintains a running average with exponential decay
// During the "bootstrapping" phase (first maxReports), it uses a simple average
// After that, it uses exponential weighted moving average with decay factor 1/maxReports
type BootstrappingDecayingRunningAverage struct {
	mu           sync.RWMutex
	maxReports   int64
	reports      int64
	currentValue float64
	min          float64
	max          float64
}

// NewBootstrappingDecayingRunningAverage creates a new running average tracker
func NewBootstrappingDecayingRunningAverage(defaultValue, min, max float64, maxReports int) *BootstrappingDecayingRunningAverage {
	if maxReports <= 0 {
		maxReports = 20 // Default
	}

	return &BootstrappingDecayingRunningAverage{
		maxReports:   int64(maxReports),
		reports:      0,
		currentValue: defaultValue,
		min:          min,
		max:          max,
	}
}

// Report adds a new value to the running average
func (bdra *BootstrappingDecayingRunningAverage) Report(value float64) {
	bdra.mu.Lock()
	defer bdra.mu.Unlock()

	// Validate value
	if value < bdra.min || value > bdra.max ||
		math.IsNaN(value) || math.IsInf(value, 0) {
		return // Ignore invalid values
	}

	// Calculate decay factor
	// First maxReports: decay = 1/(reports+1) - simple average
	// After maxReports: decay = 1/maxReports - exponential decay
	reportsFloat := float64(bdra.reports + 1)
	maxReportsFloat := float64(bdra.maxReports)
	decayFactor := 1.0 / math.Min(reportsFloat, maxReportsFloat)

	// Update: newValue = (value * decay) + (oldValue * (1 - decay))
	bdra.currentValue = (value * decayFactor) +
		(bdra.currentValue * (1.0 - decayFactor))

	bdra.reports++
}

// CurrentValue returns the current average value
func (bdra *BootstrappingDecayingRunningAverage) CurrentValue() float64 {
	bdra.mu.RLock()
	defer bdra.mu.RUnlock()
	return bdra.currentValue
}

// CountReports returns the total number of reports
func (bdra *BootstrappingDecayingRunningAverage) CountReports() int64 {
	bdra.mu.RLock()
	defer bdra.mu.RUnlock()
	return bdra.reports
}

// Reset resets the average to a new default value
func (bdra *BootstrappingDecayingRunningAverage) Reset(newDefault float64) {
	bdra.mu.Lock()
	defer bdra.mu.Unlock()

	bdra.currentValue = newDefault
	bdra.reports = 0
}

// SetMaxReports changes the maximum reports for decay calculation
func (bdra *BootstrappingDecayingRunningAverage) SetMaxReports(maxReports int) {
	bdra.mu.Lock()
	defer bdra.mu.Unlock()

	if maxReports > 0 {
		bdra.maxReports = int64(maxReports)
	}
}
