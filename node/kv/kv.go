// Package kv implements a simple in-memory key-value state machine.
//
// This is the application-level state machine that Raft replicates. When
// Raft commits an entry, the entry is applied to this KV store. All nodes
// that apply the same entries in the same order will have identical state.
//
// The KV store is deterministic: given the same sequence of Apply calls,
// it produces the same state. No randomness, no time, no I/O.
package kv

import (
	"encoding/json"
	"fmt"
	"sort"
)

// ---------------------------------------------------------------------------
// Command — the unit of replication
// ---------------------------------------------------------------------------

// CommandType enumerates the operations that can be applied to the KV store.
type CommandType int

const (
	CmdPut    CommandType = iota // Put(key, value)
	CmdDelete                   // Delete(key)
	CmdCAS                      // CAS(key, old, new)
	CmdGet                      // Linearizable Get through Raft log
)

// Command is what gets serialized into a Raft log entry. When the entry is
// committed and applied, the command is decoded and executed against the
// KV store.
type Command struct {
	Type      CommandType `json:"type"`
	Key       string      `json:"key"`
	Value     string      `json:"value"`               // for Put and CAS-new
	OldValue  string      `json:"old_value,omitempty"`  // for CAS-expected
	RequestID uint64      `json:"request_id"`           // for dedup and response matching
	ClientID  uint64      `json:"client_id"`
}

// Encode serializes a command to JSON bytes for storage in a Raft log entry.
// We use JSON for readability in traces. In production you'd use protobuf.
func (c *Command) Encode() ([]byte, error) {
	return json.Marshal(c)
}

// DecodeCommand deserializes a command from JSON bytes.
func DecodeCommand(data []byte) (Command, error) {
	var cmd Command
	err := json.Unmarshal(data, &cmd)
	return cmd, err
}

// ---------------------------------------------------------------------------
// ApplyResult — the outcome of applying a command
// ---------------------------------------------------------------------------

// ApplyResult is the outcome of applying a single command to the KV store.
type ApplyResult struct {
	OK       bool   // true if the operation succeeded
	Value    string // the value read (for Get via snapshot) or current value (for failed CAS)
	Err      string // non-empty on failure (e.g., CAS mismatch)
}

// ---------------------------------------------------------------------------
// Store — the in-memory key-value state machine
// ---------------------------------------------------------------------------

type ClientSession struct {
	LastReqID  uint64      `json:"last_req_id"`
	LastResult ApplyResult `json:"last_result"`
}

// Store is a simple in-memory key-value map. It is the state machine that
// Raft replicates. Every committed entry is applied here via the Apply method.
//
// The Store is deterministic: same sequence of Apply calls → same state.
// It is NOT thread-safe (single-threaded simulation, same as everything else).
type Store struct {
	// data is the actual key-value map. We use a plain Go map, but when
	// we need to iterate (for snapshots, comparison), we always sort keys
	// first to maintain determinism (Go map iteration order is random).
	data     map[string]string
	sessions map[uint64]ClientSession
}

// NewStore creates an empty KV store.
func NewStore() *Store {
	return &Store{
		data:     make(map[string]string),
		sessions: make(map[uint64]ClientSession),
	}
}

// Apply executes a command against the store and returns the result.
// This is called for each committed Raft entry, in commit order.
// If the command is a duplicate (same or older RequestID from the same ClientID),
// it returns the cached result without re-executing mutations.
func (s *Store) Apply(cmd Command) ApplyResult {
	if cmd.ClientID > 0 && cmd.RequestID > 0 {
		if sess, exists := s.sessions[cmd.ClientID]; exists && sess.LastReqID >= cmd.RequestID {
			return sess.LastResult
		}
	}

	var res ApplyResult
	switch cmd.Type {
	case CmdPut:
		s.data[cmd.Key] = cmd.Value
		res = ApplyResult{OK: true}

	case CmdDelete:
		delete(s.data, cmd.Key)
		res = ApplyResult{OK: true}

	case CmdCAS:
		current, exists := s.data[cmd.Key]
		if !exists {
			if cmd.OldValue == "" {
				s.data[cmd.Key] = cmd.Value
				res = ApplyResult{OK: true}
			} else {
				res = ApplyResult{
					OK:  false,
					Err: fmt.Sprintf("key %q not found", cmd.Key),
				}
			}
		} else if current != cmd.OldValue {
			res = ApplyResult{
				OK:    false,
				Value: current,
				Err:   fmt.Sprintf("CAS mismatch: expected %q, got %q", cmd.OldValue, current),
			}
		} else {
			s.data[cmd.Key] = cmd.Value
			res = ApplyResult{OK: true}
		}

	case CmdGet:
		val, exists := s.data[cmd.Key]
		if exists {
			res = ApplyResult{OK: true, Value: val}
		} else {
			res = ApplyResult{OK: true, Value: ""}
		}

	default:
		res = ApplyResult{
			OK:  false,
			Err: fmt.Sprintf("unknown command type: %d", cmd.Type),
		}
	}

	if cmd.ClientID > 0 && cmd.RequestID > 0 {
		s.sessions[cmd.ClientID] = ClientSession{
			LastReqID:  cmd.RequestID,
			LastResult: res,
		}
	}

	return res
}

