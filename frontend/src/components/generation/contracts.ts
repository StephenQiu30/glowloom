import { RequestError } from "@/lib/request";
import { isProjectID } from "@/components/projects/project";
export const taskLabels: Record<string, string> = {
  draft: "草稿",
  quoted: "待确认",
  confirmed: "等待生成",
  submitting: "正在提交",
  submitted: "正在生成",
  succeeded: "生成成功，准备结果",
  ingesting: "正在保存结果",
  completed: "已完成",
  failed: "生成失败",
  unknown: "结果待确认",
  reconciling: "正在核对结果",
  manual: "需要人工处理",
  cancelling: "正在取消",
  cancelled: "已取消",
  expired: "报价已过期",
};
export const activeTask = (s?: string) =>
  !!s &&
  [
    "confirmed",
    "submitting",
    "submitted",
    "succeeded",
    "ingesting",
    "unknown",
    "reconciling",
    "cancelling",
  ].includes(s);
export type Task = API.TaskDetail & {
  id: string;
  project_id: string;
  status: string;
};
export function requireTask(value: API.TaskDetail, project: string): Task {
  if (
    !value ||
    !isProjectID(value.id) ||
    value.project_id !== project ||
    !Object.hasOwn(taskLabels, value.status ?? "") ||
    (value.outputs !== undefined &&
      (!Array.isArray(value.outputs) ||
        value.outputs.some(
          (o) =>
            !o ||
            !isProjectID(o.id) ||
            (o.moderation_status === "passed" &&
              (o.kind === "json"
                ? !(
                    value.capability?.startsWith("text.") ||
                    value.capability?.startsWith("document.")
                  ) ||
                  value.status !== "completed" ||
                  typeof o.text !== "string" ||
                  !o.text.trim() ||
                  o.text.length > 65536 ||
                  !!o.media_asset_id
                : !isProjectID(o.media_asset_id) ||
                  !["image", "video", "audio", "model", "document"].includes(
                    o.kind ?? "",
                  ) ||
                  (o.kind === "document" &&
                    (value.status !== "completed" ||
                      !(
                        value.capability?.startsWith("text.") ||
                        value.capability?.startsWith("document.")
                      ))))),
        )))
  )
    throw new RequestError("invalid_receipt");
  return value as Task;
}
export type Quote = API.QuoteResponse & {
  operation_id: string;
  quote_micros: number;
  available_micros: number;
  expires_at: string;
  confirmable: boolean;
};
export function requireQuote(v: API.QuoteResponse): Quote {
  if (
    !v ||
    !isProjectID(v.operation_id) ||
    !Number.isSafeInteger(v.quote_micros) ||
    v.quote_micros! < 0 ||
    !Number.isSafeInteger(v.available_micros) ||
    typeof v.confirmable !== "boolean" ||
    !Number.isFinite(Date.parse(v.expires_at ?? ""))
  )
    throw new RequestError("invalid_receipt");
  return v as Quote;
}
export type ParamField = {
  field: string;
  label: string;
  description?: string;
  type: "string" | "number" | "integer" | "boolean";
  component: string;
  required?: boolean;
  default?: string | number | boolean;
  enum?: (string | number)[];
  min?: number;
  max?: number;
  step?: number;
  for_modes?: string[];
};
export function fieldsForMode(schema: unknown, mode: string): ParamField[] {
  if (!Array.isArray(schema) || schema.length > 128)
    throw new RequestError("invalid_receipt");
  const names = new Set<string>();
  for (const f of schema) {
    if (
      !f ||
      typeof f.field !== "string" ||
      !/^[A-Za-z_][A-Za-z0-9_]*$/.test(f.field) ||
      ["__proto__", "constructor", "prototype"].includes(f.field) ||
      names.has(f.field) ||
      typeof f.label !== "string" ||
      !["string", "integer", "number", "boolean"].includes(f.type) ||
      ![
        "input",
        "textarea",
        "select",
        "slider",
        "switch",
        "segmented",
        "voice",
      ].includes(f.component) ||
      (f.for_modes && !Array.isArray(f.for_modes)) ||
      (f.enum && !Array.isArray(f.enum))
    )
      throw new RequestError("invalid_receipt");
    names.add(f.field);
  }
  return (schema as ParamField[]).filter(
    (f) => !f.for_modes || f.for_modes.includes(mode),
  );
}
export function initialParams(fields: ParamField[]) {
  return Object.fromEntries(
    fields
      .filter((f) => f.default !== undefined)
      .map((f) => [f.field, f.default!]),
  );
}
export function flowError(e: unknown) {
  if (e instanceof RequestError) {
    const messages: Record<string, string> = {
      context_unavailable:
        "主体操作依赖的服务暂不可用，内容未确认保存。请稍后重试或联系管理员。",
      dependency_unavailable:
        "所需服务暂不可用，请稍后重试；当前操作结果尚未确认。",
      quote_expired: "报价已过期，请重新获取报价。",
      insufficient_balance: "可用余额不足，请联系管理员补充额度后重试。",
      insufficient_budget: "项目预算不足，请调整数量或联系管理员。",
      impact_acknowledgment_required: "此主体已被引用，请先确认影响范围。",
      revision_conflict: "内容已更新，请读取最新版本后再保存。",
      invalid_bible_content: "主体内容不符合规则，请检查名称、描述和参考图。",
    };
    if (messages[e.code]) return messages[e.code];
    if (e.status === 401) return "登录已失效，请重新登录。";
    if (e.status === 403) return "当前账号没有此操作权限。";
    if (e.status === 404) return "内容已不存在或当前账号无法访问。";
    if (e.status === 409) return "内容状态已发生变化，请刷新后确认。";
    if (e.status === 422) return "输入或模型参数不符合要求，请检查后重试。";
    if (e.status === 429) return "当前请求较多，请稍后重试。";
  }
  return "连接中断或结果未确认。请保留当前内容，重试原请求。";
}
export const money = (micros: number) =>
  new Intl.NumberFormat("zh-CN", {
    style: "currency",
    currency: "CNY",
    maximumFractionDigits: 6,
  }).format(micros / 1e6);
