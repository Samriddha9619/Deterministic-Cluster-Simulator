package harness

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/Samriddha9619/deterministic-cluster-simulator/checkers"
	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// TraceMeta holds execution metadata for a simulation run.
type TraceMeta struct {
	Seed         uint64  `json:"seed"`
	NumNodes     int     `json:"num_nodes"`
	MaxTimeNs    int64   `json:"max_time_ns"`
	MaxTimeStr   string  `json:"max_time"`
	DropRate     float64 `json:"drop_rate"`
	Partition    bool    `json:"partition"`
	NumClientOps int     `json:"num_client_ops"`
	GitCommit    string  `json:"git_commit,omitempty"`
	BugFlags     []string `json:"bug_flags,omitempty"`
}

// TraceEvent represents a single event step in the run for visualization.
type TraceEvent struct {
	Index     int          `json:"index"`
	At        int64        `json:"at_ns"`
	AtStr     string       `json:"at"`
	Kind      string       `json:"kind"`
	From      t.NodeID     `json:"from,omitempty"`
	Target    t.NodeID     `json:"target,omitempty"`
	Detail    string       `json:"detail"`
	Snapshots []t.NodeView `json:"snapshots,omitempty"`
}

// TraceVerdict represents the final evaluation of checkers on the run.
type TraceVerdict struct {
	Passed              bool                `json:"passed"`
	Violation           *checkers.Violation `json:"violation,omitempty"`
	ViolationEventIndex int                 `json:"violation_event_index,omitempty"`
}

// RunTrace is the complete recording of a run, serialized for the web UI.
type RunTrace struct {
	Meta      TraceMeta    `json:"meta"`
	Events    []TraceEvent `json:"events"`
	Verdict   TraceVerdict `json:"verdict"`
	TraceHash string       `json:"trace_hash"`
}

// ToJSON serializes the run trace to formatted JSON.
func (rt *RunTrace) ToJSON() ([]byte, error) {
	return json.MarshalIndent(rt, "", "  ")
}

// WriteNDJSON writes the trace in NDJSON format: line 1 meta, line 2..N events, last line verdict.
func (rt *RunTrace) WriteNDJSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	if err := enc.Encode(map[string]interface{}{"type": "meta", "data": rt.Meta}); err != nil {
		return fmt.Errorf("write meta: %w", err)
	}
	for _, evt := range rt.Events {
		if err := enc.Encode(map[string]interface{}{"type": "event", "data": evt}); err != nil {
			return fmt.Errorf("write event: %w", err)
		}
	}
	if err := enc.Encode(map[string]interface{}{
		"type":       "verdict",
		"verdict":    rt.Verdict,
		"trace_hash": rt.TraceHash,
	}); err != nil {
		return fmt.Errorf("write verdict: %w", err)
	}
	return nil
}

// SaveToFile writes the trace to a file path.
func (rt *RunTrace) SaveToFile(filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	return rt.WriteNDJSON(f)
}
