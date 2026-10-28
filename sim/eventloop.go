package sim

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/Samriddha9619/deterministic-cluster-simulator/checkers"
	"github.com/Samriddha9619/deterministic-cluster-simulator/harness"
	"github.com/Samriddha9619/deterministic-cluster-simulator/node/raftnode"
	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// ---------------------------------------------------------------------------
// SimConfig — configuration for a simulation run
// ---------------------------------------------------------------------------

// SimConfig holds all parameters for a single simulation run.
type SimConfig struct {
	// Seed is the master seed from which all randomness is derived.
	Seed uint64

	// NumNodes is the number of nodes in the cluster.
	NumNodes int

	// MaxEvents is the maximum number of events to process before stopping.
	MaxEvents int

	// MaxTime is the maximum virtual time before stopping.
	MaxTime t.Time

	// TickInterval is the virtual time between Raft ticks.
	TickInterval t.Time

	// ElectionTick is the number of ticks for an election timeout.
	ElectionTick int

	// HeartbeatTick is the number of ticks between heartbeats.
	HeartbeatTick int

	// NetworkConfig overrides default link settings (optional).
	LinkConfig *LinkConfig

	// DiskConfig overrides default disk settings (optional).
	DiskCfg *DiskConfig

	// Faults is an explicit fault plan to execute. If nil, faults are
	// either generated via AutoFaults or enqueued manually.
	Faults *FaultPlan

	// AutoFaults, if > 0, generates a random fault plan up front from
	// the dedicated "faults" RNG stream.
	AutoFaults int

	// Bugs holds planted bug flags for verification and shrinking.
	Bugs t.BugFlags
}

// DefaultSimConfig returns a reasonable config for M0 testing.
func DefaultSimConfig(seed uint64) SimConfig {
	return SimConfig{
		Seed:          seed,
		NumNodes:      3,
		MaxEvents:     50000,
		MaxTime:       30 * t.Second,
		TickInterval:  100 * t.Millisecond,
		ElectionTick:  10,
		HeartbeatTick: 1,
	}
}

// ---------------------------------------------------------------------------
// TraceEvent — one event recorded for the trace
// ---------------------------------------------------------------------------

// TraceEvent is the JSON-serializable record of one event in the trace.
// It captures everything needed to replay and inspect the run.
type TraceEvent struct {
	Index     int          `json:"index"`
	At        int64        `json:"at_ns"` // virtual time in nanoseconds
	AtStr     string       `json:"at"`    // human-readable time
	Kind      string       `json:"kind"`
	From      string       `json:"from,omitempty"`
	Target    string       `json:"target,omitempty"`
	Detail    string       `json:"detail,omitempty"`
	Snapshots []t.NodeView `json:"snapshots,omitempty"`
}

// ---------------------------------------------------------------------------
// Simulator — the main event loop
// ---------------------------------------------------------------------------

// Simulator runs a deterministic simulation of a Raft cluster. It owns the
// event queue, virtual clock, network, disks, and nodes. Everything runs
// on a single goroutine — no concurrency anywhere.
type Simulator struct {
	config   SimConfig
	clock    VirtualClock
	queue    *EventQueue
	network  *Network
	nodes    map[t.NodeID]*raftnode.RaftNode
	disks    map[t.NodeID]*FakeDisk
	nodeIDs  []t.NodeID // sorted for determinism

	// masterRand is the root RNG; sub-streams are forked from it.
	masterRand *SimRand

	// trace records every event for later replay and checking.
	trace []TraceEvent

	// eventCount tracks how many events have been processed.
	eventCount int

	// invariantViolation is set if an invariant check fails during the run.
	invariantViolation string

	// faultPlan is the active schedule of faults.
	faultPlan *FaultPlan

	// history records client calls and returns for linearizability checks.
	history *harness.HistoryRecorder

	// durability verifies acknowledged writes survive across crashes.
	durability *checkers.DurabilityChecker
}

