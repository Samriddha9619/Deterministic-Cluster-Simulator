import type { FormEvent } from "react";
import type { RunParams } from "../hooks/useSimulation";
import styles from "./TopBar.module.css";

interface Props {
  params: RunParams;
  onChange: (next: RunParams) => void;
  running: boolean;
  sweeping: boolean;
  onRun: () => void;
  onSweep: () => void;
}

export function TopBar({
  params,
  onChange,
  running,
  sweeping,
  onRun,
  onSweep,
}: Props) {
  const submit = (e: FormEvent) => {
    e.preventDefault();
    onRun();
  };

  return (
    <header className={styles.bar}>
      <div className={styles.brand}>
        <div className={styles.kicker}>World A · simulation</div>
        <h1 className={styles.title}>Cluster trace</h1>
        <div className={styles.sub}>
          Seeded Raft on a fake net. Same seed, same film — rewind any event.
        </div>
      </div>
      <form className={styles.form} onSubmit={submit}>
        <label className={styles.field}>
          Seed
          <input
            type="number"
            min={1}
            value={params.seed}
            onChange={(e) =>
              onChange({ ...params, seed: Number(e.target.value) || 1 })
            }
          />
        </label>
        <label className={styles.field}>
          Nodes
          <select
            value={params.nodes}
            onChange={(e) => onChange({ ...params, nodes: Number(e.target.value) })}
          >
            <option value={3}>3</option>
            <option value={5}>5</option>
          </select>
        </label>
        <label className={styles.field}>
          Puts
          <select
            value={params.puts}
            onChange={(e) => onChange({ ...params, puts: Number(e.target.value) })}
          >
            <option value={20}>20</option>
            <option value={50}>50</option>
            <option value={100}>100</option>
          </select>
        </label>
        <label className={`${styles.field} ${styles.check}`}>
          Partition
          <input
            type="checkbox"
            checked={params.partition}
            onChange={(e) => onChange({ ...params, partition: e.target.checked })}
          />
        </label>
        <button className={styles.primary} type="submit" disabled={running}>
          {running ? "Recording…" : "Record"}
        </button>
        <button
          className={styles.ghost}
          type="button"
          onClick={onSweep}
          disabled={sweeping}
        >
          {sweeping ? "Sweeping…" : "Sweep 24"}
        </button>
      </form>
    </header>
  );
}
