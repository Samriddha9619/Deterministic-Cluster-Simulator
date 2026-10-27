package sim

import (
	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// ---------------------------------------------------------------------------
// DiskConfig — how the simulated disk behaves
// ---------------------------------------------------------------------------

// DiskConfig controls the simulated disk's timing.
type DiskConfig struct {
	WriteLatency t.Time // simulated write completion delay
	SyncLatency  t.Time // simulated fsync completion delay
}

// DefaultDiskConfig returns a reasonable default: 0.5ms write, 2ms sync.
func DefaultDiskConfig() DiskConfig {
	return DiskConfig{
		WriteLatency: 500 * t.Microsecond,
		SyncLatency:  2 * t.Millisecond,
	}
}

// ---------------------------------------------------------------------------
// FakeDisk — simulated persistent storage for one node
// ---------------------------------------------------------------------------

// FakeDisk simulates a node's persistent storage with two layers:
//   - Volatile buffer: written but not synced (lost on crash).
//   - Durable image: synced data that survives crashes.
type FakeDisk struct {
	nodeID   t.NodeID
	config   DiskConfig
	rng      t.Rand
	durable  []byte // survives crash
	volatile []byte // lost on crash
	synced   bool
}

// NewFakeDisk creates a new simulated disk for the given node.
func NewFakeDisk(nodeID t.NodeID, config DiskConfig, rng t.Rand) *FakeDisk {
	return &FakeDisk{
		nodeID: nodeID,
		config: config,
		rng:    rng,
		synced: true,
	}
}

// DiskWriteResult tells the event loop when to schedule the completion callback.
type DiskWriteResult struct {
	CompletesAt t.Time
}

// Write stores data in the volatile buffer (not durable yet).
func (d *FakeDisk) Write(now t.Time, data []byte) DiskWriteResult {
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	d.volatile = dataCopy
	d.synced = false
	return DiskWriteResult{CompletesAt: now + d.config.WriteLatency}
}

// DiskSyncResult tells the event loop when to schedule the completion callback.
type DiskSyncResult struct {
	CompletesAt t.Time
}

// Sync makes all volatile data durable. After this, data survives crashes.
func (d *FakeDisk) Sync(now t.Time) DiskSyncResult {
	if d.volatile != nil {
		durableCopy := make([]byte, len(d.volatile))
		copy(durableCopy, d.volatile)
		d.durable = durableCopy
	}
	d.synced = true
	return DiskSyncResult{CompletesAt: now + d.config.SyncLatency}
}

// Crash wipes the volatile buffer. Only durable data survives.
func (d *FakeDisk) Crash() {
	d.volatile = nil
	d.synced = true
}

// DurableImage returns a safe copy of the durable state for OnRestart.
func (d *FakeDisk) DurableImage() t.DiskImage {
	if d.durable == nil {
		return t.DiskImage{Data: nil}
	}
	dataCopy := make([]byte, len(d.durable))
	copy(dataCopy, d.durable)
	return t.DiskImage{Data: dataCopy}
}

// HasUnsyncedData returns true if there are writes that haven't been synced.
func (d *FakeDisk) HasUnsyncedData() bool {
	return !d.synced
}

// SetDurable directly sets the durable image (used during initial setup).
func (d *FakeDisk) SetDurable(data []byte) {
	if data == nil {
		d.durable = nil
		return
	}
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	d.durable = dataCopy
}
