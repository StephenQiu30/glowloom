export const timelineAspectRatios = [
  "auto",
  "2.35:1",
  "21:9",
  "1.85:1",
  "16:9",
  "3:2",
  "4:3",
  "1:1",
  "3:4",
  "2:3",
  "9:16",
] as const;

export type TimelineAspectRatio = (typeof timelineAspectRatios)[number];
export type TimelineDimensions = Readonly<{ width: number; height: number }>;

const explicitDimensions: Record<
  Exclude<TimelineAspectRatio, "auto">,
  TimelineDimensions
> = {
  "2.35:1": { width: 2538, height: 1080 },
  "21:9": { width: 2520, height: 1080 },
  "1.85:1": { width: 1998, height: 1080 },
  "16:9": { width: 1920, height: 1080 },
  "3:2": { width: 1620, height: 1080 },
  "4:3": { width: 1440, height: 1080 },
  "1:1": { width: 1080, height: 1080 },
  "3:4": { width: 1080, height: 1440 },
  "2:3": { width: 1080, height: 1620 },
  "9:16": { width: 1080, height: 1920 },
};

export function isTimelineAspectRatio(
  value: unknown,
): value is TimelineAspectRatio {
  return timelineAspectRatios.some((ratio) => ratio === value);
}

/** Auto remains unresolved until the host supplies authoritative source dimensions. */
export function timelinePreviewDimensions(
  value: unknown,
  resolvedAuto?: TimelineDimensions,
): TimelineDimensions | null {
  if (!isTimelineAspectRatio(value)) return null;
  if (value !== "auto") return explicitDimensions[value];
  if (
    !resolvedAuto ||
    !Number.isSafeInteger(resolvedAuto.width) ||
    !Number.isSafeInteger(resolvedAuto.height) ||
    resolvedAuto.width <= 0 ||
    resolvedAuto.height <= 0
  ) {
    return null;
  }
  return resolvedAuto;
}

export function timelinePreviewAspectRatio(
  value: unknown,
  resolvedAuto?: TimelineDimensions,
): string | undefined {
  const dimensions = timelinePreviewDimensions(value, resolvedAuto);
  return dimensions ? `${dimensions.width} / ${dimensions.height}` : undefined;
}
