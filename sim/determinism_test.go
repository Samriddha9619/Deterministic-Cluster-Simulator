package sim

import (
	"fmt"
	"testing"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

func runSim(seed uint64, autoFaults int) RunResult {
	cfg := DefaultSimConfig(seed)
	cfg.MaxTime = 15 * t.Second
	cfg.AutoFaults = autoFaults

	linkCfg := DefaultLinkConfig()
	linkCfg.DropRate = 0.05
	cfg.LinkConfig = &linkCfg

	s, err := NewSimulator(cfg)
	if err != nil {
		panic(err)
	}

	for i := 0; i < 20; i++ {
		putTime := 2*t.Second + t.Time(i)*(500*t.Millisecond)
		s.EnqueueClientRequest(putTime, t.NodeID(1), t.ClientReq{
			RequestID: uint64(i + 1),
			ClientID:  1,
			Op:        t.OpPut,
			Key:       fmt.Sprintf("k-%d", i),
			Value:     fmt.Sprintf("v-%d", i),
		})
	}

	return s.Run()
}

func TestSimulatorDeterminismWithFaults(tst *testing.T) {
	for seed := uint64(1); seed <= 10; seed++ {
		res1 := runSim(seed, 3)
		res2 := runSim(seed, 3)

		if res1.TraceHash != res2.TraceHash {
			tst.Fatalf("seed %d: determinism failed! hash1=%s hash2=%s",
				seed, res1.TraceHash, res2.TraceHash)
		}

		if res1.EventCount != res2.EventCount {
			tst.Fatalf("seed %d: event count mismatch! %d vs %d",
				seed, res1.EventCount, res2.EventCount)
		}
	}
}
