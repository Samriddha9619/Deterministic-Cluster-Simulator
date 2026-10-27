package checkers

import (
	"fmt"
	"math"
	"sort"

	"github.com/Samriddha9619/deterministic-cluster-simulator/harness"
	t "github.com/Samriddha9619/deterministic-cluster-simulator/types"
)

// LinearOp represents a paired client operation with invocation and completion times.
type LinearOp struct {
	ID        uint64
	ClientID  uint64
	Op        harness.OpType
	Key       string
	Value     string
	OldValue  string
	OK        bool
	InvokeAt  t.Time
	ReturnAt  t.Time // math.MaxInt64 if indeterminate
}

// CheckLinearizability verifies that a recorded execution history can be linearized
// to a sequential execution of independent key-value registers.
func CheckLinearizability(entries []harness.HistoryEntry) *Violation {
	// Pair up invokes and returns
	paired := make(map[uint64]*LinearOp)
	var allOps []*LinearOp

	for _, e := range entries {
		switch e.Kind {
		case harness.KindInvoke:
			op := &LinearOp{
				ID:       e.RequestID,
				ClientID: e.ClientID,
				Op:       e.Op,
				Key:      e.Key,
				Value:    e.Value,
				OldValue: e.OldValue,
				InvokeAt: e.At,
				ReturnAt: t.Time(math.MaxInt64), // default until return
			}
			paired[e.RequestID] = op
			allOps = append(allOps, op)

		case harness.KindOK, harness.KindFail:
			if op, exists := paired[e.RequestID]; exists {
				op.ReturnAt = e.At
				op.OK = (e.Kind == harness.KindOK)
				if op.Op == harness.OpGet {
					op.Value = e.Value
				}
			}

		case harness.KindInfo:
			// Indeterminate — remains with ReturnAt = MaxInt64
		}
	}

	// Group operations by key
	byKey := make(map[string][]*LinearOp)
	for _, op := range allOps {
		byKey[op.Key] = append(byKey[op.Key], op)
	}

	// Sort keys for deterministic checking order
	sortedKeys := make([]string, 0, len(byKey))
	for k := range byKey {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	for _, k := range sortedKeys {
		ops := byKey[k]
		if violation := linearizeKey(k, ops); violation != nil {
			return violation
		}
	}

	return nil
}

// linearizeKey runs the Wing & Gong search algorithm on a single key's operations.
func linearizeKey(key string, ops []*LinearOp) *Violation {
	n := len(ops)
	if n == 0 {
		return nil
	}

	// Precompute precedence: mustPrecede[i][j] is true if ops[i] must execute before ops[j]
	// (i.e. ops[i] completed before ops[j] was invoked).
	mustPrecede := make([][]bool, n)
	for i := 0; i < n; i++ {
		mustPrecede[i] = make([]bool, n)
		for j := 0; j < n; j++ {
			if i != j && ops[i].ReturnAt < ops[j].InvokeAt {
				mustPrecede[i][j] = true
			}
		}
	}

	memo := make(map[string]bool)

	var search func(mask uint64, state string) bool
	search = func(mask uint64, state string) bool {
		if mask == (uint64(1)<<n)-1 {
			return true
		}

		memoKey := fmt.Sprintf("%d:%s", mask, state)
		if seen, ok := memo[memoKey]; ok {
			return seen
		}

		// Find candidates that can be executed next
		for i := 0; i < n; i++ {
			if (mask & (uint64(1) << i)) != 0 {
				continue // already linearized
			}

			// Precedence check: any un-linearized op that MUST precede i?
			blocked := false
			for j := 0; j < n; j++ {
				if (mask&(uint64(1)<<j)) == 0 && mustPrecede[j][i] {
					blocked = true
					break
				}
			}
			if blocked {
				continue
			}

			// Try applying op i to state
			op := ops[i]

			// If operation was indeterminate, explore the branch where it was dropped and never committed
			if op.ReturnAt == t.Time(math.MaxInt64) {
				if search(mask|(uint64(1)<<i), state) {
					memo[memoKey] = true
					return true
				}
			}

			nextState, valid := stepRegister(state, op)
			if valid {
				if search(mask|(uint64(1)<<i), nextState) {
					memo[memoKey] = true
					return true
				}
			}
		}

		memo[memoKey] = false
		return false
	}

	if search(0, "") {
		return nil
	}

	return &Violation{
		Checker: "linearizability",
		Detail: fmt.Sprintf("key %q operations cannot be linearized under sequential consistency (ops count=%d)",
			key, n),
	}
}

// stepRegister computes the next register state and whether the observed result is valid.
func stepRegister(state string, op *LinearOp) (string, bool) {
	// If the operation explicitly failed (propose failed, dropped, rejected):
	if !op.OK {
		if op.Op == harness.OpCAS {
			// A CAS that failed could be due to value mismatch or proposal failure
			return state, true
		}
		// Failed Get, Put, or Delete does not mutate state
		return state, true
	}

	switch op.Op {
	case harness.OpPut:
		return op.Value, true

	case harness.OpDelete:
		return "", true

	case harness.OpGet:
		// Successful read must observe current state
		if op.Value == state {
			return state, true
		}
		return state, false

	case harness.OpCAS:
		if state == op.OldValue {
			return op.Value, true
		}
		return state, false

	default:
		return state, true
	}
}
