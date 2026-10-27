import { isCut, type PartitionState } from "../lib/partition";
import { ROLE_LEADER, type NodeView, type TraceEvent } from "../types/trace";
import { copper, ink, moss, mute, paper, roleColor, rule, sand } from "./palette";

export interface Point {
  x: number;
  y: number;
}

export function nodePositions(count: number, w: number, h: number): Point[] {
  const cx = w / 2;
  const cy = h / 2 + 6;
  const radius = Math.min(cx, cy) * 0.58;
  const pts: Point[] = [];
  for (let i = 0; i < count; i++) {
    const angle = (i * 2 * Math.PI) / count - Math.PI / 2;
    pts.push({
      x: cx + radius * Math.cos(angle),
      y: cy + radius * Math.sin(angle),
    });
  }
  return pts;
}

function strokeLink(
  ctx: CanvasRenderingContext2D,
  a: Point,
  b: Point,
  cut: boolean,
) {
  ctx.beginPath();
  const mx = (a.x + b.x) / 2;
  const my = (a.y + b.y) / 2;
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const nx = -dy * 0.08;
  const ny = dx * 0.08;
  ctx.moveTo(a.x, a.y);
  ctx.quadraticCurveTo(mx + nx, my + ny, b.x, b.y);
  if (cut) {
    ctx.setLineDash([3, 7]);
    ctx.strokeStyle = wineSafe();
    ctx.lineWidth = 1.6;
  } else {
    ctx.setLineDash([]);
    ctx.strokeStyle = rule;
    ctx.lineWidth = 1.15;
  }
  ctx.stroke();
  ctx.setLineDash([]);
}

function wineSafe(): string {
  return "#8a2a38";
}

function drawNode(
  ctx: CanvasRenderingContext2D,
  snap: NodeView,
  pos: Point,
  selected: boolean,
) {
  const color = roleColor(snap.Role, snap.Alive);
  const r = 34;

  ctx.beginPath();
  ctx.arc(pos.x, pos.y, r + 10, 0, Math.PI * 2);
  ctx.fillStyle = selected ? "rgba(196,90,42,0.10)" : "rgba(30,22,16,0.03)";
  ctx.fill();

  ctx.beginPath();
  ctx.arc(pos.x, pos.y, r, 0, Math.PI * 2);
  ctx.fillStyle = paper;
  ctx.fill();
  ctx.lineWidth = selected ? 2.4 : 1.4;
  ctx.strokeStyle = selected ? copper : color;
  ctx.stroke();

  ctx.beginPath();
  ctx.arc(pos.x, pos.y, r - 7, 0, Math.PI * 2);
  ctx.strokeStyle = sand;
  ctx.lineWidth = 0.7;
  ctx.stroke();

  ctx.fillStyle = ink;
  ctx.font = "600 15px 'Azeret Mono', monospace";
  ctx.textAlign = "center";
  ctx.textBaseline = "middle";
  ctx.fillText(`n${snap.ID}`, pos.x, pos.y - 5);

  ctx.fillStyle = color;
  ctx.font = "400 10px 'Azeret Mono', monospace";
  ctx.fillText(`t${snap.Term ?? 0}`, pos.x, pos.y + 11);

  if (snap.Role === ROLE_LEADER && snap.Alive) {
    ctx.fillStyle = moss;
    ctx.font = "500 9px 'Azeret Mono', monospace";
    ctx.fillText("lead", pos.x, pos.y - r - 10);
  }
  if (!snap.Alive) {
    ctx.fillStyle = mute;
    ctx.font = "500 9px 'Azeret Mono', monospace";
    ctx.fillText("down", pos.x, pos.y - r - 10);
  }
}

export function hitTestNode(
  snaps: NodeView[],
  positions: Point[],
  x: number,
  y: number,
): number | null {
  for (let i = 0; i < snaps.length; i++) {
    const p = positions[i];
    if (Math.hypot(x - p.x, y - p.y) <= 38) return snaps[i].ID;
  }
  return null;
}

export function drawCluster(
  ctx: CanvasRenderingContext2D,
  w: number,
  h: number,
  snaps: NodeView[],
  event: TraceEvent | undefined,
  part: PartitionState,
  selectedId: number,
  packetT: number,
) {
  ctx.clearRect(0, 0, w, h);

  ctx.fillStyle = "rgba(30,22,16,0.035)";
  for (let x = 24; x < w; x += 28) {
    for (let y = 24; y < h; y += 28) {
      ctx.fillRect(x, y, 1.2, 1.2);
    }
  }

  if (!snaps.length) return;
  const positions = nodePositions(snaps.length, w, h);

  for (let i = 0; i < snaps.length; i++) {
    for (let j = i + 1; j < snaps.length; j++) {
      const cut = isCut(snaps[i].ID, snaps[j].ID, part);
      strokeLink(ctx, positions[i], positions[j], cut);
      if (cut) {
        const mx = (positions[i].x + positions[j].x) / 2;
        const my = (positions[i].y + positions[j].y) / 2;
        ctx.strokeStyle = wineSafe();
        ctx.lineWidth = 1.3;
        ctx.beginPath();
        ctx.moveTo(mx - 5, my - 5);
        ctx.lineTo(mx + 5, my + 5);
        ctx.moveTo(mx + 5, my - 5);
        ctx.lineTo(mx - 5, my + 5);
        ctx.stroke();
      }
    }
  }

  if (
    event &&
    event.from &&
    event.target &&
    event.from !== event.target &&
    event.kind === "deliver"
  ) {
    const fi = snaps.findIndex((s) => s.ID === event.from);
    const ti = snaps.findIndex((s) => s.ID === event.target);
    if (fi >= 0 && ti >= 0) {
      const a = positions[fi];
      const b = positions[ti];
      const t = 0.18 + packetT * 0.64;
      const x = a.x + (b.x - a.x) * t;
      const y = a.y + (b.y - a.y) * t;
      ctx.beginPath();
      ctx.arc(x, y, 5, 0, Math.PI * 2);
      ctx.fillStyle = copper;
      ctx.fill();
      ctx.beginPath();
      ctx.arc(x, y, 9, 0, Math.PI * 2);
      ctx.strokeStyle = "rgba(196,90,42,0.35)";
      ctx.lineWidth = 1;
      ctx.stroke();
    }
  }

  snaps.forEach((snap, i) => {
    drawNode(ctx, snap, positions[i], snap.ID === selectedId);
  });
}
