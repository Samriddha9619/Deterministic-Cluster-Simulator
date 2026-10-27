import { useCallback, useEffect, useRef, useState } from "react";
import type { RunTrace } from "../types/trace";

export function usePlayback(trace: RunTrace | null) {
  const [index, setIndex] = useState(0);
  const [playing, setPlaying] = useState(false);
  const [speed, setSpeed] = useState(5);
  const timer = useRef<number | null>(null);

  const max = Math.max(0, (trace?.events.length ?? 1) - 1);

  const seek = useCallback(
    (i: number) => {
      setIndex(Math.max(0, Math.min(i, max)));
    },
    [max],
  );

  const pause = useCallback(() => {
    setPlaying(false);
    if (timer.current !== null) {
      window.clearInterval(timer.current);
      timer.current = null;
    }
  }, []);

  const play = useCallback(() => {
    if (!trace?.events.length) return;
    setIndex((prev) => (prev >= max ? 0 : prev));
    setPlaying(true);
  }, [trace, max]);

  useEffect(() => {
    if (!playing || !trace?.events.length) return;
    const ms = Math.max(16, 140 / speed);
    timer.current = window.setInterval(() => {
      setIndex((prev) => {
        if (prev >= max) {
          pause();
          return prev;
        }
        return prev + 1;
      });
    }, ms);
    return () => {
      if (timer.current !== null) window.clearInterval(timer.current);
    };
  }, [playing, speed, trace, max, pause]);

  useEffect(() => {
    setIndex(0);
    pause();
  }, [trace, pause]);

  const skip = useCallback(
    (dir: 1 | -1) => {
      if (!trace) return;
      let i = index + dir;
      while (i >= 0 && i <= max) {
        const kind = trace.events[i]?.kind;
        if (kind && kind !== "tick") {
          seek(i);
          return;
        }
        i += dir;
      }
      seek(dir === 1 ? max : 0);
    },
    [index, max, seek, trace],
  );

  return { index, seek, playing, play, pause, speed, setSpeed, skip, max };
}
