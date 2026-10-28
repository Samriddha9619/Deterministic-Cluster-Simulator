package sim

import (
	"fmt"
	"strings"

	"github.com/Samriddha9619/deterministic-cluster-simulator/checkers"
	"github.com/Samriddha9619/deterministic-cluster-simulator/harness"
	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// ShrinkInput specifies the configuration and initial failing conditions to shrink.
type ShrinkInput struct {
	Seed          uint64                       `json:"seed"`
	NumNodes      int                          `json:"num_nodes"`
	MaxTimeSec    int                          `json:"max_time_sec"`
	DropRate      float64                      `json:"drop_rate"`
	FaultPlan     *FaultPlan                   `json:"fault_plan"`
	Workload      []harness.ScheduledClientReq `json:"workload"`
	Bugs          t.BugFlags                   `json:"bugs"`
	TargetChecker string                       `json:"target_checker,omitempty"`
}

// ShrinkResult details the outcome of delta-debugging minimization.
type ShrinkResult struct {
	Seed             uint64                       `json:"seed"`
	OriginalFaults   int                          `json:"original_faults"`
	ShrunkFaults     int                          `json:"shrunk_faults"`
	OriginalOps      int                          `json:"original_ops"`
	ShrunkOps        int                          `json:"shrunk_ops"`
	Iterations       int                          `json:"iterations"`
	OriginalPlan     *FaultPlan                   `json:"original_plan"`
	ShrunkPlan       *FaultPlan                   `json:"shrunk_plan"`
	ShrunkWorkload   []harness.ScheduledClientReq `json:"shrunk_workload"`
	InitialViolation string                       `json:"initial_violation"`
	FinalViolation   string                       `json:"final_violation"`
	Summary          string                       `json:"summary"`
	FinalTrace       *harness.RunTrace            `json:"final_trace,omitempty"`
}

// Shrinker implements Andreas Zeller's delta-debugging algorithm (ddmin)
// for minimizing fault plans and client workloads.
type Shrinker struct {
	input      ShrinkInput
	iterations int
}

// NewShrinker initializes a new shrinker instance.
func NewShrinker(input ShrinkInput) *Shrinker {
	if input.NumNodes <= 0 {
		input.NumNodes = 3
	}
	if input.MaxTimeSec <= 0 {
		input.MaxTimeSec = 25
	}
	return &Shrinker{input: input}
}

// Shrink executes delta-debugging on the fault plan, then on the workload operations.
func (s *Shrinker) Shrink() (*ShrinkResult, error) {
	// Baseline verification: ensure the run actually fails initially.
	initialRes := s.eval(s.input.FaultPlan, s.input.Workload)
	if initialRes.Violation == "" {
		return nil, fmt.Errorf("baseline run passed with no violation; nothing to shrink")
	}

	initialViolation := initialRes.Violation
	origFaultCount := 0
	if s.input.FaultPlan != nil {
		origFaultCount = len(s.input.FaultPlan.Actions)
	}
	origOpsCount := len(s.input.Workload)

	currentPlan := s.input.FaultPlan
	if currentPlan == nil {
		currentPlan = NewFaultPlan()
	}
	currentOps := s.input.Workload

	// Phase 1: ddmin on FaultPlan actions
	currentPlan = s.shrinkFaultPlan(currentPlan, currentOps, initialViolation)

	// Phase 2: ddmin on Workload operations
	if len(currentOps) > 0 {
		currentOps = s.shrinkWorkload(currentPlan, currentOps, initialViolation)
	}

	// Re-evaluate final minimal state
	finalRes := s.eval(currentPlan, currentOps)

	// Generate visual trace for UI inspection
	finalTrace := s.buildFinalTrace(currentPlan, currentOps, finalRes)

	shrunkSummary := fmt.Sprintf("%s; %d client ops (shrunk from %d faults, %d ops in %d iterations)",
		currentPlan.HumanSummary(), len(currentOps), origFaultCount, origOpsCount, s.iterations)

	return &ShrinkResult{
		Seed:             s.input.Seed,
		OriginalFaults:   origFaultCount,
		ShrunkFaults:     len(currentPlan.Actions),
		OriginalOps:      origOpsCount,
		ShrunkOps:        len(currentOps),
		Iterations:       s.iterations,
		OriginalPlan:     s.input.FaultPlan,
		ShrunkPlan:       currentPlan,
		ShrunkWorkload:   currentOps,
		InitialViolation: initialViolation,
		FinalViolation:   finalRes.Violation,
		Summary:          shrunkSummary,
		FinalTrace:       finalTrace,
	}, nil
}

// shrinkFaultPlan runs ddmin on fault actions.
func (s *Shrinker) shrinkFaultPlan(plan *FaultPlan, ops []harness.ScheduledClientReq, targetViolation string) *FaultPlan {
	if len(plan.Actions) <= 1 {
		return plan
	}

	actions := make([]FaultAction, len(plan.Actions))
	copy(actions, plan.Actions)

	n := 2
	for len(actions) > 0 && n <= len(actions) {
		chunks := splitActionChunks(actions, n)
		reduced := false

		// 1. Try removing each chunk (complements)
		for i := 0; i < len(chunks); i++ {
			candidate := removeActionChunk(chunks, i)
			candidatePlan := &FaultPlan{Actions: candidate}
			s.iterations++

			res := s.eval(candidatePlan, ops)
			if s.matchesFailure(res.Violation, targetViolation) {
				actions = candidate
				n = max(n-1, 2)
				reduced = true
				break
			}
		}
		if reduced {
			continue
		}

		// 2. Try keeping only each chunk (subsets)
		for i := 0; i < len(chunks); i++ {
			candidate := chunks[i]
			candidatePlan := &FaultPlan{Actions: candidate}
			s.iterations++

			res := s.eval(candidatePlan, ops)
			if s.matchesFailure(res.Violation, targetViolation) {
				actions = candidate
				n = 2
				reduced = true
				break
			}
		}
		if reduced {
			continue
		}

		// 3. Increase granularity if cannot reduce further at n
		if n < len(actions) {
			n = min(n*2, len(actions))
		} else {
			break
		}
	}

	return &FaultPlan{Actions: actions}
}

// shrinkWorkload runs ddmin on client workload requests.
func (s *Shrinker) shrinkWorkload(plan *FaultPlan, ops []harness.ScheduledClientReq, targetViolation string) []harness.ScheduledClientReq {
	if len(ops) <= 1 {
		return ops
	}

	current := make([]harness.ScheduledClientReq, len(ops))
	copy(current, ops)

	n := 2
	for len(current) > 0 && n <= len(current) {
		chunks := splitReqChunks(current, n)
		reduced := false

		// 1. Try removing each chunk (complements)
		for i := 0; i < len(chunks); i++ {
			candidate := removeReqChunk(chunks, i)
			s.iterations++

			res := s.eval(plan, candidate)
			if s.matchesFailure(res.Violation, targetViolation) {
				current = candidate
				n = max(n-1, 2)
				reduced = true
				break
			}
		}
		if reduced {
			continue
		}

		// 2. Try keeping each chunk (subsets)
		for i := 0; i < len(chunks); i++ {
			candidate := chunks[i]
			s.iterations++

			res := s.eval(plan, candidate)
			if s.matchesFailure(res.Violation, targetViolation) {
				current = candidate
				n = 2
				reduced = true
				break
			}
		}
		if reduced {
			continue
		}

		// 3. Increase granularity
		if n < len(current) {
			n = min(n*2, len(current))
		} else {
			break
		}
	}

	return current
}

// eval executes the simulator with candidate faults and workload.
func (s *Shrinker) eval(plan *FaultPlan, ops []harness.ScheduledClientReq) RunResult {
	cfg := DefaultSimConfig(s.input.Seed)
	cfg.NumNodes = s.input.NumNodes
	cfg.MaxTime = t.Time(s.input.MaxTimeSec) * t.Second
	cfg.Bugs = s.input.Bugs
	cfg.Faults = plan // explicit candidate plan

	linkCfg := DefaultLinkConfig()
	linkCfg.DropRate = s.input.DropRate
	cfg.LinkConfig = &linkCfg

	sim, err := NewSimulator(cfg)
	if err != nil {
		return RunResult{Violation: fmt.Sprintf("init: %v", err)}
	}

	for _, op := range ops {
		sim.EnqueueClientRequest(op.At, op.TargetNode, op.Req)
	}

	return sim.Run()
}

// matchesFailure checks if a candidate violation matches the expected failure type.
func (s *Shrinker) matchesFailure(actual, target string) bool {
	if actual == "" {
		return false
	}
	if s.input.TargetChecker != "" {
		return strings.Contains(strings.ToLower(actual), strings.ToLower(s.input.TargetChecker))
	}
	// If no explicit target checker, match identical checker tag (e.g. "[durability]" or "[convergence]")
	targetTag := extractCheckerTag(target)
	if targetTag != "" {
		return strings.Contains(actual, targetTag)
	}
	return true
}

func extractCheckerTag(violation string) string {
	start := strings.Index(violation, "[")
	end := strings.Index(violation, "]")
	if start != -1 && end != -1 && start < end {
		return violation[start : end+1]
	}
	return ""
}

func splitActionChunks(actions []FaultAction, n int) [][]FaultAction {
	chunks := make([][]FaultAction, n)
	total := len(actions)
	chunkSize := (total + n - 1) / n

	for i := 0; i < n; i++ {
		start := i * chunkSize
		if start >= total {
			break
		}
		end := start + chunkSize
		if end > total {
			end = total
		}
		chunks[i] = actions[start:end]
	}
	var nonNil [][]FaultAction
	for _, c := range chunks {
		if len(c) > 0 {
			nonNil = append(nonNil, c)
		}
	}
	return nonNil
}

func removeActionChunk(chunks [][]FaultAction, skipIdx int) []FaultAction {
	var res []FaultAction
	for i, c := range chunks {
		if i != skipIdx {
			res = append(res, c...)
		}
	}
	return res
}

func splitReqChunks(reqs []harness.ScheduledClientReq, n int) [][]harness.ScheduledClientReq {
	chunks := make([][]harness.ScheduledClientReq, n)
	total := len(reqs)
	chunkSize := (total + n - 1) / n

	for i := 0; i < n; i++ {
		start := i * chunkSize
		if start >= total {
			break
		}
		end := start + chunkSize
		if end > total {
			end = total
		}
		chunks[i] = reqs[start:end]
	}
	var nonNil [][]harness.ScheduledClientReq
	for _, c := range chunks {
		if len(c) > 0 {
			nonNil = append(nonNil, c)
		}
	}
	return nonNil
}

func removeReqChunk(chunks [][]harness.ScheduledClientReq, skipIdx int) []harness.ScheduledClientReq {
	var res []harness.ScheduledClientReq
	for i, c := range chunks {
		if i != skipIdx {
			res = append(res, c...)
		}
	}
	return res
}

func (s *Shrinker) buildFinalTrace(plan *FaultPlan, ops []harness.ScheduledClientReq, res RunResult) *harness.RunTrace {
	traceEvents := make([]harness.TraceEvent, len(res.Trace))
	for i, te := range res.Trace {
		var fromID, toID t.NodeID
		if te.From != "" {
			var f uint64
			fmt.Sscanf(te.From, "n%d", &f)
			fromID = t.NodeID(f)
		}
		if te.Target != "" {
			var to uint64
			fmt.Sscanf(te.Target, "n%d", &to)
			toID = t.NodeID(to)
		}

		traceEvents[i] = harness.TraceEvent{
			Index:     te.Index,
			At:        te.At,
			AtStr:     te.AtStr,
			Kind:      te.Kind,
			From:      fromID,
			Target:    toID,
			Detail:    te.Detail,
			Snapshots: te.Snapshots,
		}
	}

	verdict := harness.TraceVerdict{Passed: res.Violation == ""}
	if res.Violation != "" {
		tag := extractCheckerTag(res.Violation)
		if tag == "" {
			tag = "invariant"
		} else {
			tag = strings.Trim(tag, "[]")
		}
		verdict.Violation = &checkers.Violation{
			Checker: tag,
			Detail:  res.Violation,
		}
	}

	return &harness.RunTrace{
		Meta: harness.TraceMeta{
			Seed:         s.input.Seed,
			NumNodes:     s.input.NumNodes,
			MaxTimeNs:    int64(res.FinalTime),
			MaxTimeStr:   res.FinalTimeStr,
			DropRate:     s.input.DropRate,
			NumClientOps: len(ops),
			BugFlags:     s.input.Bugs.ActiveNames(),
		},
		Events:    traceEvents,
		Verdict:   verdict,
		TraceHash: res.TraceHash,
	}
}
