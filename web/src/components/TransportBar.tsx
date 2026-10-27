import { useEffect, useRef } from "react";
import { drawTimeline } from "../canvas/drawTimeline";
import type { TraceEvent } from "../types/trace";
import styles from "./TransportBar.module.css";

interface Props {
  events: TraceEvent[];
  index: number;
  max: number;
  playing: boolean;
  speed: number;
  onSeek: (i: number) => void;
  onPlay: () => void;
  onPause: () => void;
  onSpeed: (n: number) => void;
}

export function TransportBar({
  events,
  index,
  max,
  playing,
  speed,
  onSeek,
  onPlay,
  onPause,
  onSpeed,
}: Props) {
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const evt = events[index];

  useEffect(() => {
    const canvas = canvasRef.current;
    const wrap = wrapRef.current;
    if (!canvas || !wrap) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    const paint = () => {
      const rect = wrap.getBoundingClientRect();
      const dpr = window.devicePixelRatio || 1;
      canvas.width = Math.max(1, Math.floor(rect.width * dpr));
      canvas.height = Math.max(1, Math.floor(36 * dpr));
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
      drawTimeline(ctx, rect.width, 36, events, index);
    };
    paint();
    const ro = new ResizeObserver(paint);
    ro.observe(wrap);
    return () => ro.disconnect();
  }, [events, index]);

  return (
    <footer className={styles.bar}>
      <div className={styles.btns}>
        <button
          type="button"
          className={styles.icon}
          title="Previous event"
          onClick={() => onSeek(index - 1)}
        >
          ‹
        </button>
        <button
          type="button"
          className={`${styles.icon} ${styles.play}`}
          title="Play / pause"
          onClick={() => (playing ? onPause() : onPlay())}
        >
          {playing ? "❚❚" : "▶"}
        </button>
        <button
          type="button"
          className={styles.icon}
          title="Next event"
          onClick={() => onSeek(index + 1)}
        >
          ›
        </button>
        <select
          className={styles.speed}
          value={speed}
          onChange={(e) => onSpeed(Number(e.target.value))}
        >
          <option value={1}>1×</option>
          <option value={2}>2×</option>
          <option value={5}>5×</option>
          <option value={10}>10×</option>
          <option value={20}>20×</option>
        </select>
      </div>
      <div className={styles.timeline} ref={wrapRef}>
        <canvas ref={canvasRef} />
        <input
          className={styles.slider}
          type="range"
          min={0}
          max={max}
          value={index}
          onChange={(e) => onSeek(Number(e.target.value))}
        />
      </div>
      <div className={styles.now}>
        <span className={styles.pill}>{evt?.kind ?? "idle"}</span>
        <span className={styles.detail}>
          {evt ? evt.detail : "No trace loaded."}
        </span>
      </div>
    </footer>
  );
}
