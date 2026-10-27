package checkers

import (
	"fmt"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// CheckLogMatching verifies the Raft Log Matching Property:
// If two logs contain an entry with the same index and term, then the logs
// are identical in all entries up through the given index.
func CheckLogMatching(snapshots []t.NodeView) *Violation {
	// Compare every pair of live nodes
	for i := 0; i < len(snapshots); i++ {
		for j := i + 1; j < len(snapshots); j++ {
			nodeA := snapshots[i]
			nodeB := snapshots[j]
			if !nodeA.Alive || !nodeB.Alive {
				continue
			}

			// Map index -> term for node A
			termsA := make(map[uint64]uint64, len(nodeA.LogEntries))
			for _, e := range nodeA.LogEntries {
				termsA[e.Index] = e.Term
			}

			// Map index -> term for node B
			termsB := make(map[uint64]uint64, len(nodeB.LogEntries))
			for _, e := range nodeB.LogEntries {
				termsB[e.Index] = e.Term
			}

			// For any shared index where terms match, all previous indices must also match
			for idx, termA := range termsA {
				termB, hasB := termsB[idx]
				if hasB && termA == termB {
					// Check all indices up through idx
					for prevIdx := uint64(1); prevIdx < idx; prevIdx++ {
						pA, inA := termsA[prevIdx]
						pB, inB := termsB[prevIdx]
						if inA && inB && pA != pB {
							return &Violation{
								Checker: "log_matching",
								Detail: fmt.Sprintf("nodes %s and %s share entry (idx=%d, term=%d) but diverge at idx=%d (term %d vs %d)",
									nodeA.ID, nodeB.ID, idx, termA, prevIdx, pA, pB),
							}
						}
					}
				}
			}
		}
	}
	return nil
}

// CheckLeaderCompleteness verifies that if a log entry is committed at index i in term T,
// that entry is present in the logs of the leaders for all higher terms T' > T.
func CheckLeaderCompleteness(committedIndex uint64, committedTerm uint64, committedEntryTerm uint64, snapshots []t.NodeView) *Violation {
	if committedIndex == 0 {
		return nil
	}

	for _, snap := range snapshots {
		if !snap.Alive || snap.Role != t.RoleLeader {
			continue
		}
		if snap.Term > committedTerm {
			// Find entry at committedIndex in leader's log
			found := false
			for _, e := range snap.LogEntries {
				if e.Index == committedIndex {
					found = true
					if e.Term != committedEntryTerm {
						return &Violation{
							Checker: "leader_completeness",
							Detail: fmt.Sprintf("leader %s (term %d) has entry at committed idx %d with term %d, expected term %d",
								snap.ID, snap.Term, committedIndex, e.Term, committedEntryTerm),
						}
					}
					break
				}
			}
			if !found && snap.LogLength >= committedIndex {
				return &Violation{
					Checker: "leader_completeness",
					Detail: fmt.Sprintf("leader %s (term %d) missing committed entry at idx %d",
						snap.ID, snap.Term, committedIndex),
				}
			}
		}
	}
	return nil
}
