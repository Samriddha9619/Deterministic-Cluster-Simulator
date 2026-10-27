package harness

import (
	"encoding/json"
	"fmt"
	"sync"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// OpType represents the client operation kind.
type OpType string

const (
	OpPut    OpType = "put"
	OpGet    OpType = "get"
	OpDelete OpType = "delete"
	OpCAS    OpType = "cas"
)

// HistoryKind represents call (invocation) vs return (completion).
type HistoryKind string

const (
	KindInvoke HistoryKind = "invoke" // client initiates request
	KindOK     HistoryKind = "ok"     // request completed successfully
	KindFail   HistoryKind = "fail"   // request failed deterministically (e.g. CAS mismatch)
	KindInfo   HistoryKind = "info"   // indeterminate: timed out or node crashed
)

// HistoryEntry is a single Jepsen-style operation record.
type HistoryEntry struct {
	Index     int         `json:"index"`
	ClientID  uint64      `json:"client_id"`
	RequestID uint64      `json:"request_id"`
	Kind      HistoryKind `json:"kind"`
	Op        OpType      `json:"op"`
	Key       string      `json:"key"`
	Value     string      `json:"value,omitempty"`
	OldValue  string      `json:"old_value,omitempty"`
	At        t.Time      `json:"at_ns"`
	AtStr     string      `json:"at"`
	Err       string      `json:"err,omitempty"`
}

func (h HistoryEntry) String() string {
	switch h.Kind {
	case KindInvoke:
		return fmt.Sprintf("[%s] c%d: invoke %s %s=%s (req=%d)", h.At, h.ClientID, h.Op, h.Key, h.Value, h.RequestID)
	case KindOK:
		return fmt.Sprintf("[%s] c%d: ok %s %s=%s (req=%d)", h.At, h.ClientID, h.Op, h.Key, h.Value, h.RequestID)
	case KindFail:
		return fmt.Sprintf("[%s] c%d: fail %s %s (req=%d): %s", h.At, h.ClientID, h.Op, h.Key, h.RequestID, h.Err)
	case KindInfo:
		return fmt.Sprintf("[%s] c%d: info(timeout) %s %s (req=%d)", h.At, h.ClientID, h.Op, h.Key, h.RequestID)
	default:
		return fmt.Sprintf("[%s] c%d: %s %s", h.At, h.ClientID, h.Kind, h.Op)
	}
}

// HistoryRecorder records client calls and returns for verification.
type HistoryRecorder struct {
	mu      sync.Mutex
	entries []HistoryEntry
	pending map[uint64]HistoryEntry // map[request_id]invoke_entry
}

// NewHistoryRecorder creates an empty history recorder.
func NewHistoryRecorder() *HistoryRecorder {
	return &HistoryRecorder{
		entries: make([]HistoryEntry, 0, 1024),
		pending: make(map[uint64]HistoryEntry),
	}
}

// RecordInvoke logs the invocation of a client request.
func (hr *HistoryRecorder) RecordInvoke(at t.Time, req t.ClientReq) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	var op OpType
	switch req.Op {
	case t.OpPut:
		op = OpPut
	case t.OpGet:
		op = OpGet
	case t.OpDelete:
		op = OpDelete
	case t.OpCAS:
		op = OpCAS
	}

	entry := HistoryEntry{
		Index:     len(hr.entries),
		ClientID:  req.ClientID,
		RequestID: req.RequestID,
		Kind:      KindInvoke,
		Op:        op,
		Key:       req.Key,
		Value:     req.Value,
		OldValue:  req.OldValue,
		At:        at,
		AtStr:     at.String(),
	}

	hr.entries = append(hr.entries, entry)
	hr.pending[req.RequestID] = entry
}

// RecordReturn logs the completion of a client request.
func (hr *HistoryRecorder) RecordReturn(at t.Time, resp t.ClientResp) (HistoryEntry, bool) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	invoke, ok := hr.pending[resp.RequestID]
	if !ok {
		return HistoryEntry{}, false
	}
	delete(hr.pending, resp.RequestID)

	kind := KindOK
	if !resp.OK {
		kind = KindFail
	}

	val := resp.Value
	if val == "" {
		if invoke.Op == OpPut || (invoke.Op == OpCAS && resp.OK) {
			val = invoke.Value
		}
	}

	entry := HistoryEntry{
		Index:     len(hr.entries),
		ClientID:  invoke.ClientID,
		RequestID: resp.RequestID,
		Kind:      kind,
		Op:        invoke.Op,
		Key:       invoke.Key,
		Value:     val,
		OldValue:  invoke.OldValue,
		At:        at,
		AtStr:     at.String(),
		Err:       resp.Err,
	}

	hr.entries = append(hr.entries, entry)
	return entry, true
}

// FinalizeIndeterminate marks any unanswered requests as indeterminate (KindInfo)
// at the end of the simulation.
func (hr *HistoryRecorder) FinalizeIndeterminate(at t.Time) {
	hr.mu.Lock()
	defer hr.mu.Unlock()

	for reqID, invoke := range hr.pending {
		entry := HistoryEntry{
			Index:     len(hr.entries),
			ClientID:  invoke.ClientID,
			RequestID: reqID,
			Kind:      KindInfo,
			Op:        invoke.Op,
			Key:       invoke.Key,
			Value:     invoke.Value,
			OldValue:  invoke.OldValue,
			At:        at,
			AtStr:     at.String(),
			Err:       "timeout / node uncontactable",
		}
		hr.entries = append(hr.entries, entry)
	}
	hr.pending = make(map[uint64]HistoryEntry)
}

// Entries returns a copy of all recorded history entries.
func (hr *HistoryRecorder) Entries() []HistoryEntry {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	res := make([]HistoryEntry, len(hr.entries))
	copy(res, hr.entries)
	return res
}

// ToJSON formats the entire history as JSON.
func (hr *HistoryRecorder) ToJSON() ([]byte, error) {
	hr.mu.Lock()
	defer hr.mu.Unlock()
	return json.MarshalIndent(hr.entries, "", "  ")
}
