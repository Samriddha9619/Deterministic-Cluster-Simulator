import { useEffect, useRef } from "react";
import {
  drawCluster,
  hitTestNode,
  nodePositions,
} from "../canvas/drawCluster";
import { partitionAt, snapshotsAt } from "../lib/partition";
import type { TraceEvent } from "../types/trace";
import styles from "./ClusterView.module.css";

interface Props {
  events: TraceEvent[];
  index: number;
  selectedId: number;
  onSelect: (id: number) => void;
  playing: boolean;
}

export function ClusterView({
  events,
  index,
  selectedId,
  onSelect,
  playing,
}: Props) {
  const wrapRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);

  useEffect(() => {
    const canvas = canvasRef.current;
    const wrap = wrapRef.current;
    if (!canvas || !wrap) return;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    let raf = 0;
    let t0 = performance.now();

    const paint = (now: number) => {
      const rect = wrap.getBoundingClientRect();
      const dpr = window.devicePixelRatio || 1;
      canvas.width = Math.max(1, Math.floor(rect.width * dpr));
      canvas.height = Math.max(1, Math.floor(rect.height * dpr));
      ctx.setTransform(dpr, 0, 0, dpr, 0, 0);

      const snaps = snapshotsAt(events, index);
      const part = partitionAt(events, index);
      const evt = events[index];
      const elapsed = (now - t0) / 900;
      const packetT = playing ? (elapsed % 1) : 0.5;
      drawCluster(
        ctx,
        rect.width,
        rect.height,
        snaps,
        evt,
        part,
        selectedId,
        packetT,
      );
      if (playing) raf = requestAnimationFrame(paint);
    };

    paint(performance.now());
    const ro = new ResizeObserver(() => paint(performance.now()));
    ro.observe(wrap);
    if (playing) raf = requestAnimationFrame(paint);
    return () => {
      cancelAnimationFrame(raf);
      ro.disconnect();
    };
  }, [events, index, selectedId, playing]);

  const onClick = (e: React.MouseEvent<HTMLCanvasElement>) => {
    const canvas = canvasRef.current;
    const wrap = wrapRef.current;
    if (!canvas || !wrap) return;
    const rect = wrap.getBoundingClientRect();
    const snaps = snapshotsAt(events, index);
    const positions = nodePositions(snaps.length, rect.width, rect.height);
    const id = hitTestNode(
      snaps,
      positions,
      e.clientX - rect.left,
      e.clientY - rect.top,
    );
    if (id != null) onSelect(id);
  };

  return (
    <section className={styles.panel}>
      <div className={styles.head}>
        <span>Cluster plate</span>
        <div className={styles.legend}>
          <span>
            <i className={`${styles.dot} ${styles.leader}`} />
            leader
          </span>
          <span>
            <i className={`${styles.dot} ${styles.follower}`} />
            follower
          </span>
          <span>
            <i className={`${styles.dot} ${styles.candidate}`} />
            candidate
          </span>
          <span>
            <i className={`${styles.dot} ${styles.cut}`} />
            cut
          </span>
        </div>
      </div>
      <div className={styles.wrap} ref={wrapRef}>
        <canvas ref={canvasRef} onClick={onClick} />
        {!events.length && (
          <div className={styles.empty}>Press record to expose a seed.</div>
        )}
      </div>
    </section>
  );
}
