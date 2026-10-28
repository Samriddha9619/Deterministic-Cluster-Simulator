package sim

import (
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/Samriddha9619/deterministic-cluster-simulator/harness"
	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// SweepConfig configures a batch simulation run across multiple seeds.
type SweepConfig struct {
	StartSeed        uint64     `json:"start_seed"`
	Count            int        `json:"count"`
	Concurrency      int        `json:"concurrency"` // worker count (0 = runtime.NumCPU())
	NumNodes         int        `json:"num_nodes"`
	DropRate         float64    `json:"drop_rate"`
	Faults           int        `json:"faults"`
	Workload         bool       `json:"workload"`
	NumOps           int        `json:"num_ops"`
	CheckDeterminism bool       `json:"check_determinism"`
	MaxTimeSec       int        `json:"max_time_sec"`
	Bugs             t.BugFlags `json:"bugs,omitempty"`
}

// DefaultSweepConfig returns sensible defaults for cluster sweeping.
func DefaultSweepConfig() SweepConfig {
	return SweepConfig{
		StartSeed:        1,
		Count:            20,
		Concurrency:      runtime.NumCPU(),
		NumNodes:         3,
		DropRate:         0.05,
		Faults:           3,
		Workload:         true,
		NumOps:           40,
		CheckDeterminism: true,
		MaxTimeSec:       25,
	}
}

// SeedSummary records the outcome of a single seed execution.
type SeedSummary struct {
	Seed      uint64 `json:"seed"`
	Passed    bool   `json:"passed"`
	Events    int    `json:"events"`
	Duration  string `json:"duration"`
	Violation string `json:"violation,omitempty"`
	TraceHash string `json:"trace_hash"`
	DetFailed bool   `json:"det_failed,omitempty"`
}

// SweepReport aggregates multi-seed statistics and reports failures for shrinking.
type SweepReport struct {
	Total        int           `json:"total"`
	Passed       int           `json:"passed"`
	Failed       int           `json:"failed"`
	DetFailed    int           `json:"det_failed"`
	WallDuration time.Duration `json:"wall_duration"`
	WallDurMs    int64         `json:"wall_duration_ms"`
	SeedsPerSec  float64       `json:"seeds_per_sec"`
	Results      []SeedSummary `json:"results"`
	Failures     []SeedSummary `json:"failures"`
}

// RunSweep executes multiple seeds in parallel using isolated deterministic simulators.
func RunSweep(cfg SweepConfig) SweepReport {
	if cfg.Count <= 0 {
		cfg.Count = 10
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = runtime.NumCPU()
	}
	if cfg.NumNodes <= 0 {
		cfg.NumNodes = 3
	}
	if cfg.MaxTimeSec <= 0 {
		cfg.MaxTimeSec = 25
	}
	if cfg.NumOps <= 0 {
		cfg.NumOps = 40
	}

	start := time.Now()
	results := make([]SeedSummary, cfg.Count)
	jobs := make(chan int, cfg.Count)

	for i := 0; i < cfg.Count; i++ {
		jobs <- i
	}
	close(jobs)

	var wg sync.WaitGroup
	workers := cfg.Concurrency
	if workers > cfg.Count {
		workers = cfg.Count
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				seed := cfg.StartSeed + uint64(idx)
				results[idx] = runSingleSeed(seed, cfg)
			}
		}()
	}

	wg.Wait()
	wallDur := time.Since(start)

	var failures []SeedSummary
	passed := 0
	failed := 0
	detFailed := 0

	for _, r := range results {
		if r.DetFailed {
			detFailed++
		}
		if r.Passed && !r.DetFailed {
			passed++
		} else {
			failed++
			failures = append(failures, r)
		}
	}

	// Sort failures by seed
	sort.Slice(failures, func(i, j int) bool {
		return failures[i].Seed < failures[j].Seed
	})

	sec := wallDur.Seconds()
	sps := 0.0
	if sec > 0 {
		sps = float64(cfg.Count) / sec
	}

	return SweepReport{
		Total:        cfg.Count,
		Passed:       passed,
		Failed:       failed,
		DetFailed:    detFailed,
		WallDuration: wallDur,
		WallDurMs:    wallDur.Milliseconds(),
		SeedsPerSec:  sps,
		Results:      results,
		Failures:     failures,
	}
}

// runSingleSeed runs one simulation seed (and optionally a second run for determinism verification).
func runSingleSeed(seed uint64, cfg SweepConfig) SeedSummary {
	res1 := executeSimulation(seed, cfg)

	summary := SeedSummary{
		Seed:      seed,
		Passed:    res1.Violation == "",
		Events:    res1.EventCount,
		Duration:  res1.FinalTimeStr,
		Violation: res1.Violation,
		TraceHash: res1.TraceHash,
	}

	if cfg.CheckDeterminism {
		res2 := executeSimulation(seed, cfg)
		if res1.TraceHash != res2.TraceHash {
			summary.DetFailed = true
			if summary.Violation == "" {
				summary.Violation = fmt.Sprintf("determinism mismatch: %s != %s", res1.TraceHash, res2.TraceHash)
			}
		}
	}

	return summary
}

// executeSimulation builds and runs one independent simulator instance.
func executeSimulation(seed uint64, cfg SweepConfig) RunResult {
	simCfg := DefaultSimConfig(seed)
	simCfg.NumNodes = cfg.NumNodes
	simCfg.MaxTime = t.Time(cfg.MaxTimeSec) * t.Second
	simCfg.AutoFaults = cfg.Faults
	simCfg.Bugs = cfg.Bugs

	linkCfg := DefaultLinkConfig()
	linkCfg.DropRate = cfg.DropRate
	simCfg.LinkConfig = &linkCfg

	s, err := NewSimulator(simCfg)
	if err != nil {
		return RunResult{
			Seed:      seed,
			Violation: fmt.Sprintf("simulator init: %v", err),
		}
	}

	nodeIDs := make([]t.NodeID, cfg.NumNodes)
	for i := 0; i < cfg.NumNodes; i++ {
		nodeIDs[i] = t.NodeID(i + 1)
	}

	// Workload or simple put sequence
	if cfg.Workload {
		wcfg := harness.DefaultWorkloadConfig()
		wcfg.NumOps = cfg.NumOps
		wcfg.StartTime = 2 * t.Second
		wcfg.EndTime = t.Time(cfg.MaxTimeSec-3) * t.Second
		wRng := NewRand(seed).Fork("workload")
		reqs := harness.GenerateWorkload(wRng, nodeIDs, wcfg)
		for _, r := range reqs {
			s.EnqueueClientRequest(r.At, r.TargetNode, r.Req)
		}
	} else if cfg.NumOps > 0 {
		for i := 0; i < cfg.NumOps; i++ {
			putTime := 2*t.Second + t.Time(i)*t.Time(16*t.Second/t.Time(cfg.NumOps+1))
			s.EnqueueClientRequest(putTime, t.NodeID(1), t.ClientReq{
				RequestID: uint64(i + 1),
				ClientID:  1,
				Op:        t.OpPut,
				Key:       fmt.Sprintf("k%02d", i%10),
				Value:     fmt.Sprintf("v%03d", i),
			})
		}
	}

	return s.Run()
}
