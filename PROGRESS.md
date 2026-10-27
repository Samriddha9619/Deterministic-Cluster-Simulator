# Deterministic Cluster Simulator — Progress Tracker

## Milestone Status Overview

| Milestone | Title | Status | Description |
|---|---|---|---|
| **M0** | Feasibility Spike | **DONE** | 3-node etcd/raft cluster, seeded event loop, network drops & partition, leader-per-term & convergence checks, patched library randomness. |
| **M1** | Skeleton & First Visual | **DONE** | Trace JSON emitter, Go HTTP server, interactive cluster canvas with timeline scrubber & node inspector. |
| **M2** | Faults & Disk Subsystem | **DONE** | Network fault model (asymmetry, delays), crash/restart, volatile vs durable disk buffer with sync, upfront FaultPlan, BUGGIFY macro. |
| **M3** | Replicated KV & Complete Checkers | **NEXT** | Production Ready ordering, KV state machine, snapshotting & log compaction, client workload generator, history recorder, 6 checkers. |
| **M4** | Sweep & Shrink Engine | Pending | Parallel seed sweep runner, violation freeze, delta-debugging schedule shrinker. |
| **M5** | Bug Gallery | Pending | 6 planted bugs behind flags (ack before fsync, wrong quorum, stale term, unpersisted vote, early apply, stale read) + benchmarks. |
| **M6** | Real Runtime (`/runtime/real`) | Pending | `simnode` binary, real TCP framing, append-only WAL with `fsync`, process manager (SIGKILL, SIGSTOP, restart). |
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
