import { RequestError } from "@/lib/request";

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
export function isProjectID(value: unknown): value is string {
  return (
    typeof value === "string" &&
    uuid.test(value) &&
    value !== "00000000-0000-0000-0000-000000000000"
  );
}

export type Project = {
  id: string;
  canvas_id: string | null;
  name: string;
  aspect_ratio: string;
  style_key: string | null;
  revision: number;
  update_time: string;
};

// Validate the minimal public navigation contract before displaying or routing.
export function requireProject(value: unknown, requireCanvas = false): Project {
  const p = value as Partial<Project> | null;
  if (
    !p ||
    !isProjectID(p.id) ||
    !(p.canvas_id === null || isProjectID(p.canvas_id)) ||
    (requireCanvas && !isProjectID(p.canvas_id)) ||
    typeof p.name !== "string" ||
    !p.name.trim() ||
    Array.from(p.name).length > 50 ||
    !["9:16", "16:9", "1:1"].includes(p.aspect_ratio ?? "") ||
    !(
      p.style_key === null ||
      (typeof p.style_key === "string" &&
        p.style_key.trim() &&
        Array.from(p.style_key).length <= 50)
    ) ||
    !Number.isSafeInteger(p.revision) ||
    p.revision! < 1 ||
    typeof p.update_time !== "string" ||
    !Number.isFinite(Date.parse(p.update_time))
  ) {
    throw new RequestError("invalid_receipt");
  }
  return p as Project;
}

export function requireProjectList(value: unknown): {
  items: Project[];
  next_cursor: string | null;
} {
  const page = value as { items?: unknown; next_cursor?: unknown } | null;
  if (
    !page ||
    !Array.isArray(page.items) ||
    page.items.length > 50 ||
    !(
      page.next_cursor === null ||
      (typeof page.next_cursor === "string" &&
        page.next_cursor.length > 0 &&
        page.next_cursor.length <= 32768)
    )
  )
    throw new RequestError("invalid_receipt");
  const items = page.items.map((item: unknown) => requireProject(item));
  if (
    new Set(items.map((p) => p.id)).size !== items.length ||
    (items.length === 0 && page.next_cursor !== null)
  )
    throw new RequestError("invalid_receipt");
  return { items, next_cursor: page.next_cursor } as {
    items: Project[];
    next_cursor: string | null;
  };
}

export function canvasHref(project: Project): string {
  if (!isProjectID(project.id) || !isProjectID(project.canvas_id))
    throw new RequestError("invalid_receipt");
  return `/canvas?project_id=${project.id}&canvas_id=${project.canvas_id}`;
}

export function projectErrorMessage(error: unknown) {
  if (error instanceof RequestError) {
    if (error.status === 401) return "登录已失效，请重新登录。";
    if (error.status === 404) return "项目或画布不存在，或当前账号无权访问。";
    if (error.status === 409)
      return "该创建请求已用于其他内容，请刷新项目列表确认结果。";
    if (error.code === "invalid_receipt")
      return "项目回执不完整，结果未确认，请刷新列表或重试。";
    if (error.status === 422)
      return "项目参数不符合要求，请检查名称、画幅与风格。";
  }
  return "请求结果未确认。请重试原请求，或刷新项目列表确认是否已创建。";
}
