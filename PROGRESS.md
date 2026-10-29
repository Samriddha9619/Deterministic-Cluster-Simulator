# Deterministic Cluster Simulator — Progress Tracker

## Milestone Status Overview

| Milestone | Title | Status | Description |
|---|---|---|---|
| **M0** | Feasibility Spike | **DONE** | 3-node etcd/raft cluster, seeded event loop, network drops & partition, leader-per-term & convergence checks, patched library randomness. |
| **M1** | Skeleton & First Visual | **DONE** | Trace JSON emitter, Go HTTP server, interactive cluster canvas with timeline scrubber & node inspector. |
| **M2** | Faults & Disk Subsystem | **DONE** | Network fault model (asymmetry, delays), crash/restart, volatile vs durable disk buffer with sync, upfront FaultPlan, BUGGIFY macro. |
| **M3** | Replicated KV & Complete Checkers | **DONE** | Production Ready ordering, KV state machine, snapshotting & log compaction, client workload generator, history recorder, 6 checkers. |
| **M4** | Sweep & Shrink Engine | **DONE** | Parallel seed sweep runner, violation freeze, delta-debugging schedule shrinker. |
| **M5** | Bug Gallery | **DONE** | 6 planted bugs behind flags (ack before fsync, wrong quorum, stale term, unpersisted vote, early apply, stale read) + benchmarks. |
| **M6** | Real Runtime (`/runtime/real`) | **NEXT** | `simnode` binary, real TCP framing, append-only WAL with `fsync`, process manager (SIGKILL, SIGSTOP, restart). |
| **M7** | Jepsen-Style Real Harness | Pending | TCP fault proxy, external nemesis fault orchestrator, 4-phase chaos workload, history cross-check. |
| **M8** | Sim-vs-Real Study | Pending | Model-gap discovery, real fault schedule cross-replay in simulator, comparative evaluation. |
| **M9** | Stretch Goals | Pending | In-browser WASM compilation, custom minimal Raft implementation. |

---

## M0 Details & Verification

- [x] **Election Timeout Randomness Fix (Rule 7)**: Patched vendored `go.etcd.io/raft/v3` to inject a `Rand` interface into `raft.Config`, driving `r.rand.Intn` per node instead of ambient `crypto/rand`.
- [x] **Tick Scheduler Fix**: Removed redundant `SetTimer` invocations in `drainReady` preventing exponential tick amplification.
- [x] **Proposal Forwarding**: Enabled followers to forward client proposals to the cluster leader via Raft messages.
- [x] **Checkers**: Implemented `CheckOneLeaderPerTerm` and `CheckConvergence` in `/checkers`.
- [x] **Determinism Verification**: 100/100 seeds tested twice producing identical trace hashes (`Det-Fail: 0`).

---

## M1 Details & Verification

- [x] **Trace JSON Emitter**: Implemented `harness/trace.go` with full metadata, per-event post-dispatch cluster snapshots, and verdicts.
- [x] **Web & Simulation Server**: Implemented `server/server.go` and `cmd/server/main.go` serving static assets and REST endpoints (`/api/simulate`, `/api/sweep`, `/api/health`).
- [x] **Frontend Cluster Visualizer**: Implemented `web/index.html`, `web/styles.css`, and `web/app.js` featuring canvas topology, instant timeline scrubber, node inspector, and seed sweep drawer.
- [x] **End-to-End Browser Validation**: Tested and verified in browser subagent with WebP session recording saved.

---

## M2 Details & Verification

- [x] **Fault Plan & Shrinking Foundation**: Implemented `sim/faults.go` with `FaultKind`, `FaultAction`, `FaultPlan` (ordered, cloneable, serializable, with `WithoutAction` for M4 delta-debugging).
- [x] **Buggify Macro**: Implemented `sim/buggify.go` following FoundationDB `BUGGIFY` pattern with stable site names and activation recorder.
- [x] **Network Faults**: Added asymmetric link cuts/restores (`sim/network.go`, `sim/event.go`), delay bounds, drop rates, and packet duplication modeling.
- [x] **Disk Subsystem & Crash-Recovery**: Built crash recovery with volatile vs durable disk state in `node/raftnode/adapter.go`. On restart, completely wipes volatile RAM, reconstructs `RawNode` strictly from durable disk storage image, verifies HardState and WAL persistence.
- [x] **Determinism Verification under Chaos**: Tested 50 seeds with multi-fault plans (crashes, restarts, partitions, drops); 50/50 passed with 100% identical double-run trace SHA-256 hashes (`Det-Fail: 0`).

---

## M3 Details & Verification

