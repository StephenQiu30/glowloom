import { RequestError } from "@/lib/request";
import { isProjectID } from "@/components/projects/project";

/** Validated presentation facts; request DTOs remain generated in @/api. */
export type SubjectMedia = {
  binding_id: string;
  asset_id: string;
  asset_revision: number;
  kind: "image" | "video" | "audio";
  mime_type: string;
  file_name: string;
  duration_ms?: number;
  width?: number;
  height?: number;
};
export type UnifiedSubject = {
  id: string;
  owner_scope_id: string;
  revision: number;
  version_id: string;
  name: string;
  description: string;
  primary: SubjectMedia;
  references: SubjectMedia[];
  voice?: { name: string; media: SubjectMedia };
  favorite: boolean;
  created_at: string;
  updated_at: string;
};
export type UnifiedSubjectPage = {
  current_actor_id: string;
  current_scope_id: string;
  items: UnifiedSubject[];
  page: number;
  page_size: number;
  total: number;
  capabilities: { intelligent_voice: { available: boolean; reason?: string } };
};
export type SubjectOrder =
  "updated_desc" | "updated_asc" | "created_desc" | "created_asc";
export type SubjectSelection = { id: string; expected_revision: number };
export type SubjectBatchResult = {
  items: {
    id: string;
    status: string;
    code?: string;
    revision?: number;
    favorite?: boolean;
  }[];
};
export type SelectedSubjectMedia = {
  asset_id: string;
  asset_revision: number;
  kind: SubjectMedia["kind"];
  file_name: string;
  mime_type: string;
  project_id?: string;
  library_item_id?: string;
  library_source?: "subject";
  bound?: { subjectID: string; versionID: string; bindingID: string };
};
const record = (v: unknown): v is Record<string, unknown> =>
  !!v && typeof v === "object" && !Array.isArray(v);
const positive = (v: unknown) => Number.isSafeInteger(v) && Number(v) > 0;
const date = (v: unknown) =>
  typeof v === "string" && Number.isFinite(Date.parse(v));
