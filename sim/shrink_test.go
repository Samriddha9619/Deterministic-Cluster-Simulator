package sim

import (
	"testing"

	"github.com/Samriddha9619/deterministic-cluster-simulator/harness"
	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

func TestShrinkerBaselinePassing(tst *testing.T) {
	input := ShrinkInput{
		Seed:       1,
		NumNodes:   3,
		MaxTimeSec: 10,
		FaultPlan:  NewFaultPlan(),
	}

	shrinker := NewShrinker(input)
	_, err := shrinker.Shrink()
	if err == nil {
		tst.Fatalf("expected error when shrinking baseline passing run")
	}
}

func TestShrinkerMinimizesFaultsAndWorkload(tst *testing.T) {
	seed := uint64(1)
	nodeIDs := []t.NodeID{1, 2, 3}

	// Generate a bloated fault plan with 10 faults
	faultRng := NewRand(seed).Fork("faults")
	bloatedPlan := GenerateRandomFaultPlan(faultRng, nodeIDs, 15*t.Second, 10)

	// Generate a 25-op workload
	wcfg := harness.DefaultWorkloadConfig()
	wcfg.NumOps = 25
	wcfg.StartTime = 2 * t.Second
	wcfg.EndTime = 12 * t.Second
	workloadRng := NewRand(seed).Fork("workload")
	bloatedWorkload := harness.GenerateWorkload(workloadRng, nodeIDs, wcfg)

	input := ShrinkInput{
		Seed:       seed,
		NumNodes:   3,
		MaxTimeSec: 15,
		FaultPlan:  bloatedPlan,
		Workload:   bloatedWorkload,
		Bugs: t.BugFlags{
			AckBeforeFsync: true,
		},
		TargetChecker: "durability",
	}

	shrinker := NewShrinker(input)
	res, err := shrinker.Shrink()
	if err != nil {
		tst.Fatalf("shrinking failed: %v", err)
	}

	tst.Logf("Shrunk: %s", res.Summary)
	tst.Logf("Faults: %d -> %d", res.OriginalFaults, res.ShrunkFaults)
	tst.Logf("Ops: %d -> %d", res.OriginalOps, res.ShrunkOps)
	tst.Logf("Iterations: %d", res.Iterations)
	tst.Logf("Final violation: %s", res.FinalViolation)

	if res.ShrunkFaults >= res.OriginalFaults {
		tst.Errorf("expected fault count to shrink: %d >= %d", res.ShrunkFaults, res.OriginalFaults)
	}
	if res.FinalViolation == "" {
		tst.Errorf("expected final minimal run to retain violation")
	}
	if res.FinalTrace == nil || res.FinalTrace.Verdict.Passed {
		tst.Errorf("expected final trace to have failing verdict")
	}
}
