import type { TraceEvent } from "../types/trace";

export interface PartitionState {
  groups: number[][] | null;
  cutPairs: Array<[number, number]>;
}

function parseGroups(detail: string): number[][] | null {
  const inner = detail.match(/partition\s+(\[\[.+\]\])/);
  if (!inner) return null;
  const groups: number[][] = [];
  const re = /\[([^\]]+)\]/g;
  let m: RegExpExecArray | null;
  while ((m = re.exec(inner[1]))) {
    const ids = m[1]
      .split(/\s+/)
      .map((tok) => tok.replace(/^n/, ""))
      .map((tok) => Number(tok))
      .filter((n) => Number.isFinite(n) && n > 0);
    if (ids.length) groups.push(ids);
  }
  return groups.length ? groups : null;
}

export function partitionAt(events: TraceEvent[], index: number): PartitionState {
  let groups: number[][] | null = null;
  const cutPairs: Array<[number, number]> = [];

  const upTo = Math.min(index, events.length - 1);
  for (let i = 0; i <= upTo; i++) {
    const e = events[i];
    if (e.kind === "partition") {
      groups = parseGroups(e.detail) ?? groups;
    } else if (e.kind === "heal") {
      groups = null;
      cutPairs.length = 0;
    } else if (e.kind === "link-cut" && e.from && e.target) {
      cutPairs.push([e.from, e.target]);
    } else if (e.kind === "link-restore" && e.from && e.target) {
      for (let p = cutPairs.length - 1; p >= 0; p--) {
        const [a, b] = cutPairs[p];
        if (
          (a === e.from && b === e.target) ||
          (a === e.target && b === e.from)
        ) {
          cutPairs.splice(p, 1);
        }
      }
    }
  }

  return { groups, cutPairs };
}

export function isCut(
  a: number,
  b: number,
  state: PartitionState,
): boolean {
  for (const [x, y] of state.cutPairs) {
    if ((x === a && y === b) || (x === b && y === a)) return true;
  }
  if (!state.groups) return false;
  const ga = state.groups.findIndex((g) => g.includes(a));
  const gb = state.groups.findIndex((g) => g.includes(b));
  return ga !== -1 && gb !== -1 && ga !== gb;
}

export function snapshotsAt(events: TraceEvent[], index: number): import("../types/trace").NodeView[] {
  for (let i = index; i >= 0; i--) {
    const snaps = events[i]?.snapshots;
    if (snaps && snaps.length) return snaps;
  }
  return [];
}
