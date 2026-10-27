import { useCallback, useEffect, useState } from "react";
import { ClusterView } from "./components/ClusterView";
import { EventLog } from "./components/EventLog";
import { NodeInspector } from "./components/NodeInspector";
import { StatusStrip } from "./components/StatusStrip";
import { SweepModal } from "./components/SweepModal";
import { TopBar } from "./components/TopBar";
import { TransportBar } from "./components/TransportBar";
import { ViolationBanner } from "./components/ViolationBanner";
import { usePlayback } from "./hooks/usePlayback";
import { useSimulation, type RunParams } from "./hooks/useSimulation";
import styles from "./App.module.css";

export function App() {
  const [params, setParams] = useState<RunParams>({
    seed: 1,
    nodes: 3,
    puts: 50,
    partition: true,
  });
  const [selectedId, setSelectedId] = useState(1);
  const [sweepOpen, setSweepOpen] = useState(false);
  const { trace, running, error, sweep, sweeping, run, sweepSeeds } =
    useSimulation();
  const playback = usePlayback(trace);
  const events = trace?.events ?? [];

  useEffect(() => {
    void run({ seed: 1, nodes: 3, puts: 50, partition: true });
  }, [run]);

  const jumpViolation = useCallback(() => {
    if (!trace) return;
    const marked = trace.verdict.violation_event_index;
    if (marked && marked > 0) {
      playback.seek(marked);
      return;
    }
    const hit = events.findIndex((e) =>
      ["crash", "partition", "link-cut"].includes(e.kind),
    );
    playback.seek(hit >= 0 ? hit : playback.max);
  }, [events, playback, trace]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      const tag = (e.target as HTMLElement | null)?.tagName;
      if (tag === "INPUT" || tag === "SELECT" || tag === "TEXTAREA") return;
      if (e.key === " " || e.code === "Space") {
        e.preventDefault();
        if (playback.playing) playback.pause();
        else playback.play();
      } else if (e.key === "ArrowLeft") {
        playback.seek(playback.index - 1);
      } else if (e.key === "ArrowRight") {
        playback.seek(playback.index + 1);
      } else if (e.key === "j" || e.key === "J") {
        playback.skip(-1);
      } else if (e.key === "k" || e.key === "K") {
        playback.skip(1);
      } else if (e.key === "Escape") {
        setSweepOpen(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [playback]);

  const onRun = () => {
    void run(params).then((next) => {
      if (next?.events[0]?.snapshots?.[0]) {
        setSelectedId(next.events[0].snapshots[0].ID);
      }
    });
  };

  const onSweep = () => {
    setSweepOpen(true);
    void sweepSeeds(params);
  };

  const onPickSeed = (seed: number) => {
    const next = { ...params, seed };
    setParams(next);
    setSweepOpen(false);
    void run(next);
  };

  return (
    <div className="app">
      <TopBar
        params={params}
        onChange={setParams}
        running={running}
        sweeping={sweeping}
        onRun={onRun}
        onSweep={onSweep}
      />
      <StatusStrip
        trace={trace}
        index={playback.index}
        onJumpViolation={jumpViolation}
      />
      {error && <div className={styles.error}>{error}</div>}
      {trace && <ViolationBanner verdict={trace.verdict} />}
      <main className={styles.workspace}>
        <div className={styles.cluster}>
          <ClusterView
            events={events}
            index={playback.index}
            selectedId={selectedId}
            onSelect={setSelectedId}
            playing={playback.playing}
          />
        </div>
        <div className={styles.inspector}>
          <NodeInspector
            events={events}
            index={playback.index}
            selectedId={selectedId}
          />
        </div>
        <div className={styles.log}>
          <EventLog
            events={events}
            index={playback.index}
            onSeek={playback.seek}
          />
        </div>
      </main>
      <TransportBar
        events={events}
        index={playback.index}
        max={playback.max}
        playing={playback.playing}
        speed={playback.speed}
        onSeek={playback.seek}
        onPlay={playback.play}
        onPause={playback.pause}
        onSpeed={playback.setSpeed}
      />
      <SweepModal
        open={sweepOpen}
        sweeping={sweeping}
        data={sweep}
        onClose={() => setSweepOpen(false)}
        onPick={onPickSeed}
      />
    </div>
  );
}
