package sim

import (
	"encoding/json"
	"testing"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

func TestFaultPlanCloneAndSerialize(tst *testing.T) {
	fp := NewFaultPlan()
	fp.Add(FaultAction{
		At:     10 * t.Second,
		Kind:   FaultCrashNode,
		NodeID: t.NodeID(1),
	})
	fp.Add(FaultAction{
		At:     15 * t.Second,
		Kind:   FaultRestartNode,
		NodeID: t.NodeID(1),
	})
	fp.Add(FaultAction{
		At:       5 * t.Second,
		Kind:     FaultLinkCut,
		FromNode: t.NodeID(2),
		ToNode:   t.NodeID(3),
	})

	if len(fp.Actions) != 3 {
		tst.Fatalf("expected 3 actions, got %d", len(fp.Actions))
	}

	// Should be sorted by At: 5s, 10s, 15s
	if fp.Actions[0].Kind != FaultLinkCut {
		tst.Fatalf("expected first action to be LinkCut at 5s, got %v", fp.Actions[0])
	}
	if fp.Actions[1].Kind != FaultCrashNode {
		tst.Fatalf("expected second action to be CrashNode at 10s, got %v", fp.Actions[1])
	}

	cloned := fp.Clone()
	if len(cloned.Actions) != 3 {
		tst.Fatalf("cloned actions length mismatch")
	}

	withoutFirst := fp.WithoutAction(0)
	if len(withoutFirst.Actions) != 2 {
		tst.Fatalf("expected 2 actions after removing index 0, got %d", len(withoutFirst.Actions))
	}
	if withoutFirst.Actions[0].Kind != FaultCrashNode {
		tst.Fatalf("expected first action to be CrashNode, got %v", withoutFirst.Actions[0])
	}

	data, err := json.Marshal(fp)
	if err != nil {
		tst.Fatalf("failed to marshal fault plan: %v", err)
	}

	var decoded FaultPlan
	if err := json.Unmarshal(data, &decoded); err != nil {
		tst.Fatalf("failed to unmarshal fault plan: %v", err)
	}
	if len(decoded.Actions) != 3 {
		tst.Fatalf("decoded actions count mismatch: got %d", len(decoded.Actions))
	}
}

func TestBuggifyDeterminism(tst *testing.T) {
	r1 := NewRand(42)
	r2 := NewRand(42)
	rec := NewBuggifyRecorder()

	// Same seed must produce identical sequence of outcomes
	for i := 0; i < 100; i++ {
		b1 := Buggify(r1, BuggifySite("site-disk-delay"), 0.3, rec)
		b2 := Buggify(r2, BuggifySite("site-disk-delay"), 0.3, nil)
		if b1 != b2 {
			tst.Fatalf("iteration %d: buggify outcomes diverged for identical seed", i)
		}
	}

	// Prob 0 must always be false
	for i := 0; i < 50; i++ {
		if Buggify(r1, BuggifySite("site-never"), 0.0, rec) {
			tst.Fatalf("iteration %d: Buggify(prob=0) returned true", i)
		}
	}

	// Prob 1 must always be true
	for i := 0; i < 50; i++ {
		if !Buggify(r1, BuggifySite("site-always"), 1.0, rec) {
			tst.Fatalf("iteration %d: Buggify(prob=1) returned false", i)
		}
	}
}
