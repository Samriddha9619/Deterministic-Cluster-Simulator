import { nodeLabel, roleName } from "../lib/format";
import { snapshotsAt } from "../lib/partition";
import { ROLE_CANDIDATE, ROLE_LEADER, type TraceEvent } from "../types/trace";
import styles from "./NodeInspector.module.css";

interface Props {
  events: TraceEvent[];
  index: number;
  selectedId: number;
}

export function NodeInspector({ events, index, selectedId }: Props) {
  const snaps = snapshotsAt(events, index);
  const snap =
    snaps.find((s) => s.ID === selectedId) ?? snaps[0] ?? null;
  const kv = snap?.KVState ?? {};
  const keys = Object.keys(kv).sort();

  let pill = styles.pill;
  if (snap && !snap.Alive) pill += ` ${styles.down}`;
  else if (snap?.Role === ROLE_LEADER) pill += ` ${styles.leader}`;
  else if (snap?.Role === ROLE_CANDIDATE) pill += ` ${styles.candidate}`;

  return (
    <aside className={styles.panel}>
      <div className={styles.head}>
        <div className={styles.label}>
          Specimen
          <span className={styles.name}>{nodeLabel(snap?.ID)}</span>
        </div>
        <span className={pill}>
          {snap ? roleName(snap.Role, snap.Alive) : "—"}
        </span>
      </div>
      <div className={styles.grid}>
        <div className={styles.metric}>
          <span className={styles.k}>Term</span>
          <div className={styles.v}>{snap?.Term ?? 0}</div>
        </div>
        <div className={styles.metric}>
          <span className={styles.k}>Vote</span>
          <div className={styles.v}>
            {snap?.VotedFor ? nodeLabel(snap.VotedFor) : "none"}
          </div>
        </div>
        <div className={styles.metric}>
          <span className={styles.k}>Commit</span>
          <div className={styles.v}>{snap?.CommitIndex ?? 0}</div>
        </div>
        <div className={styles.metric}>
          <span className={styles.k}>Applied</span>
          <div className={styles.v}>{snap?.AppliedIndex ?? 0}</div>
        </div>
        <div className={styles.metric}>
          <span className={styles.k}>Log len</span>
          <div className={styles.v}>{snap?.LogLength ?? 0}</div>
        </div>
        <div className={styles.metric}>
          <span className={styles.k}>Alive</span>
          <div className={styles.v}>
            {snap ? (snap.Alive ? "yes" : "no") : "—"}
          </div>
        </div>
      </div>
      <div className={styles.kv}>
        <div className={styles.kvHead}>
          <span>Applied store</span>
          <span>{keys.length} keys</span>
        </div>
        <div className={styles.list}>
          {keys.length === 0 ? (
            <div className={styles.empty}>No keys at this frame.</div>
          ) : (
            keys.map((k) => (
              <div className={styles.row} key={k}>
                <span className={styles.key}>{k}</span>
                <span className={styles.val}>{kv[k]}</span>
              </div>
            ))
          )}
        </div>
      </div>
      <div className={styles.help}>
        Space play · arrows step · J / K skip ticks
      </div>
    </aside>
  );
}
