package sim

import (
	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// ---------------------------------------------------------------------------
// LinkConfig — per-link network behavior
// ---------------------------------------------------------------------------

// LinkConfig describes the simulated behavior of a single directed network
// link (from → to).
type LinkConfig struct {
	MinLatency t.Time
	MaxLatency t.Time
	DropRate   float64 // probability [0.0, 1.0] of silent message drop
}

// DefaultLinkConfig returns a reasonable default: 1-10ms latency, no drops.
func DefaultLinkConfig() LinkConfig {
	return LinkConfig{
		MinLatency: 1 * t.Millisecond,
		MaxLatency: 10 * t.Millisecond,
		DropRate:   0.0,
	}
}

// ---------------------------------------------------------------------------
// Network — the simulated network
// ---------------------------------------------------------------------------

// Network simulates the cluster's network. Key properties:
//   - Messages are deep-copied on send (nodes never share memory).
//   - Partitions affect delivery, not sending (nodes don't know they're
//     partitioned until messages fail to arrive).
//   - Messages to crashed nodes are dropped at delivery time.
//   - Per-link latency/drop config for asymmetric network modeling.
type Network struct {
	rng             t.Rand
	links           map[linkKey]LinkConfig
	partitionGroups [][]t.NodeID
	aliveNodes      map[t.NodeID]bool
}

type linkKey struct {
	from, to t.NodeID
}

// NewNetwork creates a new simulated network.
func NewNetwork(rng t.Rand, nodeIDs []t.NodeID) *Network {
	alive := make(map[t.NodeID]bool, len(nodeIDs))
	for _, id := range nodeIDs {
		alive[id] = true
	}
	return &Network{
		rng:        rng,
		links:      make(map[linkKey]LinkConfig),
		aliveNodes: alive,
	}
}

// SetLinkConfig overrides the default link behavior for a (from, to) pair.
func (n *Network) SetLinkConfig(from, to t.NodeID, cfg LinkConfig) {
	n.links[linkKey{from, to}] = cfg
}

// GetLinkConfig returns the link config for a (from, to) pair.
func (n *Network) GetLinkConfig(from, to t.NodeID) LinkConfig {
	if cfg, ok := n.links[linkKey{from, to}]; ok {
		return cfg
	}
	return DefaultLinkConfig()
}

// SetPartition installs a network partition. Nodes in different groups
// cannot communicate.
func (n *Network) SetPartition(groups [][]t.NodeID) {
	n.partitionGroups = groups
}

// Heal removes the current partition, restoring full connectivity.
func (n *Network) Heal() {
	n.partitionGroups = nil
}

// IsPartitioned returns true if from and to are in different partition groups.
func (n *Network) IsPartitioned(from, to t.NodeID) bool {
	if len(n.partitionGroups) == 0 {
		return false
	}
	fromGroup := n.groupOf(from)
	toGroup := n.groupOf(to)
	if fromGroup == -1 || toGroup == -1 {
		return true
	}
	return fromGroup != toGroup
}

func (n *Network) groupOf(id t.NodeID) int {
	for i, group := range n.partitionGroups {
		for _, member := range group {
			if member == id {
				return i
			}
		}
	}
	return -1
}

// CrashNode marks a node as dead (messages to it are dropped).
func (n *Network) CrashNode(id t.NodeID) {
	n.aliveNodes[id] = false
}

// RestartNode marks a node as alive again.
func (n *Network) RestartNode(id t.NodeID) {
	n.aliveNodes[id] = true
}

// IsAlive returns whether the given node is currently alive.
func (n *Network) IsAlive(id t.NodeID) bool {
	return n.aliveNodes[id]
}

// ---------------------------------------------------------------------------
// Send — the main entry point for message delivery decisions
// ---------------------------------------------------------------------------

// SendResult describes what happened when a message was sent.
type SendResult struct {
	Delivered  bool
	DeliverAt  t.Time
	DropReason string
}

// Send decides whether/when to deliver a message. It does NOT enqueue the
// delivery event — the event loop does that based on the returned SendResult.
func (n *Network) Send(now t.Time, env t.Envelope) SendResult {
	if !n.aliveNodes[env.From] {
		return SendResult{Delivered: false, DropReason: "sender dead"}
	}
	if !n.aliveNodes[env.To] {
		return SendResult{Delivered: false, DropReason: "receiver dead"}
	}
	if n.IsPartitioned(env.From, env.To) {
		return SendResult{Delivered: false, DropReason: "partitioned"}
	}
	cfg := n.GetLinkConfig(env.From, env.To)
	if cfg.DropRate > 0 && BoolWithProb(n.rng, cfg.DropRate) {
		return SendResult{Delivered: false, DropReason: "random drop"}
	}
	latency := DurationBetween(n.rng, cfg.MinLatency, cfg.MaxLatency)
	return SendResult{
		Delivered: true,
		DeliverAt: now + latency,
	}
}

// CopyEnvelope creates a deep copy of an envelope so nodes never share memory.
func CopyEnvelope(env t.Envelope) t.Envelope {
	bodyCopy := make([]byte, len(env.Body))
	copy(bodyCopy, env.Body)
	return t.Envelope{
		From: env.From,
		To:   env.To,
		Body: bodyCopy,
	}
}
