/** Observed ruler spacing contract; values are seconds, independent of media FPS. */
export function timelineRulerIntervals(
  pixelsPerSecond: number,
  compact = false,
) {
  const density = Math.max(1, pixelsPerSecond) / (compact ? 1.5 : 1);
  const major =
    [1, 2, 5, 10, 20, 50, 100].find((seconds) => seconds * density >= 48) ??
    100;
  const minor =
    [0.1, 0.2, 0.5, 1, 2, 5, 10].find(
      (seconds) =>
        seconds * density >= 12 &&
        Math.abs(major / seconds - Math.round(major / seconds)) < 1e-6,
    ) ?? major;
  return { major, minor };
}
