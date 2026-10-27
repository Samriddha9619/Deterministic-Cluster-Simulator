package checkers

import (
	"fmt"
)

// CheckDeterminism runs a simulation twice with the exact same seed and compares trace hashes.
func CheckDeterminism(seed uint64, runFn func(seed uint64) (string, error)) *Violation {
	hash1, err := runFn(seed)
	if err != nil {
		return &Violation{
			Checker: "determinism",
			Detail:  fmt.Sprintf("seed %d run 1 failed: %v", seed, err),
		}
	}

	hash2, err := runFn(seed)
	if err != nil {
		return &Violation{
			Checker: "determinism",
			Detail:  fmt.Sprintf("seed %d run 2 failed: %v", seed, err),
		}
	}

	if hash1 != hash2 {
		return &Violation{
			Checker: "determinism",
			Detail: fmt.Sprintf("seed %d trace hash diverged between identical runs: %s vs %s",
				seed, hash1, hash2),
		}
	}

	return nil
}
