# Deterministic Cluster Simulator

A deterministic simulation and testing engine for a distributed, replicated key-value store powered by Raft consensus (`etcd/raft`).

Inspired by FoundationDB's deterministic simulation testing and Jepsen's black-box fault injection, this project implements a dual-world architecture:
- **World A (Simulation)**: Simulated virtual clock, network topology, non-volatile disks, and pseudo-randomness, executing entirely single-threaded with zero ambient non-determinism. Runs hundreds of seeds per second, reproducibly finds distributed bugs, and automatically minimizes failing traces using delta-debugging schedule shrinking.
- **World B (Real Processes)**: The identical node binary executing as independent Linux OS processes communicating over real TCP sockets and durable `fsync` storage, attacked by an external chaos orchestrator.

---

## M5 Bug Gallery & Benchmark Table

Every invariant checker has a dedicated planted bug to prove non-vacuous correctness verification. The table below records the measured seeds required to produce a violation and the delta-debugging reduction achieved by the shrink engine:

| Bug ID | Planted Bug | Target Checker | Seeds to Fail | Shrunk Faults | Shrunk Ops | Iterations | Time |
|---|---|---|---|---|---|---|---|
| `ack_before_fsync` | Ack Before Fsync | `durability` | 1 (seed 1) | 7 → **1** | 25 → **1** | 14 | 445ms |
| `wrong_quorum` | Wrong Quorum | `one_leader_per_term` | 4 (seed 4) | 11 → **1** | 25 → **1** | 12 | 328ms |
| `stale_term_accepted` | Stale Term Accepted | `log_matching` | 1 (seed 1) | 7 → **1** | 25 → **1** | 6 | 20ms |
| `unpersisted_vote` | Vote Without Persistence | `one_leader_per_term` | 1 (seed 1) | 7 → **1** | 25 → **1** | 8 | 184ms |
| `early_apply` | Apply Before Commit | `convergence` | 2 (seed 2) | 9 → **1** | 25 → **1** | 12 | 359ms |
| `stale_read` | Stale Read | `linearizability` | 49 (seed 49) | 11 → **2** | 25 → **2** | 27 | 2.525s |

### Benchmark Highlights
- **100% Checker Coverage**: All 6 planted defects trigger their designated safety checkers.
- **Aggressive Fault Minimization**: Complex multi-fault chaos plans with dozens of crashes, restarts, and partitions are shrunk to just **1–2 isolated fault actions**.
- **Minimal Workload Operations**: Client request sequences (25+ operations) are minimized down to **1–2 key operations** that expose the violation.
- **Sub-Second Shrinking**: Most bugs are localized and minimized in under 500ms using Andreas Zeller's `ddmin` delta-debugging algorithm.

---

## Invariant Checkers

1. **Durability (`checkers/durability.go`)**: Verifies that once a write operation returns `OK` to a client, it is never lost or reverted across node crashes, disk syncs, or network partitions.
2. **One Leader Per Term (`checkers/checkers.go`)**: Enforces Raft's fundamental safety invariant that at most one node can act as leader in any given term.
3. **Log Matching (`checkers/raft_safety.go`)**: Verifies that if two nodes share an entry with identical index and term, their logs are identical across all preceding entries.
4. **Convergence (`checkers/checkers.go`)**: Verifies that all surviving cluster nodes converge to identical key-value states during quiescence.
5. **Linearizability (`checkers/linearizability.go`)**: Executes the Wing & Gong concurrent search algorithm over the recorded history of client calls and returns, modeling indeterminate in-flight operations.
6. **Determinism (`checkers/determinism.go`)**: Validates that repeated simulation runs with identical seeds produce byte-for-byte identical event traces and cryptographic SHA-256 hashes.

---

## Running the Simulator

### Run a Seed Sweep
```bash
# Run 100 seeds across 8 CPU workers with determinism verification
go run ./cmd/m0spike -seeds 100 -parallel 8 -det -faults 5 -workload
```

### Delta-Debugging Trace Shrinking
```bash
# Shrink a failing seed to minimal fault actions and operations
go run ./cmd/m0spike -shrink 4 -bug wrong_quorum
```

### Run Bug Gallery Benchmark Suite
```bash
go test -v ./bugs/...
```

### Start Web UI & Simulation Server
```bash
go run ./cmd/server/main.go -port 8080 -static ./web
```
Visit `http://localhost:8080` to inspect node topology, scrub virtual timelines, and run interactive sweeps.
