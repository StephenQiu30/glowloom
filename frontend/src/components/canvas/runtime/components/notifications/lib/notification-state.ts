type MailboxSnapshot = {
  dataEpoch?: string;
  mailboxVersion?: number | string;
};

/** Compare decimal versions without losing precision; the response belongs to its request epoch. */
export function reconcileMailbox<T extends MailboxSnapshot>(
  current: T | undefined,
  incoming: T,
  requestedEpoch?: string,
): T {
  if (!incoming.dataEpoch || decimal(incoming.mailboxVersion) === undefined)
    throw new Error("通知版本信息无效，请重试。");
  if (!current) return incoming;
  if (current.dataEpoch !== incoming.dataEpoch)
    return requestedEpoch === current.dataEpoch ? incoming : current;
  return decimal(incoming.mailboxVersion)! < decimal(current.mailboxVersion)!
    ? current
    : incoming;
}

export function decimal(
  value: number | string | undefined,
): bigint | undefined {
  if (typeof value === "number" && !Number.isSafeInteger(value)) return;
  const text = String(value ?? "");
  return /^(0|[1-9]\d{0,18})$/.test(text) ? BigInt(text) : undefined;
}

export function unreadBadge(count: number | string | undefined) {
  const value = decimal(count);
  return value === undefined
    ? undefined
    : value > BigInt(99)
      ? "99+"
      : value.toString();
}

export function notificationHref(item: {
  actionType?: string;
  subjectId?: string | number;
}) {
  const id = String(item.subjectId ?? "");
  if (!/^[1-9]\d{0,18}$/.test(id)) return;
  if (item.actionType === "CREDIT_ACCOUNT") return "/account/credits";
  if (item.actionType === "GENERATION_TASK") return `/tasks?taskId=${id}`;
  if (item.actionType === "TEAM_INVITATION") return `/teams?invitationId=${id}`;
}
