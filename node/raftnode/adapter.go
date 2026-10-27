// Package raftnode wraps etcd/raft's RawNode to implement the types.Node
// interface, making it a pure state machine suitable for deterministic
// simulation.
//
// This adapter is the bridge between our simulator's event-driven model and
// etcd/raft's Ready-based model. The key ordering guarantee:
//
//   1. Call RawNode.Tick() or RawNode.Step(msg) to advance Raft state.
//   2. Drain Ready():
//      a. Persist Entries + HardState to storage (via DiskOp).
//      b. ONLY AFTER disk reports durable: send Messages and apply CommittedEntries.
//      c. Call Advance().
//
// Getting this ordering right is the single most important correctness detail.
// The planted "ack before fsync" bug violates step 2b by sending messages
// before persistence completes.
package raftnode

import (
	"encoding/json"
	"fmt"

	"github.com/Samriddha9619/deterministic-cluster-simulator/node/kv"
	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"

	"go.etcd.io/raft/v3"
	pb "go.etcd.io/raft/v3/raftpb"
	"google.golang.org/protobuf/proto"
)

// ---------------------------------------------------------------------------
// RaftNode — the adapter implementing types.Node
// ---------------------------------------------------------------------------

// RaftNode adapts an etcd/raft RawNode into our types.Node interface.
// It manages:
//   - The RawNode (Raft consensus)
//   - A MemoryStorage (Raft's log and state persistence, in-memory)
//   - A KV Store (the application state machine)
//   - Pending operations waiting for disk confirmation
//
// Lifecycle:
//   1. Created via NewRaftNode with a cluster config.
//   2. Each input (OnTick, OnMessage, OnClient) advances the RawNode,
//      then drains Ready into an Output.
//   3. The event loop processes the Output (disk writes, message sends).
//   4. When disk completes, the event loop calls OnDiskDone, which triggers
//      message sending and entry application.
type RaftNode struct {
	id           t.NodeID
	raw          *raft.RawNode
	storage      *raft.MemoryStorage
	kv           *kv.Store
	peers        []t.NodeID
	nextDiskOpID uint64

	// appliedIndex tracks which entries we've applied to the KV store.
	appliedIndex uint64

	// pendingReady holds the Ready that is waiting for disk confirmation.
	// It is set when we get a Ready with MustSync=true, and cleared when
	// OnDiskDone is called.
	pendingReady *raft.Ready

	// pendingProposals maps request IDs to pending client requests so we
	// can send responses when entries are applied.
	pendingProposals map[uint64]t.ClientReq

	// tickInterval is the virtual time between Raft ticks.
	tickInterval t.Time

	// alive tracks whether this node is currently alive (not crashed).
	alive bool
}

// Config holds the parameters for creating a RaftNode.
type Config struct {
	ID            t.NodeID
	Peers         []t.NodeID // all node IDs in the cluster (including self)
	ElectionTick  int        // election timeout in ticks
	HeartbeatTick int        // heartbeat interval in ticks
	TickInterval  t.Time     // virtual time per tick
	Rand          t.Rand     // deterministic random source
}

// discardLogger implements raft.Logger by discarding all output. We don't
// want etcd/raft's internal logging going to stdout — all logging goes
// through our trace system.
type discardLogger struct{}

func (discardLogger) Debug(...interface{})            {}
func (discardLogger) Debugf(string, ...interface{})   {}
func (discardLogger) Info(...interface{})             {}
func (discardLogger) Infof(string, ...interface{})    {}
func (discardLogger) Warning(...interface{})          {}
func (discardLogger) Warningf(string, ...interface{}) {}
func (discardLogger) Error(...interface{})            {}
func (discardLogger) Errorf(string, ...interface{})   {}
func (discardLogger) Fatal(...interface{})            {}
func (discardLogger) Fatalf(string, ...interface{})   {}
func (discardLogger) Panic(...interface{})            {}
func (discardLogger) Panicf(string, ...interface{})   {}