// NewSimulator creates and initializes a simulator from the given config.
func NewSimulator(cfg SimConfig) (*Simulator, error) {
	masterRand := NewRand(cfg.Seed)

	// Build node IDs: 1, 2, ..., NumNodes
	nodeIDs := make([]t.NodeID, cfg.NumNodes)
	for i := range nodeIDs {
		nodeIDs[i] = t.NodeID(i + 1)
	}

	// Create the network with a forked RNG stream.
	netRng := masterRand.Fork("network")
	network := NewNetwork(netRng, nodeIDs)

	// Apply custom link config if provided.
	if cfg.LinkConfig != nil {
		for _, from := range nodeIDs {
			for _, to := range nodeIDs {
				if from != to {
					network.SetLinkConfig(from, to, *cfg.LinkConfig)
				}
			}
		}
	}

	// Determine disk config.
	diskCfg := DefaultDiskConfig()
	if cfg.DiskCfg != nil {
		diskCfg = *cfg.DiskCfg
	}

	// Create nodes and disks.
	nodes := make(map[t.NodeID]*raftnode.RaftNode, cfg.NumNodes)
	disks := make(map[t.NodeID]*FakeDisk, cfg.NumNodes)

	for _, id := range nodeIDs {
		// Each node's disk gets its own forked RNG stream.
		diskRng := masterRand.Fork(fmt.Sprintf("disk-%s", id))
		disks[id] = NewFakeDisk(id, diskCfg, diskRng)

		// Each node's Raft state machine gets its own forked RNG stream for election timeouts.
		raftRng := masterRand.Fork(fmt.Sprintf("raft-%s", id))

		node, err := raftnode.NewRaftNode(raftnode.Config{
			ID:            id,
			Peers:         nodeIDs,
			ElectionTick:  cfg.ElectionTick,
			HeartbeatTick: cfg.HeartbeatTick,
			TickInterval:  cfg.TickInterval,
			Rand:          raftRng,
			Bugs:          cfg.Bugs,
		})
		if err != nil {
			return nil, fmt.Errorf("sim: create node %s: %w", id, err)
		}
		nodes[id] = node
	}

	// Generate upfront fault plan if AutoFaults > 0 and no plan specified.
	faultPlan := cfg.Faults
	if faultPlan == nil && cfg.AutoFaults > 0 {
		faultRng := masterRand.Fork("faults")
		faultPlan = GenerateRandomFaultPlan(faultRng, nodeIDs, cfg.MaxTime, cfg.AutoFaults)
	}

	sim := &Simulator{
		config:     cfg,
		queue:      NewEventQueue(),
		network:    network,
		nodes:      nodes,
		disks:      disks,
		nodeIDs:    nodeIDs,
		masterRand: masterRand,
		trace:      make([]TraceEvent, 0, cfg.MaxEvents),
		faultPlan:  faultPlan,
		history:    harness.NewHistoryRecorder(),
		durability: checkers.NewDurabilityChecker(),
	}

	// Schedule the fault plan if present.
	if faultPlan != nil {
		faultPlan.ScheduleOn(sim)
	}

	// Schedule the initial tick for each node. Using sorted nodeIDs
	// ensures deterministic ordering of the first events.
	for _, id := range nodeIDs {
		sim.queue.Enqueue(&Event{
			At:     cfg.TickInterval,
			Kind:   EventTick,
			Target: id,
		})
	}

	return sim, nil
}

// ---------------------------------------------------------------------------
// Run — the main event loop
// ---------------------------------------------------------------------------

// RunResult is the outcome of a simulation run.
type RunResult struct {
	Seed          uint64                 `json:"seed"`
	EventCount    int                    `json:"event_count"`
	FinalTime     t.Time                 `json:"final_time_ns"`
	FinalTimeStr  string                 `json:"final_time"`
	Violation     string                 `json:"violation,omitempty"`
	TraceHash     string                 `json:"trace_hash"`
	NodeSnapshots []t.NodeView           `json:"node_snapshots"`
	Trace         []TraceEvent           `json:"trace,omitempty"`
	FaultPlan     *FaultPlan             `json:"fault_plan,omitempty"`
	History       []harness.HistoryEntry `json:"history,omitempty"`
}

