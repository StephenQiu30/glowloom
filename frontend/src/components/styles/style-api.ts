import { RequestError } from "@/lib/request";
import {
  createStyleLibraryItem,
  deleteStyleLibraryItem,
  getStyleLibraryItem,
  listStyleLibrary,
  updateStyleLibraryItem,
} from "@/api/styles";
import { isProjectID } from "@/components/projects/project";

export type StyleCategory = "realistic" | "2d" | "3d";
export type StyleSummary = {
  id: string;
  name: string;
  category: StyleCategory;
  revision: number;
  cover_asset_id: string | null;
  reference_asset_ids: readonly string[];
  owned: boolean;
  can_edit: boolean;
  cover_item_id?: string;
  references_available: boolean;
  reference_items: readonly {
    id: string;
    revision: number;
    asset_id: string;
    asset_revision: number;
    title: string;
  }[];
};
export type StyleDetail = StyleSummary & {
  prompt_fragment: string;
  negative_prompt: string;
};
export type StylePage = { items: StyleSummary[]; next_cursor: string | null };
export type StyleQuery = {
  q?: string;
  category?: StyleCategory;
  mine?: boolean;
  limit?: number;
  cursor?: string;
};
export type StyleInput = Pick<
  StyleDetail,
  "name" | "category" | "prompt_fragment" | "reference_asset_ids"
