import { RequestError } from "@/lib/request";
import { isProjectID } from "@/components/projects/project";
export type LibraryItem = API.LibraryItemSummary & {
  id: string;
  title: string;
  kind: string;
  revision: number;
  favorite: boolean;
  catalog_state: string;
  updated_at: string;
};
export type LibraryPage = Omit<API.LibraryPage, "items"> & {
  current_actor_id: string;
  current_org_id: string;
  library_id: string;
  revision: number;
  page: number;
  page_size: number;
  total: number;
  items: LibraryItem[];
  folders: API.LibraryFolder[];
};
const revision = (v: unknown) => Number.isSafeInteger(v) && Number(v) >= 0;
export function requireLibraryPage(
  value: unknown,
  actorID: string,
): LibraryPage {
  const p = value as Partial<LibraryPage> | null;
  if (
    !p ||
    p.current_actor_id !== actorID ||
    !isProjectID(p.current_org_id) ||
    !isProjectID(p.library_id) ||
    p.scope?.kind !== "personal" ||
    p.scope.project_id ||
    !revision(p.revision) ||
    !Number.isSafeInteger(p.page) ||
    p.page! < 1 ||
    !Number.isSafeInteger(p.page_size) ||
    p.page_size! < 1 ||
    p.page_size! > 120 ||
    !revision(p.total) ||
    !Array.isArray(p.items) ||
    p.items.length > p.page_size! ||
    !Array.isArray(p.folders) ||
    new Set(p.items.map((i) => i.id)).size !== p.items.length ||
    p.items.some(
      (i) =>
        !isProjectID(i.id) ||
        typeof i.title !== "string" ||
        !revision(i.revision) ||
        typeof i.favorite !== "boolean" ||
        !["text", "image", "video", "audio", "document", "model"].includes(
          i.kind,
        ) ||
        !["active", "trashed"].includes(i.catalog_state) ||
        !Number.isFinite(Date.parse(i.updated_at)),
    ) ||
    p.folders.some(
      (f) =>
        !isProjectID(f.id) ||
        typeof f.name !== "string" ||
        !revision(f.revision),
    )
  )
    throw new RequestError("invalid_receipt");
  return p as LibraryPage;
}
export function requireLibraryReceipt(
  value: unknown,
  base: LibraryPage,
): API.LibraryReceipt {
  const r = value as API.LibraryReceipt | null;
  if (
    !r ||
    r.current_actor_id !== base.current_actor_id ||
    r.current_org_id !== base.current_org_id ||
    r.library_id !== base.library_id ||
    r.scope?.kind !== "personal" ||
    r.scope.project_id ||
    r.revision !== base.revision + 1 ||
    !Array.isArray(r.items)
  )
    throw new RequestError("invalid_receipt");
  return r;
}
export function requirePreview(value: API.LibraryMediaPreview, id: string) {
  const u = value.url;
  if (
    value.asset?.id !== id ||
    !u ||
    !Number.isFinite(Date.parse(value.expires_at ?? "")) ||
    Date.parse(value.expires_at!) <= Date.now()
  )
    throw new RequestError("invalid_receipt");
  const url = new URL(u);
  if (!["http:", "https:"].includes(url.protocol))
    throw new RequestError("invalid_receipt");
  return u;
}
export function mediaError(error: unknown) {
  if (error instanceof RequestError) {
    if (error.status === 401) return "登录已失效，请重新登录后重试。";
    if (error.status === 403) return "当前账号无权访问此资产。";
    if (error.status === 404) return "资产已不存在或无法访问，请刷新列表。";
    if (error.status === 409)
      return "资产已被其他操作更新。请刷新后再试，本次修改尚未确认。";
    if (error.status === 413) return "文件过大，请选择符合大小限制的文件。";
    if (error.status === 415 || error.status === 422)
      return "文件内容或参数不符合要求，请检查后重试。";
    if (error.status === 503) return "素材服务暂时不可用，请稍后重试。";
  }
  return "操作结果未确认，请保留当前内容并重试原请求。";
}
export function validateUpload(file: File) {
  if (!file.size) return "不能上传空文件。";
  const ext = file.name.split(".").at(-1)?.toLowerCase() ?? "";
  const max = ["png", "jpg", "jpeg", "webp", "txt", "docx"].includes(ext)
    ? 20
    : ["mp4", "webm", "mov"].includes(ext)
      ? 500
      : ["mp3", "wav", "m4a", "ogg", "flac"].includes(ext)
        ? 100
        : ext === "glb"
          ? 64
          : 0;
  if (!max) return "暂不支持此文件格式。";
  if (file.size > max * 1024 * 1024) return `此类文件不能超过 ${max} MB。`;
  return "";
}
