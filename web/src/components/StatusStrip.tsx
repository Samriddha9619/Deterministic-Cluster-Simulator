import { useState } from "react";
import { kindCounts, leaderOf, shortHash } from "../lib/format";
import { snapshotsAt } from "../lib/partition";
import type { RunTrace } from "../types/trace";
import styles from "./StatusStrip.module.css";

interface Props {
  trace: RunTrace | null;
  index: number;
  onJumpViolation: () => void;
}

export function StatusStrip({ trace, index, onJumpViolation }: Props) {
  const events = trace?.events ?? [];
  const evt = events[index];
  const snaps = snapshotsAt(events, index);
  const leader = leaderOf(snaps);
  const passed = trace?.verdict.passed;
  const pillClass =
    passed === undefined ? styles.idle : passed ? styles.pass : styles.fail;

  const [copied, setCopied] = useState(false);

  const copyHash = async () => {
    if (!trace?.trace_hash) return;
    await navigator.clipboard.writeText(trace.trace_hash);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1200);
  };

  return (
    <section className={styles.strip}>
      <div className={styles.cell}>
        <span className={styles.k}>Verdict</span>
        <span className={`${styles.pill} ${pillClass}`}>
          {passed === undefined ? "idle" : passed ? "pass" : "violation"}
        </span>
      </div>
      <div className={styles.cell}>
        <span className={styles.k}>Virtual t</span>
        <span className={styles.v}>
          {evt ? `${evt.at} / ${trace?.meta.max_time}` : "—"}
        </span>
      </div>
      <div className={styles.cell}>
        <span className={styles.k}>Frame</span>
        <span className={styles.v}>
          {events.length ? `${index + 1} / ${events.length}` : "0 / 0"}
        </span>
      </div>
      <div className={styles.cell}>
        <span className={styles.k}>Leader</span>
        <span className={styles.v}>
          {leader ? `n${leader.ID}  term ${leader.Term}` : "none"}
        </span>
      </div>
      <div className={styles.cell}>
        <span className={styles.k}>Kind mix</span>
        <span className={styles.v}>
          {events.length ? kindCounts(events.map((e) => e.kind)) : "—"}
        </span>
      </div>
      <div className={`${styles.cell} ${styles.grow}`}>
        <span className={styles.k}>Trace sha-256</span>
        <button type="button" className={styles.hash} onClick={copyHash}>
          {copied ? "copied" : shortHash(trace?.trace_hash)}
        </button>
      </div>
      <button
        type="button"
        className={styles.tiny}
        disabled={!trace || passed !== false}
        onClick={onJumpViolation}
      >
        Jump fault
      </button>
    </section>
  );
}
