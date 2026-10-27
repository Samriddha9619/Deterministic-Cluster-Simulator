package sim

import (
	"sync"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// BuggifySite represents a named execution site where rare perturbations can be injected.
type BuggifySite string

const (
	BuggifySlowApply     BuggifySite = "slow_apply"     // Delay before applying state machine entry
	BuggifyEarlyTimeout  BuggifySite = "early_timeout"  // Trigger premature election timeout
	BuggifyDropHeartbeat BuggifySite = "drop_heartbeat" // Drop an individual heartbeat
	BuggifyBatchSplit    BuggifySite = "batch_split"    // Split append entries into single-entry batches
)

// BuggifyRecorder logs activated buggify sites for trace explanation.
type BuggifyRecorder struct {
	mu         sync.Mutex
	activations map[BuggifySite]int
}

// NewBuggifyRecorder creates an empty recorder.
func NewBuggifyRecorder() *BuggifyRecorder {
	return &BuggifyRecorder{
		activations: make(map[BuggifySite]int),
	}
}

// Record notes that a buggify site was triggered.
func (b *BuggifyRecorder) Record(site BuggifySite) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.activations[site]++
}

// Count returns the number of times a site was activated.
func (b *BuggifyRecorder) Count(site BuggifySite) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.activations[site]
}

// ActivatedSites returns a copy of all triggered sites.
func (b *BuggifyRecorder) ActivatedSites() map[BuggifySite]int {
	b.mu.Lock()
	defer b.mu.Unlock()
	res := make(map[BuggifySite]int, len(b.activations))
	for k, v := range b.activations {
		res[k] = v
	}
	return res
}

// Buggify evaluates whether to trigger a rare path perturbation.
// It is active only during deterministic simulation, driven by the simulation RNG.
func Buggify(rng t.Rand, site BuggifySite, prob float64, rec *BuggifyRecorder) bool {
	if prob <= 0.0 || rng == nil {
		return false
	}
	triggered := BoolWithProb(rng, prob)
	if triggered && rec != nil {
		rec.Record(site)
	}
	return triggered
}
