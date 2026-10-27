import type { RunTrace, SimulateRequest, SweepResponse } from "../types/trace";

async function readJson<T>(res: Response): Promise<T> {
  if (!res.ok) {
    const body = await res.text();
    throw new Error(body || `HTTP ${res.status}`);
  }
  return (await res.json()) as T;
}

export function runSimulation(req: SimulateRequest): Promise<RunTrace> {
  return fetch("/api/simulate", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(req),
  }).then((r) => readJson<RunTrace>(r));
}

export function runSweep(params: {
  start_seed: number;
  count: number;
  nodes: number;
  partition: boolean;
  puts: number;
}): Promise<SweepResponse> {
  return fetch("/api/sweep", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(params),
  }).then((r) => readJson<SweepResponse>(r));
}
