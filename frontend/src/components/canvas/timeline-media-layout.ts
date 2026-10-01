import type { TimelineClip, TimelineProject } from "./timeline";

/** Match the export renderer: crop source pixels, then contain in the output frame. */
export function timelineMediaLayout(
  sourceWidth: number,
  sourceHeight: number,
  aspectRatio: TimelineProject["aspectRatio"],
  crop: TimelineClip["crop"],
) {
  if (
    ![sourceWidth, sourceHeight].every(
      (value) => Number.isInteger(value) && value >= 2 && value <= 32768,
    )
  )
    throw new Error("素材缺少有效像素尺寸，无法预览裁切。");
  const rect = crop ?? { x: 0, y: 0, width: sourceWidth, height: sourceHeight };
  if (
    ![rect.x, rect.y, rect.width, rect.height].every(Number.isInteger) ||
    rect.x < 0 ||
    rect.y < 0 ||
    rect.width < 2 ||
    rect.height < 2 ||
    rect.x + rect.width > sourceWidth ||
    rect.y + rect.height > sourceHeight
  )
    throw new Error("裁切范围超出实际素材尺寸。");
  const [outputWidth, outputHeight] = aspectRatio.split(":").map(Number);
  const ratio = rect.width / rect.height / (outputWidth / outputHeight);
  return {
    frameWidthPercent: Math.min(100, ratio * 100),
    frameHeightPercent: Math.min(100, 100 / ratio),
    mediaWidthPercent: (sourceWidth / rect.width) * 100,
    mediaHeightPercent: (sourceHeight / rect.height) * 100,
    leftPercent: rect.x === 0 ? 0 : (-rect.x / rect.width) * 100,
    topPercent: rect.y === 0 ? 0 : (-rect.y / rect.height) * 100,
  };
}
