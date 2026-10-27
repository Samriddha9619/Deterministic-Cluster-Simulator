package raftnode

import (
	"fmt"
	"testing"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

type testRand struct{}

func (testRand) Uint64() uint64        { return 42 }
func (testRand) Intn(n int) int        { return 0 }
func (testRand) Float64() float64      { return 0.5 }
func (testRand) Fork(label string) t.Rand { return testRand{} }

func TestSnapshotAndLogCompaction(tst *testing.T) {
	rng := testRand{}
	cfg := Config{
		ID:                t.NodeID(1),
		Peers:             []t.NodeID{t.NodeID(1)},
		ElectionTick:      10,
		HeartbeatTick:     2,
		TickInterval:      100 * t.Millisecond,
		Rand:              rng,
		SnapshotThreshold: 5, // Compact every 5 applied entries
	}

	node, err := NewRaftNode(cfg)
	if err != nil {
		tst.Fatalf("failed to create node: %v", err)
	}

	drainDisk := func(now t.Time, out t.Output) {
		for len(out.DiskOps) > 0 {
			dops := out.DiskOps
			out.DiskOps = nil
			for _, dop := range dops {
				nextOut := node.OnDiskDone(now, t.DiskOpResult{
					OpID: dop.OpID,
					OK:   true,
				})
				out.DiskOps = append(out.DiskOps, nextOut.DiskOps...)
			}
		}
	}

	// Tick to become leader in single-node cluster
	for i := 0; i < 15; i++ {
		now := t.Time(i) * 100 * t.Millisecond
		out := node.OnTick(now)
		drainDisk(now, out)
	}

	// Apply 12 client puts
	for i := 1; i <= 12; i++ {
		now := 2*t.Second + t.Time(i)*(100*t.Millisecond)
		out := node.OnClient(now, t.ClientReq{
			RequestID: uint64(i),
			ClientID:  1,
			Op:        t.OpPut,
			Key:       fmt.Sprintf("k-%d", i),
			Value:     fmt.Sprintf("v-%d", i),
		})
		drainDisk(now, out)
	}

	// Check that snapshot was taken and compacted
	firstIdx, err := node.storage.FirstIndex()
	if err != nil {
		tst.Fatalf("failed to get first index: %v", err)
	}

	tst.Logf("appliedIndex=%d lastSnapshotIndex=%d firstIdx=%d", node.appliedIndex, node.lastSnapshotIndex, firstIdx)

	// Since we applied 12 entries and threshold is 5, compaction must have advanced firstIndex > 1
	if firstIdx <= 1 {
		tst.Fatalf("expected log compaction to advance first index > 1, got %d", firstIdx)
	}

	snap, err := node.storage.Snapshot()
	if err != nil {
		tst.Fatalf("failed to get storage snapshot: %v", err)
	}
	if snap.Metadata.GetIndex() == 0 {
		tst.Fatalf("expected non-zero snapshot index, got 0")
	}

	// Verify durable state has snapshot
	if node.durableState.Snapshot == nil {
		tst.Fatalf("expected durableState.Snapshot to be set")
	}
}
