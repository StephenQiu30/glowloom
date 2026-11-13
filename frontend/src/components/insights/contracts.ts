export const numberValue = (value: number | null) =>
  value === null ? "未接入" : new Intl.NumberFormat("zh-CN").format(value);
export const rateValue = (value: number | null) =>
  value === null
    ? "不适用"
    : new Intl.NumberFormat("zh-CN", {
        style: "percent",
        maximumFractionDigits: 2,
      }).format(value);
export const timestamp = (value: string | null) =>
  value
    ? new Intl.DateTimeFormat("zh-CN", {
        timeZone: "Asia/Shanghai",
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false,
      }).format(new Date(value))
    : "未接入";
export const originName: Record<string, string> = {
  canvas: "画布",
  agent: "Agent",
  pipeline: "流水线",
  batch: "批量",
  system: "系统",
};
export const statusName: Record<string, string> = {
  succeeded: "成功",
  completed: "完成",
  failed: "失败",
  cancelled: "已取消",
  partial: "部分完成",
  unknown: "待核对",
  reconciling: "核对中",
  manual: "待人工核对",
  draft: "草稿",
  quoted: "已报价",
  confirmed: "已确认",
  submitting: "提交中",
  submitted: "已提交",
  ingesting: "登记中",
  cancelling: "取消中",
  expired: "已过期",
  accepted: "已接受",
  running: "执行中",
  queued: "排队中",
  success: "成功",
  denied: "拒绝",
  info: "信息",
  warn: "警告",
  error: "错误",
};
export function filterQuery(
  draft: Record<string, string>,
  maxDays: number,
): Record<string, string> {
  const query = Object.fromEntries(
    Object.entries(draft)
      .filter(([, v]) => v.trim())
      .map(([k, v]) => [k, v.trim()]),
  );
  if (Boolean(query.from) !== Boolean(query.to))
    throw new Error("请同时填写开始和结束时间");
  if (query.from && query.to) {
    const from = Date.parse(`${query.from}+08:00`),
      to = Date.parse(`${query.to}+08:00`);
    if (
      !Number.isFinite(from) ||
      !Number.isFinite(to) ||
      to <= from ||
      to - from > maxDays * 86400000
    )
      throw new Error(`请选择不超过${maxDays}天的有效时间范围`);
    query.from = new Date(from).toISOString();
    query.to = new Date(to).toISOString();
  }
  return query;
}
