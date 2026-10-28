package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Samriddha9619/deterministic-cluster-simulator/checkers"
	"github.com/Samriddha9619/deterministic-cluster-simulator/harness"
	"github.com/Samriddha9619/deterministic-cluster-simulator/sim"
	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// Server provides HTTP API and static file serving for the trace visualizer.
type Server struct {
	mux       *http.ServeMux
	staticDir string
	mu        sync.RWMutex
}

// Config holds server configuration options.
type Config struct {
	Port      int
	StaticDir string
}

// NewServer initializes a new Server.
func NewServer(cfg Config) *Server {
	s := &Server{
		mux:       http.NewServeMux(),
		staticDir: cfg.StaticDir,
	}
	s.registerRoutes()
	return s
}

// Handler returns the underlying http.Handler.
func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) registerRoutes() {
	s.mux.HandleFunc("/api/health", s.handleHealth)
	s.mux.HandleFunc("/api/simulate", s.handleSimulate)
	s.mux.HandleFunc("/api/sweep", s.handleSweep)
	s.mux.HandleFunc("/api/shrink", s.handleShrink)

	// Static web UI
	if s.staticDir != "" {
		fs := http.FileServer(http.Dir(s.staticDir))
		s.mux.Handle("/", fs)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// SimulateRequest configures a simulation run.
type SimulateRequest struct {
	Seed          uint64  `json:"seed"`
	NumNodes      int     `json:"nodes"`
	DropRate      float64 `json:"drop_rate"`
	WithPartition bool    `json:"partition"`
	NumPuts       int     `json:"puts"`
	MaxTimeSec    int     `json:"max_time_sec"`
	Faults        int     `json:"faults"`
	Workload      bool    `json:"workload"`
}

func (s *Server) handleSimulate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	req := SimulateRequest{
		Seed:          1,
		NumNodes:      3,
		DropRate:      0.05,
		WithPartition: true,
		NumPuts:       40,
		MaxTimeSec:    25,
		Faults:        0,
		Workload:      true,
	}

	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}
	} else if r.Method == http.MethodGet {
		q := r.URL.Query()
		if sVal := q.Get("seed"); sVal != "" {
			if v, err := strconv.ParseUint(sVal, 10, 64); err == nil {
				req.Seed = v
			}
		}
		if nVal := q.Get("nodes"); nVal != "" {
			if v, err := strconv.Atoi(nVal); err == nil && v >= 1 && v <= 7 {
				req.NumNodes = v
			}
		}
		if pVal := q.Get("puts"); pVal != "" {
			if v, err := strconv.Atoi(pVal); err == nil {
				req.NumPuts = v
			}
		}
		if fVal := q.Get("faults"); fVal != "" {
			if v, err := strconv.Atoi(fVal); err == nil {
				req.Faults = v
			}
		}
		if wVal := q.Get("workload"); wVal != "" {
			req.Workload = (wVal == "true" || wVal == "1")
		}
	}

	trace := runSimulationTrace(req)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(trace); err != nil {
		http.Error(w, fmt.Sprintf("encode trace: %v", err), http.StatusInternalServerError)
	}
}

// SweepRequest holds parameters for sweeping multiple seeds.
type SweepRequest struct {
	StartSeed uint64  `json:"start_seed"`
	Count     int     `json:"count"`
	NumNodes  int     `json:"nodes"`
	DropRate  float64 `json:"drop_rate"`
	Partition bool    `json:"partition"`
	NumPuts   int     `json:"puts"`
	Faults    int     `json:"faults"`
	Workload  bool    `json:"workload"`
}

// SweepItem records the result of one seed in the sweep.
type SweepItem struct {
	Seed      uint64 `json:"seed"`
	Passed    bool   `json:"passed"`
	Events    int    `json:"events"`
	Duration  string `json:"duration"`
	Violation string `json:"violation,omitempty"`
	TraceHash string `json:"trace_hash"`
}

