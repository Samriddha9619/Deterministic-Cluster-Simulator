package sim

import (
	"testing"

	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

func TestSweepRunnerCleanControl(tst *testing.T) {
	cfg := SweepConfig{
		StartSeed:        100,
		Count:            10,
		Concurrency:      4,
		NumNodes:         3,
		DropRate:         0.02,
		Faults:           2,
		Workload:         true,
		NumOps:           20,
		CheckDeterminism: true,
		MaxTimeSec:       15,
	}

	report := RunSweep(cfg)
	if report.Total != 10 {
		tst.Fatalf("expected 10 total runs, got %d", report.Total)
	}
	if report.Passed != 10 {
		tst.Fatalf("expected all 10 clean runs to pass, got %d passed, %d failed: %v",
			report.Passed, report.Failed, report.Failures)
	}
	if report.DetFailed != 0 {
		tst.Fatalf("expected 0 determinism failures, got %d", report.DetFailed)
	}
	if len(report.Failures) != 0 {
		tst.Fatalf("expected 0 failures in report, got %d", len(report.Failures))
	}
}

func TestSweepRunnerCatchesPlantedBug(tst *testing.T) {
	cfg := SweepConfig{
		StartSeed:        1,
		Count:            5,
		Concurrency:      2,
		NumNodes:         3,
		DropRate:         0.0,
		Faults:           10,
		Workload:         true,
		NumOps:           25,
		CheckDeterminism: false,
		MaxTimeSec:       15,
		Bugs: t.BugFlags{
			AckBeforeFsync: true,
		},
	}

	report := RunSweep(cfg)
	if report.Failed == 0 {
		tst.Fatalf("expected planted AckBeforeFsync bug to fail at least one seed, got 0 failures out of %d", report.Total)
	}
	if len(report.Failures) == 0 {
		tst.Fatalf("expected non-empty Failures list when bugs occur")
	}
	tst.Logf("Caught %d failures out of %d (first: %s)", report.Failed, report.Total, report.Failures[0].Violation)
}
