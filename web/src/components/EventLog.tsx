import { useEffect, useMemo, useRef, useState } from "react";
import type { TraceEvent } from "../types/trace";
import styles from "./EventLog.module.css";

const PREFERRED = [
  "all",
  "deliver",
  "client-req",
  "partition",
  "heal",
  "crash",
  "restart",
  "tick",
];

const ROW_H = 26;

interface Props {
  events: TraceEvent[];
  index: number;
  onSeek: (i: number) => void;
}

export function EventLog({ events, index, onSeek }: Props) {
  const [filter, setFilter] = useState("all");
  const [scrollTop, setScrollTop] = useState(0);
  const [height, setHeight] = useState(240);
  const listRef = useRef<HTMLDivElement>(null);

  const kinds = useMemo(() => {
    const set = new Set(events.map((e) => e.kind));
    const extra = [...set].filter((k) => !PREFERRED.includes(k)).sort();
    return PREFERRED.filter((k) => k === "all" || set.has(k)).concat(extra);
  }, [events]);

  const rows = useMemo(() => {
    return events
      .map((e, i) => ({ e, i }))
      .filter(({ e }) => filter === "all" || e.kind === filter);
  }, [events, filter]);

  useEffect(() => {
    const el = listRef.current;
    if (!el) return;
    const ro = new ResizeObserver(() => setHeight(el.clientHeight));
    ro.observe(el);
    setHeight(el.clientHeight);
    return () => ro.disconnect();
  }, []);

  useEffect(() => {
    const pos = rows.findIndex((r) => r.i === index);
    if (pos < 0) return;
    const el = listRef.current;
    if (!el) return;
    const top = pos * ROW_H;
    if (top < el.scrollTop) el.scrollTop = top;
    else if (top + ROW_H > el.scrollTop + el.clientHeight) {
      el.scrollTop = top - el.clientHeight + ROW_H;
    }
  }, [index, rows]);

  const start = Math.max(0, Math.floor(scrollTop / ROW_H) - 10);
  const vis = Math.ceil(height / ROW_H) + 20;
  const slice = rows.slice(start, start + vis);

  return (
    <section className={styles.panel}>
      <div className={styles.head}>
        <span>Event ledger</span>
        <div className={styles.filters}>
          {kinds.map((k) => (
            <button
              key={k}
              type="button"
              className={`${styles.chip} ${filter === k ? styles.chipOn : ""}`}
              onClick={() => setFilter(k)}
            >
              {k}
            </button>
          ))}
        </div>
      </div>
      <div
        className={styles.list}
        ref={listRef}
        onScroll={(e) => setScrollTop(e.currentTarget.scrollTop)}
      >
        <div style={{ height: rows.length * ROW_H, position: "relative" }}>
          {slice.map(({ e, i }, n) => (
            <div
              key={e.index ?? i}
              data-idx={i}
              className={`${styles.row} ${i === index ? styles.active : ""}`}
              style={{
                position: "absolute",
                top: (start + n) * ROW_H,
                left: 0,
                right: 0,
                height: ROW_H,
              }}
              onClick={() => onSeek(i)}
            >
              <span className={styles.idx}>{String(i + 1).padStart(4, "0")}</span>
              <span>{e.at}</span>
              <span className={styles.kind}>{e.kind}</span>
              <span>{e.detail}</span>
            </div>
          ))}
        </div>
      </div>
    </section>
  );
}
