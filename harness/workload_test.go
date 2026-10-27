package harness

import (
	"reflect"
	"testing"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

type mockRand struct {
	state uint64
}

func newMockRand(seed uint64) *mockRand {
	if seed == 0 {
		seed = 0xdeadbeef
	}
	return &mockRand{state: seed}
}

func (r *mockRand) Uint64() uint64 {
	r.state ^= r.state >> 12
	r.state ^= r.state << 25
	r.state ^= r.state >> 27
	return r.state * 2685821657736338717
}

func (r *mockRand) Intn(n int) int {
	return int(r.Uint64() % uint64(n))
}

func (r *mockRand) Float64() float64 {
	return float64(r.Uint64()&(1<<53-1)) / (1 << 53)
}

func (r *mockRand) Fork(label string) t.Rand {
	return newMockRand(r.Uint64())
}

func TestGenerateWorkloadDeterminism(tst *testing.T) {
	nodeIDs := []t.NodeID{t.NodeID(1), t.NodeID(2), t.NodeID(3)}
	cfg := DefaultWorkloadConfig()
	cfg.NumOps = 30

	r1 := newMockRand(42)
	reqs1 := GenerateWorkload(r1, nodeIDs, cfg)

	r2 := newMockRand(42)
	reqs2 := GenerateWorkload(r2, nodeIDs, cfg)

	if len(reqs1) != 30 || len(reqs2) != 30 {
		tst.Fatalf("expected 30 ops, got %d and %d", len(reqs1), len(reqs2))
	}

	if !reflect.DeepEqual(reqs1, reqs2) {
		tst.Fatalf("workload generation is non-deterministic for identical seeds")
	}

	r3 := newMockRand(999)
	reqs3 := GenerateWorkload(r3, nodeIDs, cfg)
	if reflect.DeepEqual(reqs1, reqs3) {
		tst.Fatalf("different seeds should produce different workloads")
	}
}

func TestHistoryRecorder(tst *testing.T) {
	hr := NewHistoryRecorder()

	hr.RecordInvoke(1*t.Second, t.ClientReq{
		RequestID: 1,
		ClientID:  1,
		Op:        t.OpPut,
		Key:       "foo",
		Value:     "bar",
	})

	hr.RecordReturn(2*t.Second, t.ClientResp{
		RequestID: 1,
		OK:        true,
	})

	// Unanswered request
	hr.RecordInvoke(3*t.Second, t.ClientReq{
		RequestID: 2,
		ClientID:  2,
		Op:        t.OpGet,
		Key:       "foo",
	})

	hr.FinalizeIndeterminate(10 * t.Second)

	entries := hr.Entries()
	if len(entries) != 4 {
		tst.Fatalf("expected 4 entries, got %d", len(entries))
	}

	if entries[0].Kind != KindInvoke || entries[1].Kind != KindOK || entries[2].Kind != KindInvoke || entries[3].Kind != KindInfo {
		tst.Fatalf("unexpected entry kinds: %v", entries)
	}
}
