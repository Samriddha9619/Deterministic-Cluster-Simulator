// Package types defines the shared types used across the entire simulator.
//
// This package exists to break import cycles between /sim and /node.
// Both packages need Time, NodeID, and Envelope, so these live here where
// both can import them without circular dependencies.
//
// Rule: This package has ZERO dependencies on /sim, /node, or any other
// internal package. It depends only on the Go standard library.
package types

import (
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// Time — the unit of virtual time
// ---------------------------------------------------------------------------

// Time represents virtual time as nanoseconds since the start of the
// simulation. Using int64 gives us ~292 years of range.
//
// We deliberately do NOT use time.Duration or time.Time because those carry
// wall-clock semantics. Our Time is a pure integer: it advances only when the
// event loop says so, and it never goes backwards.
type Time int64

// Common virtual-time constants for readability: 150 * Millisecond instead
// of 150_000_000.
const (
	Nanosecond  Time = 1
	Microsecond      = 1000 * Nanosecond
	Millisecond      = 1000 * Microsecond
	Second           = 1000 * Millisecond
)

// Duration returns the Go standard library time.Duration equivalent.
// Useful only for display and logging — never for sleeping or scheduling.
func (t Time) Duration() time.Duration {
	return time.Duration(t)
}

// String formats virtual time as a human-readable duration string.
func (t Time) String() string {
	if t < Microsecond {
		return fmt.Sprintf("%dns", int64(t))
	}
	if t < Millisecond {
		return fmt.Sprintf("%.3fµs", float64(t)/float64(Microsecond))
	}
	if t < Second {
		return fmt.Sprintf("%.3fms", float64(t)/float64(Millisecond))
	}
	return fmt.Sprintf("%.3fs", float64(t)/float64(Second))
}

// ---------------------------------------------------------------------------
// NodeID — unique identifier for a node in the cluster
// ---------------------------------------------------------------------------

// NodeID uniquely identifies a node within a cluster. Values start from 1
// (matching etcd/raft's convention where 0 is reserved/invalid).
type NodeID uint64

// String returns a human-readable representation like "n1", "n2", etc.
func (id NodeID) String() string {
	return fmt.Sprintf("n%d", id)
}

// ---------------------------------------------------------------------------
// Envelope — a message between nodes
// ---------------------------------------------------------------------------

// Envelope wraps a Raft message with sender and receiver information.
// The Body is an opaque byte slice because different Raft implementations
// have different message types. The adapter serializes its library-specific
// message into bytes; the environment delivers the bytes; the receiving
// adapter deserializes them.
type Envelope struct {
	From NodeID
	To   NodeID
	Body []byte // serialized Raft message
}

// ---------------------------------------------------------------------------
// Client operations — the KV interface
// ---------------------------------------------------------------------------

// OpType enumerates the operations a client can perform on the KV store.
type OpType int

const (
	OpPut    OpType = iota // Put(key, value)
	OpGet                  // Get(key) → value
	OpDelete               // Delete(key)
	OpCAS                  // CAS(key, oldValue, newValue)
)

// String returns the operation name for logging and traces.
func (op OpType) String() string {
	switch op {
	case OpPut:
		return "put"
	case OpGet:
		return "get"
	case OpDelete:
		return "delete"
	case OpCAS:
		return "cas"
	default:
		return fmt.Sprintf("op(%d)", int(op))
	}
}

// ClientReq is a request from a client to the cluster.
type ClientReq struct {
	RequestID uint64 // globally unique, assigned by the harness
	ClientID  uint64 // which client sent this
	Op        OpType
	Key       string
	Value     string // for Put and CAS-new
	OldValue  string // for CAS-expected
}

// String returns a compact human-readable representation for traces.
func (r ClientReq) String() string {
	switch r.Op {
	case OpPut:
		return fmt.Sprintf("put(%s=%s)", r.Key, r.Value)
	case OpGet:
		return fmt.Sprintf("get(%s)", r.Key)
	case OpDelete:
		return fmt.Sprintf("del(%s)", r.Key)
	case OpCAS:
		return fmt.Sprintf("cas(%s:%s→%s)", r.Key, r.OldValue, r.Value)
	default:
		return fmt.Sprintf("?op(%d)", int(r.Op))
	}
}

// ---------------------------------------------------------------------------
// Client responses
// ---------------------------------------------------------------------------

// ClientResp is the node's response to a client request.
type ClientResp struct {
	RequestID uint64
	OK        bool   // true if the operation succeeded
	Value     string // the value read (for Get) or the current value (for failed CAS)
	Err       string // non-empty if something went wrong (not leader, etc.)
}

// ---------------------------------------------------------------------------
// Disk operations
// ---------------------------------------------------------------------------

// DiskOpType enumerates the kinds of disk operations a node can request.
type DiskOpType int

const (
	DiskWrite DiskOpType = iota // Write data to the WAL / state file
	DiskSync                    // Flush all pending writes to durable storage
)

// DiskOp is a request from the node to the disk subsystem.
type DiskOp struct {
	OpID   uint64
	Type   DiskOpType
	NodeID NodeID
	Data   []byte // serialized Raft hard state, entries, or snapshot
}

// DiskOpResult is the environment's response after completing a disk operation.
type DiskOpResult struct {
	OpID uint64
	OK   bool
	Err  string
}

// ---------------------------------------------------------------------------
// Timer
// ---------------------------------------------------------------------------

// TimerReq asks the environment to deliver an OnTick call at the given time.
type TimerReq struct {
	At Time
}

// ---------------------------------------------------------------------------
// Output — everything a node produces in response to one input
// ---------------------------------------------------------------------------

// Output is the complete set of side effects a node produces after handling
// a single input event.
type Output struct {
	Messages  []Envelope
	DiskOps   []DiskOp
	Responses []ClientResp
	SetTimer  *TimerReq
}

// IsEmpty returns true if the output contains no side effects.
func (o Output) IsEmpty() bool {
	return len(o.Messages) == 0 &&
		len(o.DiskOps) == 0 &&
		len(o.Responses) == 0 &&
		o.SetTimer == nil
}

// ---------------------------------------------------------------------------
// NodeView — a read-only snapshot for UI and checkers
// ---------------------------------------------------------------------------

// RaftRole represents a Raft node's current role.
type RaftRole int

const (
	RoleFollower  RaftRole = iota
	RoleCandidate
	RoleLeader
)

// String returns the role name.
func (r RaftRole) String() string {
	switch r {
	case RoleFollower:
		return "follower"
	case RoleCandidate:
		return "candidate"
	case RoleLeader:
		return "leader"
	default:
		return fmt.Sprintf("role(%d)", int(r))
	}
}

// LogEntrySummary summarizes a Raft log entry for inspection and log safety invariants.
type LogEntrySummary struct {
	Index uint64 `json:"index"`
	Term  uint64 `json:"term"`
}

// NodeView is a read-only snapshot of a node's state, used by the UI and checkers.
type NodeView struct {
	ID           NodeID
	Role         RaftRole
	Term         uint64
	VotedFor     NodeID
	LeaderID     NodeID
	CommitIndex  uint64
	AppliedIndex uint64
	LogLength    uint64
	LogEntries   []LogEntrySummary
	KVState      map[string]string
	Alive        bool
}

// Violation describes a safety or invariant failure found by a checker.
type Violation struct {
	Checker string `json:"checker"`
	Detail  string `json:"detail"`
}

func (v Violation) String() string {
	return fmt.Sprintf("[%s] %s", v.Checker, v.Detail)
}

// ---------------------------------------------------------------------------
// DiskImage — what survives a crash
// ---------------------------------------------------------------------------

// DiskImage represents the durable state that survives a node crash.
type DiskImage struct {
	Data []byte
}

// ---------------------------------------------------------------------------
// Node — the core interface
// ---------------------------------------------------------------------------

// Node is a deterministic state machine representing one replica. All inputs
// arrive through method calls; all outputs are returned as an Output struct.
// The node never performs I/O, never spawns goroutines, and never reads the
// wall clock.
type Node interface {
	ID() NodeID
	OnTick(now Time) Output
	OnMessage(now Time, m Envelope) Output
	OnClient(now Time, req ClientReq) Output
	OnDiskDone(now Time, result DiskOpResult) Output
	OnRestart(now Time, durable DiskImage) Output
	Snapshot() NodeView
}

// ---------------------------------------------------------------------------
// Rand — deterministic randomness interface
// ---------------------------------------------------------------------------

// Rand provides deterministic randomness for the simulation.
type Rand interface {
	Uint64() uint64
	Intn(n int) int
	Float64() float64
	Fork(label string) Rand
}

// ---------------------------------------------------------------------------
// Clock — the interface nodes use to ask "what time is it?"
// ---------------------------------------------------------------------------

// Clock provides the current virtual time.
type Clock interface {
	Now() Time
}
