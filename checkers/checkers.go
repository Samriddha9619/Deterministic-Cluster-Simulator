package checkers

import (
	"fmt"
	"sort"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// Violation describes a safety or invariant failure found by a checker.
type Violation = t.Violation

// CheckOneLeaderPerTerm verifies that at most one node is leader for any given term.
func CheckOneLeaderPerTerm(snapshots []t.NodeView) *Violation {
	leaders := make(map[uint64]t.NodeID)
	for _, snap := range snapshots {
		if !snap.Alive || snap.Role != t.RoleLeader {
			continue
		}
		if existing, ok := leaders[snap.Term]; ok && existing != snap.ID {
			return &Violation{
				Checker: "one_leader_per_term",
				Detail:  fmt.Sprintf("term %d has multiple leaders: %s and %s", snap.Term, existing, snap.ID),
			}
		}
		leaders[snap.Term] = snap.ID
	}
	return nil
}

// CheckConvergence verifies that at the end of a run (quiescence),
// all live nodes have converged to the identical KV state.
func CheckConvergence(snapshots []t.NodeView) *Violation {
	var refNode t.NodeID
	var refKV map[string]string

	for _, snap := range snapshots {
		if !snap.Alive {
			continue
		}
		if refKV == nil {
			refNode = snap.ID
			refKV = snap.KVState
			continue
		}

		if diff := diffKV(refKV, snap.KVState); diff != "" {
			return &Violation{
				Checker: "convergence",
				Detail:  fmt.Sprintf("KV state mismatch between %s and %s: %s", refNode, snap.ID, diff),
			}
		}
	}
	return nil
}

// Oracle tracks the expected KV state derived from committed writes.
type Oracle struct {
	expected map[string]string
}

// NewOracle creates an empty Oracle state machine.
func NewOracle() *Oracle {
	return &Oracle{
		expected: make(map[string]string),
	}
}

// ApplyPut records an acknowledged Put in the Oracle.
func (o *Oracle) ApplyPut(key, value string) {
	o.expected[key] = value
}

// ApplyDelete records an acknowledged Delete in the Oracle.
func (o *Oracle) ApplyDelete(key string) {
	delete(o.expected, key)
}

// CheckState verifies that every live node's KV state matches the Oracle.
func (o *Oracle) CheckState(snapshots []t.NodeView) *Violation {
	for _, snap := range snapshots {
		if !snap.Alive {
			continue
		}
		if diff := diffKV(o.expected, snap.KVState); diff != "" {
			return &Violation{
				Checker: "oracle",
				Detail:  fmt.Sprintf("node %s KV state diverges from oracle: %s", snap.ID, diff),
			}
		}
	}
	return nil
}

// diffKV compares two key-value maps and returns a deterministic description
// of the differences, or "" if they are equal.
func diffKV(a, b map[string]string) string {
	allKeys := make(map[string]struct{})
	for k := range a {
		allKeys[k] = struct{}{}
	}
	for k := range b {
		allKeys[k] = struct{}{}
	}

	sortedKeys := make([]string, 0, len(allKeys))
	for k := range allKeys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	for _, k := range sortedKeys {
		va, hasA := a[k]
		vb, hasB := b[k]
		if hasA && !hasB {
			return fmt.Sprintf("key %q present in first (val=%q) but missing in second", k, va)
		}
		if !hasA && hasB {
			return fmt.Sprintf("key %q missing in first but present in second (val=%q)", k, vb)
		}
		if va != vb {
			return fmt.Sprintf("key %q value mismatch (%q vs %q)", k, va, vb)
		}
	}
	return ""
}
