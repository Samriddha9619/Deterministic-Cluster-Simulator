package harness

import (
	"fmt"
	"sort"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// WorkloadConfig controls client workload generation.
type WorkloadConfig struct {
	NumOps      int        `json:"num_ops"`
	NumClients  int        `json:"num_clients"`
	KeyCount    int        `json:"key_count"`
	PutRatio    float64    `json:"put_ratio"`
	GetRatio    float64    `json:"get_ratio"`
	CASRatio    float64    `json:"cas_ratio"`
	DeleteRatio float64    `json:"delete_ratio"`
	HotKeyRatio float64    `json:"hot_key_ratio"` // probability of picking key-000 for high contention
	StartTime   t.Time     `json:"start_time_ns"`
	EndTime     t.Time     `json:"end_time_ns"`
}

// DefaultWorkloadConfig returns standard realistic benchmark parameters.
func DefaultWorkloadConfig() WorkloadConfig {
	return WorkloadConfig{
		NumOps:      50,
		NumClients:  3,
		KeyCount:    5,
		PutRatio:    0.5,
		GetRatio:    0.3,
		CASRatio:    0.1,
		DeleteRatio: 0.1,
		HotKeyRatio: 0.4,
		StartTime:   2 * t.Second,
		EndTime:     22 * t.Second,
	}
}

// ScheduledClientReq represents an operation to be submitted to a cluster node at virtual time At.
type ScheduledClientReq struct {
	At         t.Time      `json:"at_ns"`
	AtStr      string      `json:"at"`
	TargetNode t.NodeID    `json:"target_node"`
	Req        t.ClientReq `json:"req"`
}

// GenerateWorkload produces a deterministic sequence of client operations.
func GenerateWorkload(rng t.Rand, nodeIDs []t.NodeID, cfg WorkloadConfig) []ScheduledClientReq {
	if cfg.NumOps <= 0 || len(nodeIDs) == 0 {
		return nil
	}
	if cfg.NumClients <= 0 {
		cfg.NumClients = 1
	}
	if cfg.KeyCount <= 0 {
		cfg.KeyCount = 1
	}

	totalTime := cfg.EndTime - cfg.StartTime
	if totalTime <= 0 {
		totalTime = 10 * t.Second
	}
	interval := totalTime / t.Time(cfg.NumOps+1)

	// Keep track of values written so CAS has plausible expected values
	knownValues := make(map[string]string)

	reqs := make([]ScheduledClientReq, cfg.NumOps)
	for i := 0; i < cfg.NumOps; i++ {
		clientID := uint64(1 + (i % cfg.NumClients))
		reqID := uint64(i + 1)
		target := nodeIDs[rng.Intn(len(nodeIDs))]

		// Pick key with hot-key skew
		var key string
		if rng.Float64() < cfg.HotKeyRatio {
			key = "hot-000"
		} else {
			key = fmt.Sprintf("k-%03d", rng.Intn(cfg.KeyCount))
		}

		// Choose operation according to distribution
		r := rng.Float64()
		var op t.OpType
		var val, oldVal string

		switch {
		case r < cfg.PutRatio:
			op = t.OpPut
			val = fmt.Sprintf("val-%d-%d", clientID, reqID)
			knownValues[key] = val

		case r < cfg.PutRatio+cfg.GetRatio:
			op = t.OpGet

		case r < cfg.PutRatio+cfg.GetRatio+cfg.CASRatio:
			op = t.OpCAS
			oldVal = knownValues[key]
			val = fmt.Sprintf("cas-%d-%d", clientID, reqID)
			knownValues[key] = val

		default:
			op = t.OpDelete
			delete(knownValues, key)
		}

		at := cfg.StartTime + t.Time(i+1)*interval + t.Time(rng.Intn(int(interval/2+1)))
		reqs[i] = ScheduledClientReq{
			At:         at,
			AtStr:      at.String(),
			TargetNode: target,
			Req: t.ClientReq{
				RequestID: reqID,
				ClientID:  clientID,
				Op:        op,
				Key:       key,
				Value:     val,
				OldValue:  oldVal,
			},
		}
	}

	sort.SliceStable(reqs, func(i, j int) bool {
		return reqs[i].At < reqs[j].At
	})

	return reqs
}