// Run executes the simulation until a stopping condition is met:
//   - MaxEvents reached
//   - MaxTime reached
//   - Event queue is empty
//   - An invariant violation is detected
//
// Returns the run result including the trace hash (for determinism checking).
func (s *Simulator) Run() RunResult {
	hasher := sha256.New()

	for s.eventCount < s.config.MaxEvents {
		evt := s.queue.Dequeue()
		if evt == nil {
			break // queue empty
		}

		// Stop if we've exceeded the time limit.
		if evt.At > s.config.MaxTime {
			break
		}

		// Advance the virtual clock.
		s.clock.Set(evt.At)

		// Record the event in the trace.
		te := TraceEvent{
			Index: s.eventCount,
			At:    int64(evt.At),
			AtStr: evt.At.String(),
			Kind:  evt.Kind.String(),
		}
		if evt.Target != 0 {
			te.Target = evt.Target.String()
		}
		if evt.Message != nil && evt.Message.From != 0 {
			te.From = evt.Message.From.String()
		}
		te.Detail = evt.String()

		// Hash the trace event for determinism checking.
		// We hash the index, time, kind, and target — enough to detect
		// any divergence in event ordering.
		fmt.Fprintf(hasher, "%d|%d|%d|%d\n", te.Index, te.At, evt.Kind, evt.Target)

		// Dispatch the event.
		s.dispatch(evt)

		// Snapshot cluster state after this event has been handled.
		snaps := s.NodeSnapshots()
		te.Snapshots = snaps

		s.trace = append(s.trace, te)
		s.eventCount++

		// Check invariants after every event.
		if v := s.checkInvariants(); v != "" {
			s.invariantViolation = v
			break
		}
	}

	// Finalize any pending client requests as indeterminate.
	s.history.FinalizeIndeterminate(s.clock.Now())
	for _, entry := range s.history.Entries() {
		if entry.Kind == harness.KindInfo {
			s.durability.RecordIndeterminate(entry)
		}
	}

	// Compute final trace hash.
	hashBytes := hasher.Sum(nil)
	hashStr := hex.EncodeToString(hashBytes)

	// Collect final node snapshots.
	snapshots := s.NodeSnapshots()

	// If no online violation occurred, check quiescent convergence, durability, and linearizability.
	if s.invariantViolation == "" {
		if v := checkers.CheckConvergence(snapshots); v != nil {
			s.invariantViolation = v.String()
		} else if v := s.durability.CheckFinalState(snapshots); v != nil {
			s.invariantViolation = v.String()
		} else if v := checkers.CheckLinearizability(s.history.Entries()); v != nil {
			s.invariantViolation = v.String()
		}
	}

	return RunResult{
		Seed:          s.config.Seed,
		EventCount:    s.eventCount,
		FinalTime:     s.clock.Now(),
		FinalTimeStr:  s.clock.Now().String(),
		Violation:     s.invariantViolation,
		TraceHash:     hashStr,
		NodeSnapshots: snapshots,
		Trace:         s.trace,
		FaultPlan:     s.faultPlan,
		History:       s.history.Entries(),
	}
}

// NodeSnapshots returns the current view of all nodes in sorted order.
func (s *Simulator) NodeSnapshots() []t.NodeView {
	snaps := make([]t.NodeView, len(s.nodeIDs))
	for i, id := range s.nodeIDs {
		snaps[i] = s.nodes[id].Snapshot()
	}
	return snaps
}

// History returns a copy of the simulation's recorded client history.
func (s *Simulator) History() []harness.HistoryEntry {
	return s.history.Entries()
}