const fail = () => {
  throw new RequestError("invalid_receipt");
};
function media(v: unknown, kinds: readonly string[]): v is SubjectMedia {
  return (
    record(v) &&
    isProjectID(v.binding_id) &&
    isProjectID(v.asset_id) &&
    positive(v.asset_revision) &&
    typeof v.kind === "string" &&
    kinds.includes(v.kind) &&
    typeof v.mime_type === "string" &&
    typeof v.file_name === "string"
  );
}
export function requireUnifiedSubject(
  value: unknown,
  scopeID: string,
  id?: string,
): UnifiedSubject {
  const v = value as UnifiedSubject | null;
  if (
    !v ||
    !isProjectID(v.id) ||
    (id && v.id !== id) ||
    v.owner_scope_id !== scopeID ||
    !positive(v.revision) ||
    !isProjectID(v.version_id) ||
    typeof v.name !== "string" ||
    !v.name.trim() ||
    typeof v.description !== "string" ||
    typeof v.favorite !== "boolean" ||
    !date(v.created_at) ||
    !date(v.updated_at) ||
    !media(v.primary, ["image", "video"]) ||
    !Array.isArray(v.references) ||
    v.references.some((r) => !media(r, ["image"])) ||
    (v.voice &&
      (typeof v.voice.name !== "string" || !media(v.voice.media, ["audio"])))
  )
    return fail();
  const bindings = [
    v.primary,
    ...v.references,
    ...(v.voice ? [v.voice.media] : []),
  ].map((m) => m.binding_id);
  if (new Set(bindings).size !== bindings.length) return fail();
  return v;
}
export function requireUnifiedSubjectPage(
  value: unknown,
  actorID: string,
  scopeID: string,
): UnifiedSubjectPage {
  const v = value as UnifiedSubjectPage | null;
  if (
    !v ||
    v.current_actor_id !== actorID ||
    v.current_scope_id !== scopeID ||
    !positive(v.page) ||
    !positive(v.page_size) ||
    v.page_size > 120 ||
    !Number.isSafeInteger(v.total) ||
    v.total < 0 ||
    !Array.isArray(v.items) ||
    v.items.length > v.page_size ||
    !v.capabilities ||
    typeof v.capabilities.intelligent_voice?.available !== "boolean"
  )
    return fail();
  v.items.forEach((s) => requireUnifiedSubject(s, scopeID));
  if (
    new Set(v.items.map((s) => s.id)).size !== v.items.length ||
    v.items.length > v.total
  )
    return fail();
  return v;
}
export function requireSubjectBatchResult(
  value: unknown,
  targets: SubjectSelection[],
): SubjectBatchResult {
  const v = value as SubjectBatchResult | null;
  if (
    !v ||
    !Array.isArray(v.items) ||
    v.items.length !== targets.length ||
    new Set(v.items.map((i) => i.id)).size !== targets.length ||
    v.items.some(
      (i) =>
        !targets.some((t) => t.id === i.id) ||
        !["succeeded", "failed"].includes(i.status) ||
        (i.status === "failed" && typeof i.code !== "string"),
    )
  )
    return fail();
  return v;
}
export function selectedSubjectMedia(
  subject: UnifiedSubject,
  value: SubjectMedia,
): SelectedSubjectMedia {
  return {
    ...value,
    bound: {
      subjectID: subject.id,
      versionID: subject.version_id,
      bindingID: value.binding_id,
    },
  };
}
export function subjectDateRange(
  from: string,
  through: string,
): { from?: string; to?: string } {
  function parse(value: string) {
    const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
    if (!match) throw new Error("请选择有效日期。");
    const y = Number(match[1]),
      m = Number(match[2]),
      d = Number(match[3]);
    const result = new Date(2000, 0, 1);
    result.setFullYear(y, m - 1, d);
    if (
      y < 1 ||
      result.getFullYear() !== y ||
      result.getMonth() !== m - 1 ||
      result.getDate() !== d
    )
      throw new Error("请选择有效日期。");
    return result;
  }
  const start = from ? parse(from) : undefined;
  const end = through ? parse(through) : undefined;
  if (start && end && start > end)
    throw new Error("开始日期不能晚于结束日期。");
  if (end) end.setDate(end.getDate() + 1);
  return { from: start?.toISOString(), to: end?.toISOString() };
}
export function subjectError(reason: unknown) {
  if (reason instanceof RequestError) {
    if (reason.status === 401) return "登录已失效，请重新登录。";
    if (reason.status === 403) return "当前空间没有此主体的访问权限。";
    if (reason.status === 404)
      return "主体或参考素材已不可用，请刷新后重新选择。";
    if (reason.status === 409)
      return "主体或参考已更新。输入已保留，请刷新后核对再保存。";
    if ([413, 415, 422].includes(reason.status))
      return "名称或参考素材不符合要求，请检查后重试。";
    if (reason.status === 503)
      return "主体服务暂不可用，输入已保留，请稍后重试。";
  }
  return "操作结果尚未确认，请保留当前内容并重试原请求。";
}
export function subjectBatchError(code?: string) {
  if (code === "revision_conflict") return "内容已更新，请重新选择";
  if (["not_found", "subject_not_found"].includes(code ?? ""))
    return "主体已不可用";
  if (["forbidden", "scope_denied"].includes(code ?? "")) return "没有访问权限";
  if (["media_unavailable", "subject_media_unavailable"].includes(code ?? ""))
    return "参考素材已不可用";
  return "暂时无法完成，请重试";
}
export function subjectRelativeTime(
  value: string,
  field: "created" | "updated" = "updated",
  now = Date.now(),
) {
  const action = field === "created" ? "创建" : "修改";
  const minutes = Math.max(0, Math.floor((now - Date.parse(value)) / 60000));
  if (minutes < 1) return `刚刚${action}`;
  if (minutes < 60) return `${minutes}分钟前${action}`;
  if (minutes < 1440) return `${Math.floor(minutes / 60)}小时前${action}`;
  return `${Math.floor(minutes / 1440)}天前${action}`;
}
