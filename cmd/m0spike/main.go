// Command m0spike runs the M0 feasibility spike: a deterministic simulation
// of a 3-node Raft cluster with elections, message drops, a partition, and
// a basic invariant check (one leader per term).
//
// Usage:
//   go run ./cmd/m0spike                  # single seed
//   go run ./cmd/m0spike -seeds 100       # run 100 seeds
//   go run ./cmd/m0spike -seeds 100 -det  # determinism check (run each seed twice)
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/Samriddha9619/deterministic-cluster-simulator/harness"
	"github.com/Samriddha9619/deterministic-cluster-simulator/sim"
	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

func main() {
	seedCount := flag.Int("seeds", 1, "number of seeds to run")
	startSeed := flag.Uint64("start", 1, "starting seed value")
	detCheck := flag.Bool("det", false, "determinism check: run each seed twice and compare trace hashes")
	verbose := flag.Bool("v", false, "verbose output (print per-seed results)")
	withPuts := flag.Int("puts", 50, "number of operations to submit per seed")
	withPartition := flag.Bool("partition", true, "inject a network partition mid-run")
	autoFaults := flag.Int("faults", 0, "number of random upfront faults (crashes, restarts, partitions) to generate")
	withWorkload := flag.Bool("workload", true, "generate mixed concurrent client workload (Put, Get, Delete, CAS)")
	shrinkSeed := flag.Uint64("shrink", 0, "seed to shrink: runs delta-debugging to minimize faults and workload")
	bugFlag := flag.String("bug", "", "planted bug to activate (e.g. ack_fsync, early_apply, unpersisted_vote)")
	parallel := flag.Int("parallel", 0, "parallel worker count (0 = serial loop, >0 = parallel sweep)")
	flag.Parse()

	bugs := parseBugFlags(*bugFlag)

	// If shrink mode is requested, run delta-debugging shrinker on that seed.
	if *shrinkSeed > 0 {
		runShrinkerOnSeed(*shrinkSeed, *autoFaults, *withWorkload, *withPuts, bugs)
		return
	}

	// If parallel sweep is requested with > 1 seed
	if *parallel > 0 && *seedCount > 1 {
		runParallelSweep(*startSeed, *seedCount, *parallel, *autoFaults, *withWorkload, *withPuts, *detCheck, bugs)
		return
	}

	passed := 0
	failed := 0
	detFail := 0
	wallStart := time.Now()

	for i := 0; i < *seedCount; i++ {
		seed := *startSeed + uint64(i)

		result1 := runOneSeed(seed, *withPuts, *withPartition, *autoFaults, *withWorkload, bugs)

		if result1.Violation != "" {
			failed++
			fmt.Printf("FAIL  seed=%d violation=%s\n", seed, result1.Violation)
			continue
		}

		if *detCheck {
			result2 := runOneSeed(seed, *withPuts, *withPartition, *autoFaults, *withWorkload, bugs)
			if result1.TraceHash != result2.TraceHash {
				detFail++
				fmt.Printf("DET-FAIL  seed=%d hash1=%s hash2=%s\n",
					seed, result1.TraceHash[:16], result2.TraceHash[:16])
				continue
			}
		}

		passed++
		if *verbose {
			fmt.Printf("PASS  seed=%d events=%d time=%s nodes=",
				seed, result1.EventCount, result1.FinalTimeStr)
			for _, snap := range result1.NodeSnapshots {
				fmt.Printf("%s[%s,term=%d,keys=%d] ", snap.ID, snap.Role, snap.Term, len(snap.KVState))
			}
			fmt.Println()
		}
	}

	wallDur := time.Since(wallStart)

	fmt.Println()
	fmt.Println("═══════════════════════════════════════")
	fmt.Printf("  Seeds: %d  Passed: %d  Failed: %d", *seedCount, passed, failed)
	if *detCheck {
		fmt.Printf("  Det-Fail: %d", detFail)
	}
	fmt.Println()
	fmt.Printf("  Wall time: %s (%.0f seeds/sec)\n",
		wallDur.Round(time.Millisecond),
		float64(*seedCount)/wallDur.Seconds())
	fmt.Println("═══════════════════════════════════════")

	if failed > 0 || detFail > 0 {
		os.Exit(1)
	}
}

func parseBugFlags(name string) t.BugFlags {
	var b t.BugFlags
	switch name {
	case "ack_fsync", "ack_before_fsync":
		b.AckBeforeFsync = true
	case "early_apply":
		b.EarlyApply = true
	case "unpersisted_vote":
		b.UnpersistedVote = true
	}
	return b
}

