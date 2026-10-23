// Adapted from basketikun/infinite-canvas @ 9f2bd9f (MIT).
// Copyright (c) 2026 basketikun. See /licenses/infinite-canvas.txt.
export function fitNodeSize(
  width: number,
  height: number,
  maxWidth = 640,
  maxHeight = 640,
) {
  const w = Math.max(1, width);
  const h = Math.max(1, height);
  const scale = Math.min(1, maxWidth / w, maxHeight / h);
  return { width: w * scale, height: h * scale };
}
