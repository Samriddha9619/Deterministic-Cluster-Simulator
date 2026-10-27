import type { TraceEvent } from "../types/trace";
import { copper, indigo, moss, rule, sand, wine } from "./palette";

function colorFor(kind: string): string | null {
  switch (kind) {
    case "partition":
    case "link-cut":
    case "crash":
      return wine;
    case "heal":
    case "restart":
    case "link-restore":
      return moss;
    case "client-req":
      return copper;
    case "deliver":
      return indigo;
    default:
      return null;
  }
}

export function drawTimeline(
  ctx: CanvasRenderingContext2D,
  w: number,
  h: number,
  events: TraceEvent[],
  index: number,
) {
  ctx.clearRect(0, 0, w, h);
  if (!events.length) return;

  const pad = 8;
  const y = h / 2;
  ctx.strokeStyle = rule;
  ctx.lineWidth = 1;
  ctx.beginPath();
  ctx.moveTo(pad, y);
  ctx.lineTo(w - pad, y);
  ctx.stroke();

  const n = events.length;
  const maxMarks = Math.min(n, 420);
  const step = Math.max(1, Math.floor(n / maxMarks));

  for (let i = 0; i < n; i += step) {
    const kind = events[i].kind;
    const c = colorFor(kind);
    if (!c && kind === "tick") continue;
    const x = pad + ((w - pad * 2) * i) / Math.max(1, n - 1);
    ctx.beginPath();
    ctx.strokeStyle = c ?? sand;
    ctx.lineWidth = c ? 1.6 : 0.8;
    const hgt = c ? 9 : 4;
    ctx.moveTo(x, y - hgt);
    ctx.lineTo(x, y + hgt);
    ctx.stroke();
  }

  const x = pad + ((w - pad * 2) * index) / Math.max(1, n - 1);
  ctx.beginPath();
  ctx.arc(x, y, 4.5, 0, Math.PI * 2);
  ctx.fillStyle = copper;
  ctx.fill();
}
