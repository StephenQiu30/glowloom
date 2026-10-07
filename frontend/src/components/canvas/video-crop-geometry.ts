// Derived from BeefTV 1ae25027 web/src/lib/canvas/video-crop-geometry.ts.
// MIT; see workspace/content/licenses/beeftv.txt. Added explicit finite-value/source budgets.
export type VideoCropRect = {
  x: number;
  y: number;
  width: number;
  height: number;
};
export type VideoDimensions = { width: number; height: number };
export type VideoCropHandle = "nw" | "n" | "ne" | "e" | "se" | "s" | "sw" | "w";
const clamp = (value: number, minimum: number, maximum: number) =>
  Math.max(minimum, Math.min(maximum, value));
function validate(crop: VideoCropRect, source: VideoDimensions) {
  if (
    !Object.values(crop).every(Number.isFinite) ||
    crop.width < 2 ||
    crop.height < 2 ||
    ![source.width, source.height].every(
      (value) => Number.isSafeInteger(value) && value >= 2 && value <= 32768,
    )
  )
    throw new Error("视频源尺寸或裁切坐标无效。");
}
export function resizeVideoCrop(
  crop: VideoCropRect,
  video: VideoDimensions,
  handle: VideoCropHandle,
  deltaX: number,
  deltaY: number,
): VideoCropRect {
  validate(crop, video);
  if (![deltaX, deltaY].every(Number.isFinite))
    throw new Error("裁切位移无效。");
  const min = Math.min(16, video.width, video.height),
    east = crop.x + crop.width,
    south = crop.y + crop.height;
  let left = crop.x,
    top = crop.y,
    right = east,
    bottom = south;
  if (handle.includes("w")) left = clamp(crop.x + deltaX, 0, east - min);
  if (handle.includes("e"))
    right = clamp(east + deltaX, crop.x + min, video.width);
  if (handle.includes("n")) top = clamp(crop.y + deltaY, 0, south - min);
  if (handle.includes("s"))
    bottom = clamp(south + deltaY, crop.y + min, video.height);
  return {
    x: Math.round(left),
    y: Math.round(top),
    width: Math.round(right - left),
    height: Math.round(bottom - top),
  };
}
export function moveVideoCrop(
  crop: VideoCropRect,
  video: VideoDimensions,
  deltaX: number,
  deltaY: number,
): VideoCropRect {
  validate(crop, video);
  if (![deltaX, deltaY].every(Number.isFinite))
    throw new Error("裁切位移无效。");
  return {
    ...crop,
    x: Math.round(
      clamp(crop.x + deltaX, 0, Math.max(0, video.width - crop.width)),
    ),
    y: Math.round(
      clamp(crop.y + deltaY, 0, Math.max(0, video.height - crop.height)),
    ),
  };
}
/** Keep even H264/yuv420p origins and dimensions inside the actual source. */
export function normalizeVideoCropForEncoding(
  crop: VideoCropRect,
  video: VideoDimensions,
): VideoCropRect {
  validate(crop, video);
  const evenOrigin = (value: number, maximum: number) =>
    clamp(
      Math.floor(value / 2) * 2,
      0,
      Math.max(0, Math.floor(maximum / 2) * 2),
    );
  const evenSize = (value: number, maximum: number) =>
    clamp(
      Math.round(value / 2) * 2,
      2,
      Math.max(2, Math.floor(maximum / 2) * 2),
    );
  const x = evenOrigin(crop.x, video.width - 2),
    y = evenOrigin(crop.y, video.height - 2);
  return {
    x,
    y,
    width: evenSize(crop.width, video.width - x),
    height: evenSize(crop.height, video.height - y),
  };
}
