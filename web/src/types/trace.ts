export interface NodeView {
  ID: number;
  Role: number;
  Term: number;
  VotedFor: number;
  LeaderID: number;
  CommitIndex: number;
  AppliedIndex: number;
  LogLength: number;
  KVState?: Record<string, string>;
  Alive: boolean;
}

export interface TraceEvent {
  index: number;
  at_ns: number;
  at: string;
  kind: string;
  from?: number;
  target?: number;
  detail: string;
  snapshots?: NodeView[];
}

export interface Violation {
  checker: string;
  detail: string;
}

export interface TraceVerdict {
  passed: boolean;
  violation?: Violation;
  violation_event_index?: number;
}

export interface TraceMeta {
  seed: number;
  num_nodes: number;
  max_time_ns: number;
  max_time: string;
  drop_rate: number;
  partition: boolean;
  num_client_ops: number;
}

export interface RunTrace {
  meta: TraceMeta;
  events: TraceEvent[];
  verdict: TraceVerdict;
  trace_hash: string;
}

export interface SimulateRequest {
  seed: number;
  nodes: number;
  puts: number;
  partition: boolean;
  drop_rate: number;
}

export interface SweepItem {
  seed: number;
  passed: boolean;
  events: number;
  duration: string;
  violation?: string;
  trace_hash: string;
}

export interface SweepResponse {
  total: number;
  passed: number;
  failed: number;
  wall_duration_ms: number;
  results: SweepItem[];
}

export const ROLE_FOLLOWER = 0;
export const ROLE_CANDIDATE = 1;
export const ROLE_LEADER = 2;

export const ROLE_NAMES = ["follower", "candidate", "leader"] as const;
