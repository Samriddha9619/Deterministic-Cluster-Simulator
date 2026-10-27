# Deterministic Cluster Simulator — Progress Tracker

## Milestone Status Overview

| Milestone | Title | Status | Description |
|---|---|---|---|
| **M0** | Feasibility Spike | **DONE** | 3-node etcd/raft cluster, seeded event loop, network drops & partition, leader-per-term & convergence checks, patched library randomness. |
| **M1** | Skeleton & First Visual | **NEXT** | Trace JSON emitter, Go HTTP/WebSocket trace server, minimal TypeScript cluster canvas with timeline scrubber. |
| **M2** | Faults & Disk Subsystem | Pending | Network fault model (asymmetry, delays), crash/restart, volatile vs durable disk buffer with sync, fault plan. |
| **M3** | Replicated KV & Complete Checkers | Pending | Production Ready ordering, KV state machine, snapshotting & log compaction, client workload generator, history recorder, 6 checkers. |
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
