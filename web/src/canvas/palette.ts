export const ink = "#1e1610";
export const paper = "#efe6d4";
export const rule = "#d5c4a8";
export const moss = "#3f5d3a";
export const copper = "#c45a2a";
export const indigo = "#2c3d5a";
export const wine = "#8a2a38";
export const sand = "#cbb892";
export const mute = "#7a6d5b";

export function roleColor(role: number, alive: boolean): string {
  if (!alive) return mute;
  if (role === 2) return moss;
  if (role === 1) return copper;
  return indigo;
}
