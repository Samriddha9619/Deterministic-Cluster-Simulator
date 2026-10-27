import { ROLE_LEADER, ROLE_NAMES, type NodeView } from "../types/trace";

export function nodeLabel(id: number | undefined): string {
  if (!id) return "—";
  return `n${id}`;
}

export function roleName(role: number, alive: boolean): string {
  if (!alive) return "crashed";
  return ROLE_NAMES[role] ?? "unknown";
}

export function leaderOf(snaps: NodeView[]): NodeView | undefined {
  return snaps.find((s) => s.Alive && s.Role === ROLE_LEADER);
}

export function shortHash(hash: string | undefined): string {
  if (!hash) return "—";
  return `${hash.slice(0, 12)}…${hash.slice(-4)}`;
}

export function kindCounts(kinds: string[]): string {
  const map = new Map<string, number>();
  for (const k of kinds) map.set(k, (map.get(k) ?? 0) + 1);
  return [...map.entries()]
    .sort((a, b) => b[1] - a[1])
    .slice(0, 4)
    .map(([k, n]) => `${k} ${n}`)
    .join("  ·  ");
}

export function escapeText(str: string): string {
  return str.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}