// SweepResponse returns all seeds in the sweep.
type SweepResponse struct {
	Total     int         `json:"total"`
	Passed    int         `json:"passed"`
	Failed    int         `json:"failed"`
	WallDurMs int64       `json:"wall_duration_ms"`
	Results   []SweepItem `json:"results"`
}

func (s *Server) handleSweep(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	req := SweepRequest{
		StartSeed: 1,
		Count:     20,
		NumNodes:  3,
		DropRate:  0.05,
		Partition: true,
		NumPuts:   30,
		Workload:  true,
		Faults:    3,
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		http.Error(w, fmt.Sprintf("invalid sweep request: %v", err), http.StatusBadRequest)
		return
	}

	if req.Count <= 0 {
		req.Count = 10
	}
	if req.Count > 500 {
		req.Count = 500
	}

	sweepCfg := sim.SweepConfig{
		StartSeed:        req.StartSeed,
		Count:            req.Count,
		NumNodes:         req.NumNodes,
		DropRate:         req.DropRate,
		Faults:           req.Faults,
		Workload:         req.Workload,
		NumOps:           req.NumPuts,
		CheckDeterminism: true,
		MaxTimeSec:       25,
	}

	report := sim.RunSweep(sweepCfg)

	resp := SweepResponse{
		Total:     report.Total,
		Passed:    report.Passed,
		Failed:    report.Failed,
		WallDurMs: report.WallDurMs,
		Results:   make([]SweepItem, len(report.Results)),
	}

	for i, r := range report.Results {
		resp.Results[i] = SweepItem{
			Seed:      r.Seed,
			Passed:    r.Passed,
			Events:    r.Events,
			Duration:  r.Duration,
			Violation: r.Violation,
			TraceHash: r.TraceHash,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ShrinkRequest holds parameters for delta-debugging shrinking.
type ShrinkRequest struct {
	Seed          uint64     `json:"seed"`
	NumNodes      int        `json:"nodes"`
	Faults        int        `json:"faults"`
	Workload      bool       `json:"workload"`
	NumOps        int        `json:"num_ops"`
	DropRate      float64    `json:"drop_rate"`
	MaxTimeSec    int        `json:"max_time_sec"`
	TargetChecker string     `json:"target_checker,omitempty"`
	Bugs          t.BugFlags `json:"bugs,omitempty"`
}

func (s *Server) handleShrink(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	var req ShrinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		http.Error(w, fmt.Sprintf("invalid shrink request: %v", err), http.StatusBadRequest)
		return
	}

	if req.NumNodes <= 0 {
		req.NumNodes = 3
	}
	if req.MaxTimeSec <= 0 {
		req.MaxTimeSec = 25
	}
	if req.NumOps <= 0 {
		req.NumOps = 40
	}

	nodeIDs := make([]t.NodeID, req.NumNodes)
	for i := 0; i < req.NumNodes; i++ {
		nodeIDs[i] = t.NodeID(i + 1)
	}

	var initialPlan *sim.FaultPlan
	if req.Faults > 0 {
		faultRng := sim.NewRand(req.Seed).Fork("faults")
		initialPlan = sim.GenerateRandomFaultPlan(faultRng, nodeIDs, t.Time(req.MaxTimeSec)*t.Second, req.Faults)
	} else {
		initialPlan = sim.NewFaultPlan()
	}

	var initialWorkload []harness.ScheduledClientReq
	if req.Workload {
		wcfg := harness.DefaultWorkloadConfig()
		wcfg.NumOps = req.NumOps
		wcfg.StartTime = 2 * t.Second
		wcfg.EndTime = t.Time(req.MaxTimeSec-3) * t.Second
		wRng := sim.NewRand(req.Seed).Fork("workload")
		initialWorkload = harness.GenerateWorkload(wRng, nodeIDs, wcfg)
	}

	shrinker := sim.NewShrinker(sim.ShrinkInput{
		Seed:          req.Seed,
		NumNodes:      req.NumNodes,
		MaxTimeSec:    req.MaxTimeSec,
		DropRate:      req.DropRate,
		FaultPlan:     initialPlan,
		Workload:      initialWorkload,
		Bugs:          req.Bugs,
		TargetChecker: req.TargetChecker,
	})

	result, err := shrinker.Shrink()
	if err != nil {
		http.Error(w, fmt.Sprintf("shrink error: %v", err), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func runSimulationTrace(req SimulateRequest) *harness.RunTrace {
	cfg := sim.DefaultSimConfig(req.Seed)
	if req.NumNodes > 0 {
		cfg.NumNodes = req.NumNodes
	}
	if req.MaxTimeSec > 0 {
		cfg.MaxTime = t.Time(req.MaxTimeSec) * t.Second
	}

	linkCfg := sim.DefaultLinkConfig()
	linkCfg.DropRate = req.DropRate
	cfg.LinkConfig = &linkCfg

	if req.Faults > 0 {
		cfg.AutoFaults = req.Faults
	}

	s, err := sim.NewSimulator(cfg)
	if err != nil {
		return &harness.RunTrace{
			Verdict: harness.TraceVerdict{
				Passed: false,
				Violation: &checkers.Violation{
					Checker: "simulator_init",
					Detail:  err.Error(),
				},
			},
		}
	}

	// Schedule operations across the run
	if req.Workload {
		wcfg := harness.DefaultWorkloadConfig()
		if req.NumPuts > 0 {
			wcfg.NumOps = req.NumPuts
		}
		wcfg.StartTime = 2 * t.Second
		wcfg.EndTime = 22 * t.Second
		wRng := sim.NewRand(req.Seed).Fork("workload")
		var nodeIDs []t.NodeID
		for i := 1; i <= cfg.NumNodes; i++ {
			nodeIDs = append(nodeIDs, t.NodeID(i))
		}
		workloadReqs := harness.GenerateWorkload(wRng, nodeIDs, wcfg)
		for _, r := range workloadReqs {
			s.EnqueueClientRequest(r.At, r.TargetNode, r.Req)
		}
	} else if req.NumPuts > 0 {
		for i := 0; i < req.NumPuts; i++ {
			putTime := 2*t.Second + t.Time(i)*t.Time(16*t.Second/t.Time(req.NumPuts+1))
			s.EnqueueClientRequest(putTime, t.NodeID(1), t.ClientReq{
				RequestID: uint64(i + 1),
				ClientID:  1,
				Op:        t.OpPut,
				Key:       fmt.Sprintf("k%02d", i%10),
				Value:     fmt.Sprintf("v%03d", i),
			})
		}
	}

	if cfg.AutoFaults == 0 && req.WithPartition && req.NumNodes >= 3 {
		s.EnqueuePartition(7*t.Second, [][]t.NodeID{
			{t.NodeID(1), t.NodeID(2)},
			{t.NodeID(3)},
		})
		s.EnqueueHeal(14 * t.Second)
	}

	res := s.Run()

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

	verdict := harness.TraceVerdict{Passed: true}
	if res.Violation != "" {
		verdict.Passed = false
		verdict.Violation = &checkers.Violation{
			Checker: "runtime_invariant",
			Detail:  res.Violation,
		}
	} else if v := checkers.CheckOneLeaderPerTerm(res.NodeSnapshots); v != nil {
		verdict.Passed = false
		verdict.Violation = v
	} else if v := checkers.CheckConvergence(res.NodeSnapshots); v != nil {
		verdict.Passed = false
		verdict.Violation = v
	}

	return &harness.RunTrace{
		Meta: harness.TraceMeta{
			Seed:         req.Seed,
			NumNodes:     cfg.NumNodes,
			MaxTimeNs:    int64(res.FinalTime),
			MaxTimeStr:   res.FinalTimeStr,
			DropRate:     req.DropRate,
			Partition:    req.WithPartition,
			NumClientOps: req.NumPuts,
		},
		Events:    traceEvents,
		Verdict:   verdict,
		TraceHash: res.TraceHash,
	}
}
