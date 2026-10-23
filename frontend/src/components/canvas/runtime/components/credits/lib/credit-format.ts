export function creditValue(value: number | string | undefined): number {
  const parsed = Number(value);
  if (value === undefined || !Number.isSafeInteger(parsed) || parsed < 0)
    throw new Error("积分数据格式不完整，请重新读取");
  return parsed;
}
export function creditFormat(value: number | string | undefined) {
  if (
    (typeof value === "number" &&
      (!Number.isSafeInteger(value) || value < 0)) ||
    (typeof value !== "number" &&
      (typeof value !== "string" || !/^(0|[1-9]\d*)$/.test(value)))
  )
    throw new Error("积分数据格式不完整，请重新读取");
  return BigInt(value).toLocaleString("zh-CN");
}

export function creditEntryLabel(row: CanvasContract.CreditLedgerVO) {
  return (
    (
      {
        ADMIN_GRANT: "管理员发放",
        MODEL_RESERVE: "模型预占",
        MODEL_CHARGE: "模型消费",
        MODEL_RELEASE: "积分释放",
      } as Record<string, string>
    )[row.entryType ?? ""] ?? "积分变动"
  );
}

export function creditEntryAmount(row: CanvasContract.CreditLedgerVO) {
  const prefix =
    row.entryType === "ADMIN_GRANT"
      ? "+"
      : row.entryType === "MODEL_CHARGE"
        ? "−"
        : row.entryType === "MODEL_RESERVE"
          ? "占用 "
          : row.entryType === "MODEL_RELEASE"
            ? "释放 "
            : "";
  return prefix + creditFormat(row.points);
}

export function creditEntrySource(row: CanvasContract.CreditLedgerVO) {
  return row.entryType === "ADMIN_GRANT"
    ? row.sourceProjectName
      ? `来源：${row.sourceProjectName}`
      : "直接发放"
    : row.reason || "模型任务费用";
}
export function creditTime(value?: string) {
  if (!value) return "—";
  const date = new Date(
    /(?:Z|[+-]\d\d:\d\d)$/.test(value) ? value : `${value}Z`,
  );
  return Number.isNaN(date.getTime())
    ? "—"
    : date.toLocaleString("zh-CN", {
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        hour12: false,
      });
}
export function grantStatus(batch: CanvasContract.CreditGrantBatchVO) {
  if (batch.state === "PREVIEW") return "待确认";
  const total = batch.recipientCount;
  if (total && batch.succeededCount === total) return "全部到账";
  if (batch.succeededCount) return "部分到账";
  if (batch.pendingCount) return "待发放";
  return "未到账";
}
export function parseGrantPoints(value: string): number | undefined {
  if (!/^[1-9]\d{0,7}$/.test(value)) return undefined;
  const parsed = Number(value);
  return Number.isSafeInteger(parsed) ? parsed : undefined;
}