// NewRaftNode creates a new RaftNode. It initializes the etcd/raft RawNode
// with the given configuration and bootstraps the cluster membership.
func NewRaftNode(cfg Config) (*RaftNode, error) {
	storage := raft.NewMemoryStorage()

	// Build the list of raft peers for bootstrap.
	raftPeers := make([]raft.Peer, len(cfg.Peers))
	for i, pid := range cfg.Peers {
		raftPeers[i] = raft.Peer{ID: uint64(pid)}
	}

	// Bootstrap the storage with the initial cluster configuration.
	if err := storage.SetHardState(&pb.HardState{}); err != nil {
		return nil, fmt.Errorf("raftnode: set initial hard state: %w", err)
	}

	raftCfg := &raft.Config{
		ID:              uint64(cfg.ID),
		ElectionTick:    cfg.ElectionTick,
		HeartbeatTick:   cfg.HeartbeatTick,
		Storage:         storage,
		MaxSizePerMsg:   1024 * 1024, // 1MB
		MaxInflightMsgs: 256,
		Logger:          discardLogger{},
		Rand:            cfg.Rand,
	}

	raw, err := raft.NewRawNode(raftCfg)
	if err != nil {
		return nil, fmt.Errorf("raftnode: create raw node: %w", err)
	}

	// Bootstrap the cluster membership.
	if err := raw.Bootstrap(raftPeers); err != nil {
		return nil, fmt.Errorf("raftnode: bootstrap: %w", err)
	}

	return &RaftNode{
		id:               cfg.ID,
		raw:              raw,
		storage:          storage,
		kv:               kv.NewStore(),
		peers:            cfg.Peers,
		pendingProposals: make(map[uint64]t.ClientReq),
		tickInterval:     cfg.TickInterval,
		alive:            true,
	}, nil
}

// ---------------------------------------------------------------------------
// types.Node interface implementation
// ---------------------------------------------------------------------------

// ID returns this node's unique identifier.
func (n *RaftNode) ID() t.NodeID {
	return n.id
}

// OnTick is called when the node's timer fires. It advances the Raft tick
// counter (which drives election and heartbeat timeouts) and then drains
// any resulting Ready.
func (n *RaftNode) OnTick(now t.Time) t.Output {
	if !n.alive {
		return t.Output{}
	}
	n.raw.Tick()
	return n.drainReady(now)
}

// OnMessage is called when a message from another node is delivered.
// It feeds the message to the Raft state machine and drains the Ready.
func (n *RaftNode) OnMessage(now t.Time, m t.Envelope) t.Output {
	if !n.alive {
		return t.Output{}
	}

	// Deserialize the Raft protobuf message from the envelope body.
	var msg pb.Message
	if err := proto.Unmarshal(m.Body, &msg); err != nil {
		// Corrupted message — silently drop.
		return t.Output{}
	}

	// Feed the message to Raft.
	if err := n.raw.Step(&msg); err != nil {
		// Step errors are non-fatal (e.g., message from unknown peer).
	}

	return n.drainReady(now)
}