// ---------------------------------------------------------------------------
// Event dispatch
// ---------------------------------------------------------------------------

// dispatch routes an event to the appropriate handler.
func (s *Simulator) dispatch(evt *Event) {
	switch evt.Kind {
	case EventTick:
		s.handleTick(evt)
	case EventDeliver:
		s.handleDeliver(evt)
	case EventClientReq:
		s.handleClientReq(evt)
	case EventDiskDone:
		s.handleDiskDone(evt)
	case EventRestart:
		s.handleRestart(evt)
	case EventCrash:
		s.handleCrash(evt)
	case EventPartition:
		s.handlePartition(evt)
	case EventHeal:
		s.handleHeal(evt)
	case EventLinkCut:
		s.handleLinkCut(evt)
	case EventLinkRestore:
		s.handleLinkRestore(evt)
	}
}

// handleTick dispatches a tick to a node and processes the output.
// IMPORTANT: Always schedule the next tick for alive nodes, even if
// Raft has no Ready. Without this, ticks stop and elections never fire.
func (s *Simulator) handleTick(evt *Event) {
	node, ok := s.nodes[evt.Target]
	if !ok || !s.network.IsAlive(evt.Target) {
		return
	}
	output := node.OnTick(s.clock.Now())
	s.processOutput(evt.Target, output)

	// Always schedule the next tick. The node's drainReady may or may not
	// set a timer, but we need ticks to keep firing regardless.
	if output.SetTimer == nil {
		s.queue.Enqueue(&Event{
			At:     s.clock.Now() + s.config.TickInterval,
			Kind:   EventTick,
			Target: evt.Target,
		})
	}
}

// handleDeliver dispatches a message delivery to a node.
func (s *Simulator) handleDeliver(evt *Event) {
	if evt.Message == nil {
		return
	}
	// Check at delivery time (not send time) whether the receiver is alive
	// and reachable. This models in-flight messages being affected by
	// partitions that occur after sending.
	if !s.network.IsAlive(evt.Target) {
		return // node is dead, drop the message
	}
	if s.network.IsPartitioned(evt.Message.From, evt.Target) {
		return // partition appeared after send, drop
	}

	node, ok := s.nodes[evt.Target]
	if !ok {
		return
	}
	output := node.OnMessage(s.clock.Now(), *evt.Message)
	s.processOutput(evt.Target, output)
}

// handleClientReq dispatches a client request to a node.
func (s *Simulator) handleClientReq(evt *Event) {
	if evt.ClientReq == nil {
		return
	}
	s.history.RecordInvoke(s.clock.Now(), *evt.ClientReq)

	node, ok := s.nodes[evt.Target]
	if !ok || !s.network.IsAlive(evt.Target) {
		return
	}
	output := node.OnClient(s.clock.Now(), *evt.ClientReq)
	s.processOutput(evt.Target, output)
}

// handleDiskDone dispatches a disk completion to a node.
func (s *Simulator) handleDiskDone(evt *Event) {
	if evt.DiskResult == nil {
		return
	}
	node, ok := s.nodes[evt.Target]
	if !ok || !s.network.IsAlive(evt.Target) {
		return
	}
	output := node.OnDiskDone(s.clock.Now(), *evt.DiskResult)
	s.processOutput(evt.Target, output)
}

// handleRestart restarts a crashed node from its durable state.
func (s *Simulator) handleRestart(evt *Event) {
	node, ok := s.nodes[evt.Target]
	if !ok {
		return
	}
	disk, ok := s.disks[evt.Target]
	if !ok {
		return
	}

	s.network.RestartNode(evt.Target)
	node.SetAlive(true)
	durable := disk.DurableImage()
	output := node.OnRestart(s.clock.Now(), durable)
	s.processOutput(evt.Target, output)

	// Reschedule ticks for the restarted node so it can participate in elections and heartbeats.
	s.queue.Enqueue(&Event{
		At:     s.clock.Now() + s.config.TickInterval,
		Kind:   EventTick,
		Target: evt.Target,
	})
}