- [x] **Replicated KV State Machine (`node/kv/kv.go`)**: Implemented full KV store supporting `Put`, `Get`, `Delete`, and `CAS` with client deduplication sessions (`ClientID`, `LastReqID`, `LastResult`). Re-applied commands return cached responses without duplicate side effects.
- [x] **Snapshotting & Log Compaction (`node/raftnode/adapter.go`)**: Implemented snapshot generation (`CreateSnapshot`, `Compact`) upon reaching configurable threshold, applied incoming snapshots from leader, and restored state machine and client sessions across crash-recovery.
- [x] **Workload Generator & History Recorder (`harness/`)**: Implemented Jepsen-style history logger (`history.go`) capturing `Invoke`, `OK`, `Fail`, and indeterminate `Info` events. Built configurable workload generator (`workload.go`) supporting concurrent client workers, hot-key access skew, and operation mixtures.
- [x] **All 6 Invariant Checkers (`checkers/`)**:
  1. *Oracle State Checker*: compares final KV state with sequential committed write model.
  2. *Convergence Checker*: verifies identical KV state across all live nodes at quiescence.
  3. *Raft Safety Checkers*: online per-event verification of single leader per term, log prefix matching, and leader completeness.
  4. *Durability Checker*: verifies all acknowledged client writes survive subsequent crashes and partitions.
  5. *Linearizability Checker*: exact Wing & Gong concurrent register search algorithm handling indeterminate in-flight branches.
  6. *Determinism Checker*: byte-for-byte SHA-256 trace verification across repeated executions.
- [x] **End-to-End Stress Verification**: 100/100 seeds tested twice with concurrent workload, multi-action fault injection (crashes, restarts, network partitions), snapshotting, and log compaction: 100/100 passed with 0 invariant violations and 0 determinism failures (`Det-Fail: 0`).

---

## M4 Details & Verification

- [x] **Parallel Seed Runner (`sim/sweep.go`)**: Multi-core worker pool executing independent deterministic simulations in parallel. Achieved **76–200 seeds/sec** across CPU cores with full double-run determinism verification (`Det-Fail: 0`).
- [x] **Delta-Debugging Schedule Shrinker (`sim/shrink.go`)**: Implemented Andreas Zeller's `ddmin` algorithm over both fault plans and client workload operations. Proved reduction from 9 faults and 25 client ops down to 2 faults and 3 ops (in 39 iterations, 0.15s) while preserving exact linearizability and durability invariant violations.
- [x] **Planted Bug Framework (`types/types.go`, `node/raftnode/adapter.go`)**: Planted bug flags (`AckBeforeFsync`, `EarlyApply`, `UnpersistedVote`, etc.) integrated into `Config`, allowing reliable defect injection and shrinker evaluation.
- [x] **API & CLI Integration (`server/server.go`, `cmd/m0spike/main.go`)**:
  - `/api/sweep`: Multi-seed parallel execution with fast JSON response summaries.
  - `/api/shrink`: REST endpoint returning minimal fault actions, reduced client operations, and frozen visual trace.
  - CLI flags: `-parallel W` for multi-worker sweeps and `-shrink <seed>` for instant terminal delta-debugging.

---

## M5 Details & Verification

- [x] **Catalog of All 6 Planted Bugs (`bugs/gallery.go`, `node/raftnode/adapter.go`)**:
  1. `ack_before_fsync`: Responds OK to client before log entries and hard state are flushed to disk → trips `durability` checker.
  2. `wrong_quorum`: Counts a minority as a quorum during partition → trips `one_leader_per_term` checker.
  3. `stale_term_accepted`: Accepts messages from lower terms without rejection → trips `log_matching` checker.
  4. `unpersisted_vote`: Forgets votedFor across node restart → trips `one_leader_per_term` checker.
  5. `early_apply`: Applies speculative uncommitted entries to state machine → trips `convergence` checker.
  6. `stale_read`: Serves Get requests from local memory without consensus/lease → trips `linearizability` checker.
- [x] **Planted Bug Benchmark Table (`README.md`, `bugs/gallery_test.go`)**:
  
  | Bug ID | Planted Bug | Target Checker | Seeds to Fail | Shrunk Faults | Shrunk Ops | Iterations | Time |
  |---|---|---|---|---|---|---|---|
  | `ack_before_fsync` | Ack Before Fsync | `durability` | 1 (seed 1) | 7 → **1** | 25 → **1** | 14 | 445ms |
  | `wrong_quorum` | Wrong Quorum | `one_leader_per_term` | 4 (seed 4) | 11 → **1** | 25 → **1** | 12 | 328ms |
  | `stale_term_accepted` | Stale Term Accepted | `log_matching` | 1 (seed 1) | 7 → **1** | 25 → **1** | 6 | 20ms |
  | `unpersisted_vote` | Vote Without Persistence | `one_leader_per_term` | 1 (seed 1) | 7 → **1** | 25 → **1** | 8 | 184ms |
  | `early_apply` | Apply Before Commit | `convergence` | 2 (seed 2) | 9 → **1** | 25 → **1** | 12 | 359ms |
  | `stale_read` | Stale Read | `linearizability` | 49 (seed 49) | 11 → **2** | 25 → **2** | 27 | 2.525s |

- [x] **Restart Membership Fix**: Fixed `adapter.go` on restart to preserve cluster `ConfState` in `MemoryStorage`, ensuring recovered nodes correctly re-establish consensus and quorum.
- [x] **Log Storage Entries Full Scan**: Fixed `n.storage.Entries` in `adapter.go` to use `math.MaxUint64` instead of `0` so all log entries are returned for online log matching verification.
- [x] **Clean Control Baseline Verification**: Ran 20 clean control seeds with chaos faults and concurrent workload with 100% pass rate (`0 failures`).