// OnClient is called when a client request arrives at this node.
// If this node is the leader, it proposes the operation to Raft.
// If not, it returns an error.
func (n *RaftNode) OnClient(now t.Time, req t.ClientReq) t.Output {
	if !n.alive {
		return t.Output{}
	}


	// For Get operations, read directly from the applied state.
	// NOTE: This is a simplification for M0 — a proper linearizable read
	// requires a read-index or lease check.
	if req.Op == t.OpGet {
		val, _ := n.kv.Get(req.Key)
		return t.Output{
			Responses: []t.ClientResp{{
				RequestID: req.RequestID,
				OK:        true,
				Value:     val,
			}},
		}
	}

	// For write operations, create a KV command and propose it to Raft.
	cmd := kv.Command{
		RequestID: req.RequestID,
		ClientID:  req.ClientID,
		Key:       req.Key,
		Value:     req.Value,
		OldValue:  req.OldValue,
	}

	switch req.Op {
	case t.OpPut:
		cmd.Type = kv.CmdPut
	case t.OpDelete:
		cmd.Type = kv.CmdDelete
	case t.OpCAS:
		cmd.Type = kv.CmdCAS
	}

	data, err := cmd.Encode()
	if err != nil {
		return t.Output{
			Responses: []t.ClientResp{{
				RequestID: req.RequestID,
				OK:        false,
				Err:       fmt.Sprintf("encode command: %v", err),
			}},
		}
	}

	// Propose the command to Raft.
	if err := n.raw.Propose(data); err != nil {
		return t.Output{
			Responses: []t.ClientResp{{
				RequestID: req.RequestID,
				OK:        false,
				Err:       fmt.Sprintf("propose failed: %v", err),
			}},
		}
	}

	// Track the pending proposal for response matching.
	n.pendingProposals[req.RequestID] = req

	return n.drainReady(now)
}

// OnDiskDone is called when a previously requested disk operation completes.
// This is the critical ordering point: only NOW do we send the Raft messages
// and apply committed entries from the pending Ready.
func (n *RaftNode) OnDiskDone(now t.Time, result t.DiskOpResult) t.Output {
	if !n.alive {
		return t.Output{}
	}
	if !result.OK {
		return t.Output{}
	}

	output := t.Output{}

	if n.pendingReady != nil {
		rd := n.pendingReady
		n.pendingReady = nil

		// NOW it's safe to send messages (persistence is durable).
		for _, msg := range rd.Messages {
			body, err := proto.Marshal(msg)
			if err != nil {
				continue
			}
			output.Messages = append(output.Messages, t.Envelope{
				From: n.id,
				To:   t.NodeID(msg.GetTo()),
				Body: body,
			})
		}

		// Apply committed entries to the KV state machine.
		for _, entry := range rd.CommittedEntries {
			resps := n.applyEntry(entry)
			output.Responses = append(output.Responses, resps...)
		}

		// Tell Raft we've processed this Ready.
		n.raw.Advance(raft.Ready{})
	}

	// There might be more Readys after Advance.
	next := n.drainReady(now)
	output.Messages = append(output.Messages, next.Messages...)
	output.DiskOps = append(output.DiskOps, next.DiskOps...)
	output.Responses = append(output.Responses, next.Responses...)
	if next.SetTimer != nil {
		output.SetTimer = next.SetTimer
	}

	return output
}

// OnRestart is called after a crash to reinitialize the node from durable state.
func (n *RaftNode) OnRestart(now t.Time, durable t.DiskImage) t.Output {
	n.alive = true
	return n.drainReady(now)
}

// Snapshot returns a read-only view of the node's current state.
func (n *RaftNode) Snapshot() t.NodeView {
	view := t.NodeView{
		ID:           n.id,
		AppliedIndex: n.appliedIndex,
		KVState:      n.kv.Clone(),
		Alive:        n.alive,
	}

	if n.alive {
		status := n.raw.BasicStatus()
		view.Term = status.HardState.GetTerm()
		view.VotedFor = t.NodeID(status.HardState.GetVote())
		view.LeaderID = t.NodeID(status.Lead)
		view.CommitIndex = status.HardState.GetCommit()

		switch status.RaftState {
		case raft.StateFollower:
			view.Role = t.RoleFollower
		case raft.StateCandidate:
			view.Role = t.RoleCandidate
		case raft.StateLeader:
			view.Role = t.RoleLeader
		}
	}

	return view
}

// ---------------------------------------------------------------------------
// Internal: Ready processing
// ---------------------------------------------------------------------------