// Get reads a value from the store. Returns (value, true) if the key exists,
// ("", false) if it doesn't. This is for direct reads (e.g., linearizable
// reads that go through the leader), not for Raft-replicated operations.
func (s *Store) Get(key string) (string, bool) {
	v, ok := s.data[key]
	return v, ok
}

// Clone returns a deep copy of the store's data. Used for snapshots and
// checker comparisons. Keys are always sorted for determinism.
func (s *Store) Clone() map[string]string {
	clone := make(map[string]string, len(s.data))
	for k, v := range s.data {
		clone[k] = v
	}
	return clone
}

// Len returns the number of keys in the store.
func (s *Store) Len() int {
	return len(s.data)
}

// ---------------------------------------------------------------------------
// Snapshot serialization
// ---------------------------------------------------------------------------

// SnapshotData is the serializable form of the KV store's state, used for
// Raft snapshots (compaction). When Raft takes a snapshot, the entire KV
// state is serialized and stored; when a node restores from a snapshot,
// the state is deserialized back into the Store.
type SnapshotData struct {
	// Entries is sorted by key for deterministic serialization. We DON'T
	// use a map here because JSON map serialization order is undefined.
	Entries  []KVEntry      `json:"entries"`
	Sessions []SessionEntry `json:"sessions,omitempty"`
}

// KVEntry is a single key-value pair in a snapshot.
type KVEntry struct {
	Key   string `json:"k"`
	Value string `json:"v"`
}

// SessionEntry stores client session state in a snapshot.
type SessionEntry struct {
	ClientID   uint64      `json:"client_id"`
	LastReqID  uint64      `json:"last_req_id"`
	LastResult ApplyResult `json:"last_result"`
}

// Serialize converts the store's state to bytes for a Raft snapshot.
func (s *Store) Serialize() ([]byte, error) {
	// Collect keys and sort them for deterministic output.
	keys := make([]string, 0, len(s.data))
	for k := range s.data {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	entries := make([]KVEntry, len(keys))
	for i, k := range keys {
		entries[i] = KVEntry{Key: k, Value: s.data[k]}
	}

	// Sort client IDs for deterministic session serialization.
	clientIDs := make([]uint64, 0, len(s.sessions))
	for cid := range s.sessions {
		clientIDs = append(clientIDs, cid)
	}
	sort.Slice(clientIDs, func(i, j int) bool { return clientIDs[i] < clientIDs[j] })

	sessions := make([]SessionEntry, len(clientIDs))
	for i, cid := range clientIDs {
		sess := s.sessions[cid]
		sessions[i] = SessionEntry{
			ClientID:   cid,
			LastReqID:  sess.LastReqID,
			LastResult: sess.LastResult,
		}
	}

	return json.Marshal(SnapshotData{Entries: entries, Sessions: sessions})
}

// Restore replaces the store's state with data from a Raft snapshot.
func (s *Store) Restore(data []byte) error {
	var snap SnapshotData
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("kv: failed to restore snapshot: %w", err)
	}
	s.data = make(map[string]string, len(snap.Entries))
	for _, e := range snap.Entries {
		s.data[e.Key] = e.Value
	}
	s.sessions = make(map[uint64]ClientSession, len(snap.Sessions))
	for _, sess := range snap.Sessions {
		s.sessions[sess.ClientID] = ClientSession{
			LastReqID:  sess.LastReqID,
			LastResult: sess.LastResult,
		}
	}
	return nil
}
