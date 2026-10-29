/** Compact asset metadata; exact timestamps remain available on the card. */
export function formatAssetTime(value: string, now = Date.now()): string {
  const date = new Date(value);
  const minutes = Math.max(0, Math.floor((now - date.getTime()) / 60_000));
  if (minutes < 1) return "刚刚";
  if (minutes < 60) return `${minutes}分钟前`;
  if (minutes < 1440) return `${Math.floor(minutes / 60)}小时前`;
  if (minutes < 10080) return `${Math.floor(minutes / 1440)}天前`;
  const prefix =
    date.getFullYear() === new Date(now).getFullYear()
      ? ""
      : `${date.getFullYear()}年`;
  return `${prefix}${String(date.getMonth() + 1).padStart(2, "0")}月${String(date.getDate()).padStart(2, "0")}日`;
}