// handleCrash crashes a node, wiping its volatile disk state.
func (s *Simulator) handleCrash(evt *Event) {
	node, ok := s.nodes[evt.Target]
	if !ok {
		return
	}
	disk, ok := s.disks[evt.Target]
	if !ok {
		return
	}

	s.network.CrashNode(evt.Target)
	node.SetAlive(false)
	disk.Crash()
}

// handlePartition installs a network partition.
func (s *Simulator) handlePartition(evt *Event) {
	if evt.Partition != nil {
		s.network.SetPartition(evt.Partition)
	}
}

// handleHeal removes all network partitions.
func (s *Simulator) handleHeal(evt *Event) {
	s.network.Heal()
}

// handleLinkCut cuts a single directed link.
func (s *Simulator) handleLinkCut(evt *Event) {
	s.network.CutLink(evt.LinkFrom, evt.LinkTo)
}

// handleLinkRestore restores a single directed link.
func (s *Simulator) handleLinkRestore(evt *Event) {
	s.network.RestoreLink(evt.LinkFrom, evt.LinkTo)
}

// ---------------------------------------------------------------------------
// Output processing
// ---------------------------------------------------------------------------

// processOutput takes the output from a node and creates the corresponding
// events (message deliveries, disk ops, timers).
func (s *Simulator) processOutput(nodeID t.NodeID, output t.Output) {
	now := s.clock.Now()

	// Process disk operations.
	for _, op := range output.DiskOps {
		disk, ok := s.disks[nodeID]
		if !ok {
			continue
		}

		switch op.Type {
		case t.DiskWrite:
			result := disk.Write(now, op.Data)
			s.queue.Enqueue(&Event{
				At:     result.CompletesAt,
				Kind:   EventDiskDone,
				Target: nodeID,
				DiskResult: &t.DiskOpResult{
					OpID: op.OpID,
					OK:   true,
				},
			})
		case t.DiskSync:
			// Write first, then sync.
			disk.Write(now, op.Data)
			result := disk.Sync(now)
			s.queue.Enqueue(&Event{
				At:     result.CompletesAt,
				Kind:   EventDiskDone,
				Target: nodeID,
				DiskResult: &t.DiskOpResult{
					OpID: op.OpID,
					OK:   true,
				},
			})
		}
	}

	// Process outgoing messages.
	for _, msg := range output.Messages {
		// Deep copy the message so nodes never share memory.
		msgCopy := CopyEnvelope(msg)

		// Ask the network whether/when to deliver.
		result := s.network.Send(now, msgCopy)
		if result.Delivered {
			s.queue.Enqueue(&Event{
				At:      result.DeliverAt,
				Kind:    EventDeliver,
				Target:  msgCopy.To,
				Message: &msgCopy,
			})
			if result.Duplicate {
				dupeCopy := CopyEnvelope(msgCopy)
				s.queue.Enqueue(&Event{
					At:      result.DuplicateAt,
					Kind:    EventDeliver,
					Target:  dupeCopy.To,
					Message: &dupeCopy,
				})
			}
		}
		// If not delivered, the message is silently dropped (logged in trace).
	}

	// Process timer requests.
	if output.SetTimer != nil {
		s.queue.Enqueue(&Event{
			At:     output.SetTimer.At,
			Kind:   EventTick,
			Target: nodeID,
		})
	}

	// Process client responses.
	for _, resp := range output.Responses {
		if entry, ok := s.history.RecordReturn(now, resp); ok && entry.Kind == harness.KindOK {
			s.durability.RecordAck(entry)
		}
	}
}

// ---------------------------------------------------------------------------
// Invariant checking
// ---------------------------------------------------------------------------

