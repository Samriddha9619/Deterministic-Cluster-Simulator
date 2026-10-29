package bugs

import (
	"fmt"
	"time"

	"github.com/Samriddha9619/deterministic-cluster-simulator/harness"
	"github.com/Samriddha9619/deterministic-cluster-simulator/sim"
	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// PlantedBug defines metadata for a bug gallery entry.
type PlantedBug struct {
	ID                string     `json:"id"`
	Name              string     `json:"name"`
	Description       string     `json:"description"`
	ExpectedViolation string     `json:"expected_violation"`
	Flags             t.BugFlags `json:"flags"`
}

// AllBugs returns the catalog of all 6 planted bugs specified in the blueprint.
func AllBugs() []PlantedBug {
	return []PlantedBug{
		{
			ID:                "ack_before_fsync",
			Name:              "Ack Before Fsync",
			Description:       "Responds OK to client before log entries and hard state are flushed to disk",
			ExpectedViolation: "durability",
			Flags:             t.BugFlags{AckBeforeFsync: true},
		},
		{
			ID:                "wrong_quorum",
			Name:              "Wrong Quorum",
			Description:       "Counts a minority (1 node) as a quorum, leading to split-brain leaders during partition",
			ExpectedViolation: "one_leader_per_term",
			Flags:             t.BugFlags{WrongQuorum: true},
		},
		{
			ID:                "stale_term_accepted",
			Name:              "Stale Term Accepted",
			Description:       "Accepts messages from lower terms without rejection, causing log prefix divergence",
			ExpectedViolation: "log_matching",
			Flags:             t.BugFlags{StaleTermAccepted: true},
		},
		{
			ID:                "unpersisted_vote",
			Name:              "Vote Without Persistence",
			Description:       "Forgets votedFor across restart, permitting multiple leaders in the same term",
			ExpectedViolation: "one_leader_per_term",
			Flags:             t.BugFlags{UnpersistedVote: true},
		},
		{
			ID:                "early_apply",
			Name:              "Apply Before Commit",
			Description:       "Applies speculative entries directly to state machine before quorum commitment",
			ExpectedViolation: "convergence",
			Flags:             t.BugFlags{EarlyApply: true},
		},
		{
			ID:                "stale_read",
			Name:              "Stale Read",
			Description:       "Serves Get requests directly from local memory without consensus or ReadIndex lease",
			ExpectedViolation: "linearizability",
			Flags:             t.BugFlags{StaleRead: true},
		},
	}
}

// BenchmarkResult stores the measured seeds-to-failure and shrinking metrics for a bug.
type BenchmarkResult struct {
	BugID            string        `json:"bug_id"`
	BugName          string        `json:"bug_name"`
	ExpectedChecker  string        `json:"expected_checker"`
	PassedControl    bool          `json:"passed_control,omitempty"`
	SeedsToFailure   int           `json:"seeds_to_failure"`
	FirstFailingSeed uint64        `json:"first_failing_seed"`
	InitialViolation string        `json:"initial_violation"`
	FinalViolation   string        `json:"final_violation"`
	OriginalFaults   int           `json:"original_faults"`
	ShrunkFaults     int           `json:"shrunk_faults"`
	OriginalOps      int           `json:"original_ops"`
	ShrunkOps        int           `json:"shrunk_ops"`
	ShrunkSummary    string        `json:"shrunk_summary"`
	Iterations       int           `json:"iterations"`
	Duration         time.Duration `json:"duration"`
}

// RunBugBenchmark evaluates a planted bug across seeds, finds the first failure, and shrinks it.
func RunBugBenchmark(bug PlantedBug, maxSeeds int) (*BenchmarkResult, error) {
	start := time.Now()
	res := &BenchmarkResult{
		BugID:           bug.ID,
		BugName:         bug.Name,
		ExpectedChecker: bug.ExpectedViolation,
	}

	nodeIDs := []t.NodeID{1, 2, 3}
	var failingSeed uint64
	var initialPlan *sim.FaultPlan
	var initialOps []harness.ScheduledClientReq
	var initialViolation string
	seedsTested := 0

	for s := 1; s <= maxSeeds; s++ {
		seed := uint64(s)
		seedsTested++

		faultRng := sim.NewRand(seed).Fork("faults")
		plan := sim.GenerateRandomFaultPlan(faultRng, nodeIDs, 25*t.Second, 8)

		wcfg := harness.DefaultWorkloadConfig()
		wcfg.NumOps = 25
		wcfg.StartTime = 2 * t.Second
		wcfg.EndTime = 22 * t.Second
		workloadRng := sim.NewRand(seed).Fork("workload")
		ops := harness.GenerateWorkload(workloadRng, nodeIDs, wcfg)

		simCfg := sim.DefaultSimConfig(seed)
		simCfg.Bugs = bug.Flags
		simCfg.Faults = plan
		simulator, err := sim.NewSimulator(simCfg)
		if err != nil {
			return nil, err
		}

		for _, op := range ops {
			simulator.EnqueueClientRequest(op.At, op.TargetNode, op.Req)
		}

		runRes := simulator.Run()
		if runRes.Violation != "" {
			failingSeed = seed
			initialPlan = plan
			initialOps = ops
			initialViolation = runRes.Violation
			break
		}
	}

	res.SeedsToFailure = seedsTested
	res.FirstFailingSeed = failingSeed
	res.InitialViolation = initialViolation

	if failingSeed == 0 {
		res.Duration = time.Since(start)
		return res, fmt.Errorf("bug %q did not produce a violation within %d seeds", bug.Name, maxSeeds)
	}

	// Run delta-debugging shrinker
	shrinker := sim.NewShrinker(sim.ShrinkInput{
		Seed:          failingSeed,
		NumNodes:      3,
		MaxTimeSec:    25,
		FaultPlan:     initialPlan,
		Workload:      initialOps,
		Bugs:          bug.Flags,
		TargetChecker: bug.ExpectedViolation,
	})

	shrinkRes, err := shrinker.Shrink()
	if err != nil {
		res.Duration = time.Since(start)
		return res, fmt.Errorf("shrinking failed: %w", err)
	}

	res.OriginalFaults = shrinkRes.OriginalFaults
	res.ShrunkFaults = shrinkRes.ShrunkFaults
	res.OriginalOps = shrinkRes.OriginalOps
	res.ShrunkOps = shrinkRes.ShrunkOps
	res.FinalViolation = shrinkRes.FinalViolation
	res.ShrunkSummary = shrinkRes.Summary
	res.Iterations = shrinkRes.Iterations
	res.Duration = time.Since(start)

	return res, nil
}

// GenerateMarkdownTable formats benchmark results as a GitHub markdown table.
func GenerateMarkdownTable(results []*BenchmarkResult) string {
	out := "| Bug ID | Planted Bug | Target Checker | Seeds to Fail | Shrunk Faults | Shrunk Ops | Iterations | Time |\n"
	out += "|---|---|---|---|---|---|---|---|\n"

	for _, r := range results {
		faultsCol := fmt.Sprintf("%d → **%d**", r.OriginalFaults, r.ShrunkFaults)
		opsCol := fmt.Sprintf("%d → **%d**", r.OriginalOps, r.ShrunkOps)
		out += fmt.Sprintf("| `%s` | %s | `%s` | %d (seed %d) | %s | %s | %d | %s |\n",
			r.BugID, r.BugName, r.ExpectedChecker, r.SeedsToFailure, r.FirstFailingSeed,
			faultsCol, opsCol, r.Iterations, r.Duration.Round(time.Millisecond))
	}
	return out
}
