import { RequestError } from "@/lib/request";

export type NotificationSummary = Required<API.NotificationSummary>;
export type InboxNotification = Required<API.InboxNotification>;
export type DisplayAnnouncement = Required<API.DisplayAnnouncement>;
export type Announcement = Required<API.Announcement>;

const sequence = /^(0|[1-9]\d*)$/;

export function requireNotificationSummary(
  value: API.NotificationSummary,
): NotificationSummary {
  if (
    !value.data_epoch ||
    !sequence.test(value.mailbox_version ?? "") ||
    !sequence.test(value.max_inbox_seq ?? "") ||
    !Number.isSafeInteger(value.unread_count) ||
    value.unread_count! < 0
  )
    throw new RequestError("invalid_notification_response");
  return value as NotificationSummary;
}

export function mergeNotificationSummary(
  current: NotificationSummary | undefined,
  incoming: API.NotificationSummary,
): NotificationSummary {
  const next = requireNotificationSummary(incoming);
  if (
    current?.data_epoch === next.data_epoch &&
    BigInt(current.mailbox_version) > BigInt(next.mailbox_version)
  )
    return current;
  return next;
}

export function requireInboxNotification(
  value: API.InboxNotification,
): InboxNotification {
  if (
    !value.id ||
    !value.title ||
    typeof value.body !== "string" ||
    typeof value.read !== "boolean" ||
    !value.occurred_at ||
    !sequence.test(value.inbox_seq ?? "")
  )
    throw new RequestError("invalid_notification_response");
  return value as InboxNotification;
}

export function requireDisplayAnnouncement(
  value: API.DisplayAnnouncement,
): DisplayAnnouncement {
  if (
    !value.id ||
    !value.revision ||
    !value.title ||
    typeof value.body !== "string" ||
    !Number.isInteger(value.duration_seconds) ||
    value.duration_seconds! < 5 ||
    value.duration_seconds! > 60
  )
    throw new RequestError("invalid_notification_response");
  return value as DisplayAnnouncement;
}

export function requireAnnouncement(value: API.Announcement): Announcement {
  if (!value.id || !value.version || !value.draft?.title || !value.status)
    throw new RequestError("invalid_notification_response");
  return value as Announcement;
}

export function safeNotificationAction(
  path: string | undefined,
): string | undefined {
  if (!path || !path.startsWith("/") || path.startsWith("//")) return;
  let decoded: string;
  try {
    decoded = decodeURIComponent(path);
  } catch {
    return;
  }
  if (/[\\\u0000-\u0020]/.test(decoded) || decoded.startsWith("//")) return;
  return path;
}

export const notificationCategories = [
  ["all", "全部"],
  ["system", "系统"],
  ["task", "任务"],
  ["collaboration", "协作"],
  ["account", "账户"],
] as const;
export const categoryLabel = (category: string) =>
  notificationCategories.find(([key]) => key === category)?.[1] ?? "系统";
export const notificationDate = (value: string) =>
  new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(value));
