/** A held pointer does not count as motion after 150 ms without a position update. */
export function isMotionSample(
  moving: boolean,
  lastMotionMs: number,
  nowMs: number,
) {
  // Input dispatch can happen after the current RAF frame timestamp.
  return moving && Math.abs(nowMs - lastMotionMs) <= 150;
}

export function summarizeFrames(windows: number[][]) {
  const intervals: number[] = [];
  for (const frames of windows) {
    if (frames.some((frame) => !Number.isFinite(frame))) {
      throw new RangeError("invalid frame timestamp");
    }
    for (let index = 1; index < frames.length; index += 1) {
      const elapsed = frames[index] - frames[index - 1];
      if (elapsed <= 0) throw new RangeError("frame timestamps must increase");
      intervals.push(elapsed);
    }
  }
  const activeMs = intervals.reduce((sum, interval) => sum + interval, 0);
  const sorted = intervals.toSorted((a, b) => a - b);
  return {
    frameIntervals: intervals.length,
    activeMs,
    meanFps: activeMs > 0 ? (intervals.length * 1000) / activeMs : null,
    p95FrameMs: sorted.length
      ? sorted[Math.ceil(sorted.length * 0.95) - 1]
      : null,
  };
}

export function measurementQuality(input: {
  elapsedMs: number;
  activeMs: number;
  trustedInputs: number;
  interrupted: boolean;
}) {
  if (input.interrupted) return "记录被中断";
  if (input.trustedInputs === 0) return "缺少真实交互输入";
  if (input.elapsedMs < 60000) return "短时冒烟记录";
  if (input.activeMs < 30000) return "运动采样不足";
  return "有效本机交互记录";
}