func runShrinkerOnSeed(seed uint64, faults int, workload bool, numOps int, bugs t.BugFlags) {
	fmt.Printf("▶ Running delta-debugging shrinker on seed %d...\n", seed)
	nodeIDs := []t.NodeID{1, 2, 3}

	var plan *sim.FaultPlan
	if faults > 0 {
		faultRng := sim.NewRand(seed).Fork("faults")
		plan = sim.GenerateRandomFaultPlan(faultRng, nodeIDs, 25*t.Second, faults)
	} else {
		plan = sim.NewFaultPlan()
	}

	var ops []harness.ScheduledClientReq
	if workload {
		wcfg := harness.DefaultWorkloadConfig()
		wcfg.NumOps = numOps
		wcfg.StartTime = 2 * t.Second
		wcfg.EndTime = 22 * t.Second
		wRng := sim.NewRand(seed).Fork("workload")
		ops = harness.GenerateWorkload(wRng, nodeIDs, wcfg)
	}

	shrinker := sim.NewShrinker(sim.ShrinkInput{
		Seed:       seed,
		NumNodes:   3,
		MaxTimeSec: 25,
		FaultPlan:  plan,
		Workload:   ops,
		Bugs:       bugs,
	})

	res, err := shrinker.Shrink()
	if err != nil {
		fmt.Printf("Shrinking aborted: %v\n", err)
		return
	}

	fmt.Println("═══════════════════════════════════════")
	fmt.Printf("  Shrunk Seed: %d\n", seed)
	fmt.Printf("  Initial Violation: %s\n", res.InitialViolation)
	fmt.Printf("  Final Violation:   %s\n", res.FinalViolation)
	fmt.Printf("  Faults: %d → %d actions\n", res.OriginalFaults, res.ShrunkFaults)
	fmt.Printf("  Client Ops: %d → %d ops\n", res.OriginalOps, res.ShrunkOps)
	fmt.Printf("  Iterations: %d\n", res.Iterations)
	fmt.Printf("  Summary: %s\n", res.Summary)
	fmt.Println("───────────────────────────────────────")
	fmt.Println("  Minimal Fault Actions:")
	for i, a := range res.ShrunkPlan.Actions {
		fmt.Printf("    [%d] %s\n", i, a)
	}
	fmt.Println("═══════════════════════════════════════")
}

func runParallelSweep(start uint64, count int, workers int, faults int, workload bool, puts int, det bool, bugs t.BugFlags) {
	fmt.Printf("▶ Sweeping %d seeds using %d parallel workers...\n", count, workers)
	report := sim.RunSweep(sim.SweepConfig{
		StartSeed:        start,
		Count:            count,
		Concurrency:      workers,
		NumNodes:         3,
		DropRate:         0.05,
		Faults:           faults,
		Workload:         workload,
		NumOps:           puts,
		CheckDeterminism: det,
		MaxTimeSec:       25,
		Bugs:             bugs,
	})

	for _, f := range report.Failures {
		fmt.Printf("FAIL  seed=%d violation=%s\n", f.Seed, f.Violation)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════")
	fmt.Printf("  Seeds: %d  Passed: %d  Failed: %d", report.Total, report.Passed, report.Failed)
	if det {
		fmt.Printf("  Det-Fail: %d", report.DetFailed)
	}
	fmt.Println()
	fmt.Printf("  Wall time: %s (%.0f seeds/sec)\n",
		report.WallDuration.Round(time.Millisecond), report.SeedsPerSec)
	fmt.Println("═══════════════════════════════════════")

	if report.Failed > 0 || report.DetFailed > 0 {
		os.Exit(1)
	}
}

// runOneSeed creates a simulator, injects workload and faults, and runs it.
func runOneSeed(seed uint64, numPuts int, withPartition bool, autoFaults int, withWorkload bool, bugs t.BugFlags) sim.RunResult {
	cfg := sim.DefaultSimConfig(seed)
	cfg.Bugs = bugs
	if autoFaults > 0 {
		cfg.AutoFaults = autoFaults
	}

	// Add some drops to make things interesting.
	linkCfg := sim.DefaultLinkConfig()
	linkCfg.DropRate = 0.05 // 5% message drop rate
	cfg.LinkConfig = &linkCfg

	s, err := sim.NewSimulator(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create simulator: %v\n", err)
		os.Exit(2)
	}

	if withWorkload {
		wcfg := harness.DefaultWorkloadConfig()
		wcfg.NumOps = numPuts
		wcfg.StartTime = 2 * t.Second
		wcfg.EndTime = 22 * t.Second
		wRng := sim.NewRand(seed).Fork("workload")
		nodeIDs := []t.NodeID{t.NodeID(1), t.NodeID(2), t.NodeID(3)}
		reqs := harness.GenerateWorkload(wRng, nodeIDs, wcfg)
		for _, r := range reqs {
			s.EnqueueClientRequest(r.At, r.TargetNode, r.Req)
		}
	} else {
		for i := 0; i < numPuts; i++ {
			putTime := 2*t.Second + t.Time(i)*t.Time(18*t.Second/t.Time(numPuts+1))
			s.EnqueueClientRequest(putTime, t.NodeID(1), t.ClientReq{
				RequestID: uint64(i + 1),
				ClientID:  1,
				Op:        t.OpPut,
				Key:       fmt.Sprintf("key-%03d", i),
				Value:     fmt.Sprintf("val-%03d", i),
			})
		}
	}

	if autoFaults == 0 {
		if withPartition {
			// Partition node 3 away from nodes 1,2 at 8s, heal at 15s.
			s.EnqueuePartition(8*t.Second, [][]t.NodeID{
				{t.NodeID(1), t.NodeID(2)},
				{t.NodeID(3)},
			})
			s.EnqueueHeal(15 * t.Second)
		}

		// Crash node 2 at 5s, restart from durable state at 10s.
		s.EnqueueCrash(5*t.Second, t.NodeID(2))
		s.EnqueueRestart(10*t.Second, t.NodeID(2))
	}

	return s.Run()
}

// printJSON pretty-prints a value as JSON (for debugging).
func printJSON(v interface{}) {
	data, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(data))
}

// Suppress unused warning.
var _ = printJSON
