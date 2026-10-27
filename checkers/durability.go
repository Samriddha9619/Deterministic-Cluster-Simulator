package checkers

import (
	"fmt"
	"sort"
	"sync"

	"github.com/Samriddha9619/deterministic-cluster-simulator/harness"
	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// DurabilityChecker verifies that all acknowledged client writes survive crashes,
// restarts, and network partitions.
type DurabilityChecker struct {
	mu            sync.Mutex
	durableState  map[string]durableEntry           // key -> last confirmed write
	subsequentOps map[string][]harness.HistoryEntry // key -> ops invoked at or after last confirmed write
}

type durableEntry struct {
	Value     string
	Deleted   bool
	RequestID uint64
	At        t.Time
}

// NewDurabilityChecker creates an empty durability checker.
func NewDurabilityChecker() *DurabilityChecker {
	return &DurabilityChecker{
		durableState:  make(map[string]durableEntry),
		subsequentOps: make(map[string][]harness.HistoryEntry),
	}
}

// RecordAck logs an acknowledged write completion.
func (dc *DurabilityChecker) RecordAck(entry harness.HistoryEntry) {
	if entry.Kind != harness.KindOK {
		return
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	switch entry.Op {
	case harness.OpPut:
		dc.durableState[entry.Key] = durableEntry{
			Value:     entry.Value,
			Deleted:   false,
			RequestID: entry.RequestID,
			At:        entry.At,
		}
		dc.subsequentOps[entry.Key] = nil
	case harness.OpDelete:
		dc.durableState[entry.Key] = durableEntry{
			Value:     "",
			Deleted:   true,
			RequestID: entry.RequestID,
			At:        entry.At,
		}
		dc.subsequentOps[entry.Key] = nil
	case harness.OpCAS:
		dc.durableState[entry.Key] = durableEntry{
			Value:     entry.Value,
			Deleted:   false,
			RequestID: entry.RequestID,
			At:        entry.At,
		}
		dc.subsequentOps[entry.Key] = nil
	}
}

// RecordIndeterminate logs an indeterminate operation that was invoked and may or may not have committed.
func (dc *DurabilityChecker) RecordIndeterminate(entry harness.HistoryEntry) {
	if entry.Kind != harness.KindInfo {
		return
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	last, exists := dc.durableState[entry.Key]
	if !exists || entry.At >= last.At {
		dc.subsequentOps[entry.Key] = append(dc.subsequentOps[entry.Key], entry)
	}
}

// CheckFinalState checks that every live node's KV store contains all acknowledged writes.
func (dc *DurabilityChecker) CheckFinalState(snapshots []t.NodeView) *Violation {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Sort keys for deterministic error reporting
	keys := make([]string, 0, len(dc.durableState))
	for k := range dc.durableState {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, snap := range snapshots {
		if !snap.Alive {
			continue
		}

		for _, k := range keys {
			expected := dc.durableState[k]
			actualVal, exists := snap.KVState[k]

			if expected.Deleted {
				if exists {
					matchedSubsequent := false
					for _, op := range dc.subsequentOps[k] {
						if (op.Op == harness.OpPut || op.Op == harness.OpCAS) && op.Value == actualVal {
							matchedSubsequent = true
							break
						}
					}
					if !matchedSubsequent {
						return &Violation{
							Checker: "durability",
							Detail: fmt.Sprintf("node %s retained deleted key %q (val=%q) after confirmed deletion (req=%d at %s)",
								snap.ID, k, actualVal, expected.RequestID, expected.At),
						}
					}
				}
			} else {
				if !exists {
					matchedSubsequent := false
					for _, op := range dc.subsequentOps[k] {
						if op.Op == harness.OpDelete {
							matchedSubsequent = true
							break
						}
					}
					if !matchedSubsequent {
						return &Violation{
							Checker: "durability",
							Detail: fmt.Sprintf("node %s lost confirmed write for key %q (expected=%q) from req %d at %s",
								snap.ID, k, expected.Value, expected.RequestID, expected.At),
						}
					}
				} else if actualVal != expected.Value {
					matchedSubsequent := false
					for _, op := range dc.subsequentOps[k] {
						if (op.Op == harness.OpPut || op.Op == harness.OpCAS) && op.Value == actualVal {
							matchedSubsequent = true
							break
						}
					}
					if !matchedSubsequent {
						return &Violation{
							Checker: "durability",
							Detail: fmt.Sprintf("node %s has wrong value for key %q (got %q, expected %q) confirmed in req %d at %s",
								snap.ID, k, actualVal, expected.Value, expected.RequestID, expected.At),
						}
					}
				}
			}
		}
	}

	return nil
}
