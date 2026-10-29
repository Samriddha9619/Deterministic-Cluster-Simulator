package bugs

import (
	"testing"
)

func TestAllGalleryBugsCatchViolationsAndShrink(tst *testing.T) {
	bugs := AllBugs()
	if len(bugs) != 6 {
		tst.Fatalf("expected 6 gallery bugs, got %d", len(bugs))
	}

	var results []*BenchmarkResult

	for _, b := range bugs {
		tst.Run(b.ID, func(t *testing.T) {
			res, err := RunBugBenchmark(b, 60)
			if err != nil {
				t.Fatalf("benchmark failed for bug %s: %v", b.ID, err)
			}

			t.Logf("Bug %s (%s): seed %d failed on %s", b.ID, b.Name, res.FirstFailingSeed, res.InitialViolation)
			t.Logf("Shrunk: %s in %d iterations (%s)", res.ShrunkSummary, res.Iterations, res.Duration)

			if res.ShrunkFaults > res.OriginalFaults {
				t.Errorf("expected faults to shrink or stay equal: %d > %d", res.ShrunkFaults, res.OriginalFaults)
			}
			if res.FinalViolation == "" {
				t.Errorf("expected final violation not empty")
			}

			results = append(results, res)
		})
	}

	tst.Log("\n" + GenerateMarkdownTable(results))
}
