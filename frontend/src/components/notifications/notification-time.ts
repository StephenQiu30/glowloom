/** Announcement inputs always use Asia/Shanghai, independently of the browser. */
export function beijingDateTime(value: string): string {
  const instant = Date.parse(value);
  return Number.isFinite(instant)
    ? new Date(instant + 8 * 60 * 60 * 1000).toISOString().slice(0, 16)
    : "";
}

export function beijingInstant(value: string): string {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) return "";
  const instant = Date.parse(`${value}:00+08:00`);
  if (!Number.isFinite(instant)) return "";
  const result = new Date(instant).toISOString();
  return beijingDateTime(result) === value ? result : "";
}