// drainReady checks if the RawNode has a Ready and processes it.
// Implements the Ready contract:
//   1. Persist Entries + HardState to MemoryStorage.
//   2. If MustSync, request a disk op (messages wait for OnDiskDone).
//   3. If !MustSync, send messages immediately and apply entries.
func (n *RaftNode) drainReady(now t.Time) t.Output {
	if n.pendingReady != nil {
		return t.Output{}
	}
	if !n.raw.HasReady() {
		return t.Output{}
	}

	rd := n.raw.Ready()
	output := t.Output{}

	// Step 1: Persist entries and hard state to the in-memory storage.
	if rd.HardState != nil {
		if err := n.storage.SetHardState(rd.HardState); err != nil {
			panic(fmt.Sprintf("raftnode: set hard state: %v", err))
		}
	}
	if len(rd.Entries) > 0 {
		if err := n.storage.Append(rd.Entries); err != nil {
			panic(fmt.Sprintf("raftnode: append entries: %v", err))
		}
	}
	if rd.Snapshot != nil && !raft.IsEmptySnap(rd.Snapshot) {
		if err := n.storage.ApplySnapshot(rd.Snapshot); err != nil {
			panic(fmt.Sprintf("raftnode: apply snapshot: %v", err))
		}
	}

	if rd.MustSync {
		// Persistence required before sending messages.
		diskData := n.serializeForDisk(rd)
		n.nextDiskOpID++
		output.DiskOps = append(output.DiskOps, t.DiskOp{
			OpID:   n.nextDiskOpID,
			Type:   t.DiskSync,
			NodeID: n.id,
			Data:   diskData,
		})
		saved := rd
		n.pendingReady = &saved
	} else {
		// No sync needed — send messages and apply immediately.
		for _, msg := range rd.Messages {
			body, err := proto.Marshal(msg)
			if err != nil {
				continue
			}
			output.Messages = append(output.Messages, t.Envelope{
				From: n.id,
				To:   t.NodeID(msg.GetTo()),
				Body: body,
			})
		}

		for _, entry := range rd.CommittedEntries {
			resps := n.applyEntry(entry)
			output.Responses = append(output.Responses, resps...)
		}

		n.raw.Advance(raft.Ready{})
	}

	return output
}

// applyEntry applies a single committed Raft log entry to the KV state machine.
func (n *RaftNode) applyEntry(entry *pb.Entry) []t.ClientResp {
	idx := entry.GetIndex()
	if idx > n.appliedIndex {
		n.appliedIndex = idx
	}

	// Skip config change entries and empty entries (Raft internals).
	entryType := entry.GetType()
	if entryType != pb.EntryNormal || len(entry.GetData()) == 0 {
		return nil
	}

	// Decode and apply the KV command.
	cmd, err := kv.DecodeCommand(entry.GetData())
	if err != nil {
		return nil
	}

	result := n.kv.Apply(cmd)

	// Look up the pending proposal to send a client response.
	if req, ok := n.pendingProposals[cmd.RequestID]; ok {
		delete(n.pendingProposals, cmd.RequestID)
		return []t.ClientResp{{
			RequestID: req.RequestID,
			OK:        result.OK,
			Value:     result.Value,
			Err:       result.Err,
		}}
	}

	return nil
}

// serializeForDisk creates a serialized representation of data that needs
// to be persisted.
func (n *RaftNode) serializeForDisk(rd raft.Ready) []byte {
	// For M0, a simple JSON blob. Later milestones will use a proper WAL format.
	type diskPayload struct {
		HasHardState bool   `json:"has_hs"`
		HardState    []byte `json:"hs,omitempty"`
		NumEntries   int    `json:"num_entries"`
	}
	payload := diskPayload{
		HasHardState: rd.HardState != nil,
		NumEntries:   len(rd.Entries),
	}
	if rd.HardState != nil {
		payload.HardState, _ = proto.Marshal(rd.HardState)
	}
	data, _ := json.Marshal(payload)
	return data
}

// SetAlive sets whether this node is alive. Used by the event loop for
// crash/restart.
func (n *RaftNode) SetAlive(alive bool) {
	n.alive = alive
}

// Compile-time interface check.
var _ t.Node = (*RaftNode)(nil)