// checkInvariants runs cheap invariant checks after every event.
// Returns a non-empty string describing the first violation found.
func (s *Simulator) checkInvariants() string {
	snaps := s.NodeSnapshots()
	if v := checkers.CheckOneLeaderPerTerm(snaps); v != nil {
		return v.String()
	}
	if v := checkers.CheckLogMatching(snaps); v != nil {
		return v.String()
	}
	return ""
}

// checkOneLeaderPerTerm verifies that at most one node claims to be leader
// for any given term. This is Raft's most fundamental safety property.
func (s *Simulator) checkOneLeaderPerTerm() string {
	// termLeaders maps term → node ID of the leader in that term.
	// We only check currently alive nodes.
	termLeaders := make(map[uint64]t.NodeID)

	// Iterate nodeIDs in sorted order for determinism.
	for _, id := range s.nodeIDs {
		snap := s.nodes[id].Snapshot()
		if !snap.Alive {
			continue
		}
		if snap.Role != t.RoleLeader {
			continue
		}
		if existing, ok := termLeaders[snap.Term]; ok {
			if existing != id {
				return fmt.Sprintf(
					"invariant violation: two leaders in term %d: %s and %s",
					snap.Term, existing, id,
				)
			}
		}
		termLeaders[snap.Term] = id
	}
	return ""
}

// ---------------------------------------------------------------------------
// Public helpers for fault injection
// ---------------------------------------------------------------------------

// EnqueueCrash schedules a node crash at the given virtual time.
func (s *Simulator) EnqueueCrash(at t.Time, nodeID t.NodeID) {
	s.queue.Enqueue(&Event{
		At:     at,
		Kind:   EventCrash,
		Target: nodeID,
	})
}

// EnqueueRestart schedules a node restart at the given virtual time.
func (s *Simulator) EnqueueRestart(at t.Time, nodeID t.NodeID) {
	s.queue.Enqueue(&Event{
		At:   at,
		Kind: EventRestart,
		Target: nodeID,
	})
}

// EnqueuePartition schedules a network partition at the given virtual time.
func (s *Simulator) EnqueuePartition(at t.Time, groups [][]t.NodeID) {
	s.queue.Enqueue(&Event{
		At:        at,
		Kind:      EventPartition,
		Partition: groups,
	})
}

// EnqueueHeal schedules partition healing at the given virtual time.
func (s *Simulator) EnqueueHeal(at t.Time) {
	s.queue.Enqueue(&Event{
		At:   at,
		Kind: EventHeal,
	})
}

// EnqueueClientRequest schedules a client request at the given virtual time.
func (s *Simulator) EnqueueClientRequest(at t.Time, target t.NodeID, req t.ClientReq) {
	s.queue.Enqueue(&Event{
		At:        at,
		Kind:      EventClientReq,
		Target:    target,
		ClientReq: &req,
	})
}

// FindLeader returns the NodeID of the current leader, or 0 if none.
// Iterates in sorted order for determinism.
func (s *Simulator) FindLeader() t.NodeID {
	for _, id := range s.nodeIDs {
		snap := s.nodes[id].Snapshot()
		if snap.Alive && snap.Role == t.RoleLeader {
			return id
		}
	}
	return 0
}

// Trace returns the recorded trace events.
func (s *Simulator) Trace() []TraceEvent {
	return s.trace
}

// TraceJSON returns the trace as a pretty-printed JSON byte slice.
func (s *Simulator) TraceJSON() ([]byte, error) {
	return json.MarshalIndent(s.trace, "", "  ")
}

// NodeIDs returns the sorted list of node IDs.
func (s *Simulator) NodeIDs() []t.NodeID {
	return s.nodeIDs
}

// NodeSnapshot returns a snapshot of a specific node.
func (s *Simulator) NodeSnapshot(id t.NodeID) (t.NodeView, bool) {
	node, ok := s.nodes[id]
	if !ok {
		return t.NodeView{}, false
	}
	return node.Snapshot(), true
}

// Suppress unused import warning for sort.
var _ = sort.Strings
