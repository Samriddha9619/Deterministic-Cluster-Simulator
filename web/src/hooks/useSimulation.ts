import { useCallback, useState } from "react";
import { runSimulation, runSweep } from "../api/client";
import type { RunTrace, SimulateRequest, SweepResponse } from "../types/trace";

export interface RunParams {
  seed: number;
  nodes: number;
  puts: number;
  partition: boolean;
}

export function useSimulation() {
  const [trace, setTrace] = useState<RunTrace | null>(null);
  const [running, setRunning] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [sweep, setSweep] = useState<SweepResponse | null>(null);
  const [sweeping, setSweeping] = useState(false);

  const run = useCallback(async (params: RunParams) => {
    setRunning(true);
    setError(null);
    const body: SimulateRequest = {
      seed: params.seed,
      nodes: params.nodes,
      puts: params.puts,
      partition: params.partition,
      drop_rate: 0.05,
    };
    try {
      const next = await runSimulation(body);
      setTrace(next);
      return next;
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(msg);
      return null;
    } finally {
      setRunning(false);
    }
  }, []);

  const sweepSeeds = useCallback(async (params: RunParams) => {
    setSweeping(true);
    setError(null);
    try {
      const data = await runSweep({
        start_seed: 1,
        count: 24,
        nodes: params.nodes,
        partition: params.partition,
        puts: Math.min(params.puts, 30),
      });
      setSweep(data);
      return data;
    } catch (err) {
      const msg = err instanceof Error ? err.message : String(err);
      setError(msg);
      return null;
    } finally {
      setSweeping(false);
    }
  }, []);

  return { trace, running, error, sweep, sweeping, run, sweepSeeds, setSweep };
}
