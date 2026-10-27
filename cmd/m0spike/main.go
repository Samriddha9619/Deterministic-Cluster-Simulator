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
	withPuts := flag.Int("puts", 50, "number of put operations to submit per seed")
	withPartition := flag.Bool("partition", true, "inject a network partition mid-run")
	autoFaults := flag.Int("faults", 0, "number of random upfront faults (crashes, restarts, partitions) to generate")
	withWorkload := flag.Bool("workload", true, "generate mixed concurrent client workload (Put, Get, Delete, CAS)")
	flag.Parse()

	passed := 0
	failed := 0
	detFail := 0
	wallStart := time.Now()

	for i := 0; i < *seedCount; i++ {
		seed := *startSeed + uint64(i)

		result1 := runOneSeed(seed, *withPuts, *withPartition, *autoFaults, *withWorkload)

		if result1.Violation != "" {
			failed++
			fmt.Printf("FAIL  seed=%d violation=%s\n", seed, result1.Violation)
			continue
		}

		if *detCheck {
			// Run the same seed again and compare trace hashes.
			result2 := runOneSeed(seed, *withPuts, *withPartition, *autoFaults, *withWorkload)
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

// runOneSeed creates a simulator, injects workload and faults, and runs it.
func runOneSeed(seed uint64, numPuts int, withPartition bool, autoFaults int, withWorkload bool) sim.RunResult {
	cfg := sim.DefaultSimConfig(seed)
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