>;
export type StyleReceipt = {
  request_id: string;
  action: "create" | "patch" | "delete";
  preset: StyleDetail;
  deleted: boolean;
};
function summary(raw: unknown): StyleSummary {
  const value = raw as Partial<StyleSummary> | null;
  const refs = value?.reference_asset_ids;
  if (
    !value ||
    !isProjectID(value.id) ||
    typeof value.name !== "string" ||
    !value.name.trim() ||
    Array.from(value.name).length > 20 ||
    !["realistic", "2d", "3d"].includes(value.category ?? "") ||
    !Number.isSafeInteger(value.revision) ||
    Number(value.revision) < 1 ||
    typeof value.owned !== "boolean" ||
    typeof value.can_edit !== "boolean" ||
    (value.can_edit && !value.owned) ||
    !Array.isArray(refs) ||
    refs.length > 4 ||
    refs.some((id) => !isProjectID(id)) ||
    new Set(refs).size !== refs.length ||
    value.cover_asset_id !== (refs[0] ?? null) ||
    typeof value.references_available !== "boolean" ||
    !Array.isArray(value.reference_items) ||
    (value.references_available
      ? value.reference_items.length !== refs.length ||
        value.reference_items.some(
          (item, index) =>
            !isProjectID(item.id) ||
            item.asset_id !== refs[index] ||
            !Number.isSafeInteger(item.revision) ||
            item.revision < 1 ||
            !Number.isSafeInteger(item.asset_revision) ||
            item.asset_revision < 1 ||
            typeof item.title !== "string",
        ) ||
        value.cover_item_id !== value.reference_items[0]?.id
      : value.reference_items.length !== 0 || value.cover_item_id !== undefined)
  )
    throw new RequestError("invalid_receipt");
  return value as StyleSummary;
}
function detail(raw: unknown, requireReferences = true): StyleDetail {
  const value = summary(raw) as StyleDetail;
  if (
    typeof value.prompt_fragment !== "string" ||
    !value.prompt_fragment.trim() ||
    Array.from(value.prompt_fragment).length > 2000 ||
    typeof value.negative_prompt !== "string"
  )
    throw new RequestError("invalid_receipt");
  if (requireReferences && !value.references_available)
    throw new RequestError("style_reference_unavailable", 409);
  return value;
}
export async function listStyles(
  query: StyleQuery,
  signal?: AbortSignal,
): Promise<StylePage> {
  const value = await listStyleLibrary(query, { signal });
  const page = value as Partial<StylePage> | null;
  if (
    !page ||
    !Array.isArray(page.items) ||
    page.items.length > (query.limit ?? 24) ||
    (page.next_cursor !== null &&
      (typeof page.next_cursor !== "string" || !page.next_cursor))
  )
    throw new RequestError("invalid_receipt");
  const items = page.items.map(summary);
  if (new Set(items.map((item) => item.id)).size !== items.length)
    throw new RequestError("invalid_receipt");
  return { items, next_cursor: page.next_cursor } as StylePage;
}
export async function readStyle(
  id: string,
  signal?: AbortSignal,
): Promise<StyleDetail> {
  const value = detail(await getStyleLibraryItem({ id }, { signal }), false);
  if (value.id !== id) throw new RequestError("invalid_receipt");
  return value;
}
/** Freeze the selected revision; later library edits must not mutate a creation draft. */
export function freezeStyle(value: StyleDetail): Readonly<StyleDetail> {
  return Object.freeze({
    ...detail(value),
    reference_asset_ids: Object.freeze([...value.reference_asset_ids]),
    reference_items: Object.freeze(
      value.reference_items.map((item) => Object.freeze({ ...item })),
    ),
  });
}
async function mutate(
  action: StyleReceipt["action"],
  id: string | undefined,
  input:
    | StyleInput
    | (StyleInput & { expected_revision: number })
    | { expected_revision: number },
  key: string,
  signal?: AbortSignal,
): Promise<StyleReceipt> {
  const options = { headers: { "Idempotency-Key": key }, signal };
  let raw: unknown;
  if (action === "create" && "name" in input) {
    raw = await createStyleLibraryItem(
      { ...input, reference_asset_ids: [...input.reference_asset_ids] },
      options,
    );
  } else if (
    action === "patch" &&
    id &&
    "name" in input &&
    "expected_revision" in input
  ) {
    raw = await updateStyleLibraryItem(
      { id },
      { ...input, reference_asset_ids: [...input.reference_asset_ids] },
      options,
    );
  } else if (action === "delete" && id && "expected_revision" in input) {
    raw = await deleteStyleLibraryItem(
      { id },
      { expected_revision: input.expected_revision },
      options,
    );
  } else throw new RequestError("invalid_request", 422);
  const result = raw as Partial<StyleReceipt> | null;
  if (
    !result ||
    result.request_id !== key ||
    result.action !== action ||
    result.deleted !== (action === "delete")
  )
    throw new RequestError("invalid_receipt");
  const preset = detail(result.preset, action !== "delete");
  if (id && preset.id !== id) throw new RequestError("invalid_receipt");
  return { request_id: key, action, preset, deleted: action === "delete" };
}
export function createStyle(
  input: StyleInput,
  key: string,
  signal?: AbortSignal,
) {
  return mutate("create", undefined, input, key, signal);
}
export function updateStyle(
  id: string,
  input: StyleInput & { expected_revision: number },
  key: string,
  signal?: AbortSignal,
) {
  return mutate("patch", id, input, key, signal);
}
export function deleteStyle(
  id: string,
  revision: number,
  key: string,
  signal?: AbortSignal,
) {
  return mutate("delete", id, { expected_revision: revision }, key, signal);
}
export function styleError(error: unknown) {
  if (error instanceof RequestError) {
    if (error.code === "style_preset_in_use")
      return "风格仍被使用，暂不能删除。当前风格已保留。";
    if (error.code === "style_reference_unavailable")
      return "风格参考图已不可用，请重新选择图片后再保存。";
    if (error.code === "invalid_receipt") return "风格回执不完整，请重新读取。";
    if (error.status === 401) return "登录已失效，请重新登录后重试。";
    if (error.status === 403 || error.status === 404)
      return "风格已不存在或当前账号无权访问，请刷新后重试。";
    if (error.status === 409)
      return "风格已被更新，请读取最新版本后重试。当前内容已保留。";
    if (error.status === 422)
      return "风格内容或参考图不符合要求，请检查后重试。";
  }
  return "操作结果尚未确认，请保留当前内容并重试原请求。";
}
