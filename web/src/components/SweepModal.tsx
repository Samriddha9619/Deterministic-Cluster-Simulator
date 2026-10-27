import type { SweepResponse } from "../types/trace";
import styles from "./SweepModal.module.css";

interface Props {
  open: boolean;
  sweeping: boolean;
  data: SweepResponse | null;
  onClose: () => void;
  onPick: (seed: number) => void;
}

export function SweepModal({ open, sweeping, data, onClose, onPick }: Props) {
  if (!open) return null;

  return (
    <div className={styles.overlay} onClick={onClose} role="presentation">
      <div
        className={styles.card}
        onClick={(e) => e.stopPropagation()}
        role="dialog"
        aria-label="Seed sweep"
      >
        <div className={styles.head}>
          <span>Seed sweep</span>
          <button type="button" className={styles.close} onClick={onClose}>
            ×
          </button>
        </div>
        <div className={styles.stats}>
          {sweeping && "Running 24 seeds…"}
          {!sweeping && !data && "Idle."}
          {!sweeping && data && (
            <>
              {data.passed}/{data.total} pass · {data.failed} fail ·{" "}
              {data.wall_duration_ms} ms · click a cell to record that seed
            </>
          )}
        </div>
        <div className={styles.grid}>
          {(data?.results ?? []).map((item) => (
            <button
              key={item.seed}
              type="button"
              className={`${styles.cell} ${item.passed ? styles.ok : styles.bad}`}
              title={item.violation || item.trace_hash}
              onClick={() => onPick(item.seed)}
            >
              <span className={styles.seed}>{item.seed}</span>
              <span>{item.passed ? "pass" : "fail"}</span>
              <span className={styles.meta}>{item.events} ev</span>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
