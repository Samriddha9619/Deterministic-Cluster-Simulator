package checkers

import (
	"errors"
	"testing"

	"github.com/Samriddha9619/deterministic-cluster-simulator/harness"
	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

func TestCheckOneLeaderPerTerm(tst *testing.T) {
	valid := []t.NodeView{
		{ID: t.NodeID(1), Role: t.RoleLeader, Term: 2, Alive: true},
		{ID: t.NodeID(2), Role: t.RoleFollower, Term: 2, Alive: true},
		{ID: t.NodeID(3), Role: t.RoleFollower, Term: 2, Alive: true},
	}
	if v := CheckOneLeaderPerTerm(valid); v != nil {
		tst.Fatalf("unexpected violation for valid cluster: %v", v)
	}

	splitBrain := []t.NodeView{
		{ID: t.NodeID(1), Role: t.RoleLeader, Term: 2, Alive: true},
		{ID: t.NodeID(2), Role: t.RoleLeader, Term: 2, Alive: true},
		{ID: t.NodeID(3), Role: t.RoleFollower, Term: 2, Alive: true},
	}
	if v := CheckOneLeaderPerTerm(splitBrain); v == nil {
		tst.Fatalf("expected violation for split brain, got nil")
	} else if v.Checker != "one_leader_per_term" {
		tst.Fatalf("expected one_leader_per_term checker, got %s", v.Checker)
	}
}

func TestCheckConvergence(tst *testing.T) {
	kv1 := map[string]string{"foo": "bar", "alpha": "beta"}
	kv2 := map[string]string{"foo": "bar", "alpha": "beta"}
	kvDivergent := map[string]string{"foo": "bar", "alpha": "gamma"}

	converged := []t.NodeView{
		{ID: t.NodeID(1), KVState: kv1, Alive: true},
		{ID: t.NodeID(2), KVState: kv2, Alive: true},
	}
	if v := CheckConvergence(converged); v != nil {
		tst.Fatalf("unexpected violation for converged cluster: %v", v)
	}

	diverged := []t.NodeView{
		{ID: t.NodeID(1), KVState: kv1, Alive: true},
		{ID: t.NodeID(2), KVState: kvDivergent, Alive: true},
	}
	if v := CheckConvergence(diverged); v == nil {
		tst.Fatalf("expected convergence violation, got nil")
	} else if v.Checker != "convergence" {
		tst.Fatalf("expected convergence checker, got %s", v.Checker)
	}
}

func TestOracle(tst *testing.T) {
	o := NewOracle()
	o.ApplyPut("k1", "v1")
	o.ApplyPut("k2", "v2")
	o.ApplyDelete("k1")

	matched := []t.NodeView{
		{ID: t.NodeID(1), KVState: map[string]string{"k2": "v2"}, Alive: true},
	}
	if v := o.CheckState(matched); v != nil {
		tst.Fatalf("unexpected oracle violation: %v", v)
	}

	mismatched := []t.NodeView{
		{ID: t.NodeID(1), KVState: map[string]string{"k1": "v1", "k2": "v2"}, Alive: true},
	}
	if v := o.CheckState(mismatched); v == nil {
		tst.Fatalf("expected oracle violation, got nil")
	}
}

func TestCheckLogMatching(tst *testing.T) {
	// Valid matching logs
	matching := []t.NodeView{
		{
			ID:    t.NodeID(1),
			Alive: true,
			LogEntries: []t.LogEntrySummary{
				{Index: 1, Term: 1},
				{Index: 2, Term: 1},
				{Index: 3, Term: 2},
			},
		},
		{
			ID:    t.NodeID(2),
			Alive: true,
			LogEntries: []t.LogEntrySummary{
				{Index: 1, Term: 1},
				{Index: 2, Term: 1},
				{Index: 3, Term: 2},
			},
		},
	}
	if v := CheckLogMatching(matching); v != nil {
		tst.Fatalf("unexpected log matching violation: %v", v)
	}

	// Divergent logs at index 2 despite matching at index 3
	divergent := []t.NodeView{
		{
			ID:    t.NodeID(1),
			Alive: true,
			LogEntries: []t.LogEntrySummary{
				{Index: 1, Term: 1},
				{Index: 2, Term: 1},
				{Index: 3, Term: 2},
			},
		},
		{
			ID:    t.NodeID(2),
			Alive: true,
			LogEntries: []t.LogEntrySummary{
				{Index: 1, Term: 1},
				{Index: 2, Term: 99}, // diverged term
				{Index: 3, Term: 2},
			},
		},
	}
	if v := CheckLogMatching(divergent); v == nil {
		tst.Fatalf("expected log matching violation for diverged prefix")
	}
}

func TestCheckLeaderCompleteness(tst *testing.T) {
	leaders := []t.NodeView{
		{
			ID:    t.NodeID(1),
			Role:  t.RoleLeader,
			Term:  3,
			Alive: true,
			LogEntries: []t.LogEntrySummary{
				{Index: 5, Term: 2},
			},
		},
	}

	// Term 3 leader contains entry from term 2 at index 5 -> valid
	if v := CheckLeaderCompleteness(5, 2, 2, leaders); v != nil {
		tst.Fatalf("unexpected leader completeness violation: %v", v)
	}

	// Missing committed entry -> violation
	leadersMissing := []t.NodeView{
		{
			ID:        t.NodeID(2),
			Role:      t.RoleLeader,
			Term:      4,
			Alive:     true,
			LogLength: 10,
			LogEntries: []t.LogEntrySummary{
				{Index: 5, Term: 1}, // wrong term
			},
		},
	}
	if v := CheckLeaderCompleteness(5, 2, 2, leadersMissing); v == nil {
		tst.Fatalf("expected leader completeness violation")
	}
}

func TestDurabilityChecker(tst *testing.T) {
	dc := NewDurabilityChecker()
	dc.RecordAck(harness.HistoryEntry{
		Kind:      harness.KindOK,
		Op:        harness.OpPut,
		Key:       "important",
		Value:     "safe",
		RequestID: 101,
		At:        5 * t.Second,
	})

	// Live node with durable state present -> valid
	liveOK := []t.NodeView{
		{ID: t.NodeID(1), Alive: true, KVState: map[string]string{"important": "safe"}},
	}
	if v := dc.CheckFinalState(liveOK); v != nil {
		tst.Fatalf("unexpected durability violation: %v", v)
	}

	// Live node missing durable state -> violation
	liveMissing := []t.NodeView{
		{ID: t.NodeID(1), Alive: true, KVState: map[string]string{}},
	}
	if v := dc.CheckFinalState(liveMissing); v == nil {
		tst.Fatalf("expected durability violation for lost confirmed write")
	}
}

func TestLinearizabilityChecker(tst *testing.T) {
	// Valid sequential history: Put(k, v1) -> Get(k)=v1 -> Put(k, v2) -> Get(k)=v2
	validHistory := []harness.HistoryEntry{
		{Kind: harness.KindInvoke, RequestID: 1, ClientID: 1, Op: harness.OpPut, Key: "k", Value: "v1", At: 1 * t.Second},
		{Kind: harness.KindOK, RequestID: 1, ClientID: 1, Op: harness.OpPut, Key: "k", Value: "v1", At: 2 * t.Second},

		{Kind: harness.KindInvoke, RequestID: 2, ClientID: 2, Op: harness.OpGet, Key: "k", At: 3 * t.Second},
		{Kind: harness.KindOK, RequestID: 2, ClientID: 2, Op: harness.OpGet, Key: "k", Value: "v1", At: 4 * t.Second},

		{Kind: harness.KindInvoke, RequestID: 3, ClientID: 1, Op: harness.OpPut, Key: "k", Value: "v2", At: 5 * t.Second},
		{Kind: harness.KindOK, RequestID: 3, ClientID: 1, Op: harness.OpPut, Key: "k", Value: "v2", At: 6 * t.Second},

		{Kind: harness.KindInvoke, RequestID: 4, ClientID: 2, Op: harness.OpGet, Key: "k", At: 7 * t.Second},
		{Kind: harness.KindOK, RequestID: 4, ClientID: 2, Op: harness.OpGet, Key: "k", Value: "v2", At: 8 * t.Second},
	}
	if v := CheckLinearizability(validHistory); v != nil {
		tst.Fatalf("unexpected linearizability violation on valid history: %v", v)
	}

	// Stale read violation: After Put(k, v2) completes at 6s, a Get invoked at 7s returns stale "v1"
	staleHistory := []harness.HistoryEntry{
		{Kind: harness.KindInvoke, RequestID: 1, ClientID: 1, Op: harness.OpPut, Key: "k", Value: "v1", At: 1 * t.Second},
		{Kind: harness.KindOK, RequestID: 1, ClientID: 1, Op: harness.OpPut, Key: "k", Value: "v1", At: 2 * t.Second},

		{Kind: harness.KindInvoke, RequestID: 2, ClientID: 1, Op: harness.OpPut, Key: "k", Value: "v2", At: 3 * t.Second},
		{Kind: harness.KindOK, RequestID: 2, ClientID: 1, Op: harness.OpPut, Key: "k", Value: "v2", At: 4 * t.Second},

		// Read starts AFTER Put(v2) completed, but reads stale "v1"
		{Kind: harness.KindInvoke, RequestID: 3, ClientID: 2, Op: harness.OpGet, Key: "k", At: 5 * t.Second},
		{Kind: harness.KindOK, RequestID: 3, ClientID: 2, Op: harness.OpGet, Key: "k", Value: "v1", At: 6 * t.Second},
	}
	if v := CheckLinearizability(staleHistory); v == nil {
		tst.Fatalf("expected linearizability violation for stale read")
	} else if v.Checker != "linearizability" {
		tst.Fatalf("expected linearizability checker, got %s", v.Checker)
	}
}

func TestCheckDeterminism(tst *testing.T) {
	// Identical runs
	mockFn := func(seed uint64) (string, error) {
		return "hash-abc-123", nil
	}
	if v := CheckDeterminism(42, mockFn); v != nil {
		tst.Fatalf("unexpected determinism violation: %v", v)
	}

	// Divergent runs
	callCount := 0
	divergentFn := func(seed uint64) (string, error) {
		callCount++
		if callCount == 1 {
			return "hash-run-1", nil
		}
		return "hash-run-2", nil
	}
	if v := CheckDeterminism(42, divergentFn); v == nil {
		tst.Fatalf("expected determinism violation for divergent hashes")
	}

	// Error run
	errFn := func(seed uint64) (string, error) {
		return "", errors.New("sim failed")
	}
	if v := CheckDeterminism(42, errFn); v == nil {
		tst.Fatalf("expected violation when runner returns error")
	}
}
