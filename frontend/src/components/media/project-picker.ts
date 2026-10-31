import { isProjectID } from "@/components/projects/project";
import { RequestError } from "@/lib/request";
export type ProjectPickerFilter = {
  from: string;
  through: string;
  origin: "" | "upload" | "generated" | "system";
  resolutions: string[];
  ratios: string[];
};
export const defaultProjectPickerFilter: ProjectPickerFilter = {
  from: "",
  through: "",
  origin: "",
  resolutions: [],
  ratios: [],
};
export function projectPickerFilterForKind(
  filter: ProjectPickerFilter,
  kind: string,
): ProjectPickerFilter {
  return kind === "image" ? filter : { ...filter, resolutions: [], ratios: [] };
}
export function projectPickerQuery(
  projectID: string,
  kind: string,
  search: string,
  page: number,
  filter: ProjectPickerFilter,
): API.listMediaLibraryParams {
  const from = filter.from ? pickerDate(filter.from) : undefined;
  const before = filter.through ? pickerDate(filter.through) : undefined;
  if (before) before.setDate(before.getDate() + 1);
  if (
    (from && !Number.isFinite(from.getTime())) ||
    (before && !Number.isFinite(before.getTime())) ||
    (from && before && from >= before)
  )
    throw new Error("请选择有效的起止日期。");
  return {
    scope: "project",
    project_id: projectID,
    kind,
    search: search.trim() || undefined,
    page,
    page_size: 40,
    catalog_state: "active",
    order: "updated_desc",
    created_from: from?.toISOString(),
    created_before: before?.toISOString(),
    origin: filter.origin || undefined,
    resolutions:
      kind === "image" ? filter.resolutions.join(",") || undefined : undefined,
    ratios: kind === "image" ? filter.ratios.join(",") || undefined : undefined,
  };
}
function pickerDate(value: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  const date = new Date(`${value}T00:00:00`);
  if (
    !match ||
    !Number.isFinite(date.getTime()) ||
    date.getFullYear() !== Number(match[1]) ||
    date.getMonth() + 1 !== Number(match[2]) ||
    date.getDate() !== Number(match[3])
  )
    throw new Error("请选择有效的起止日期。");
  return date;
}
const integer = (value: unknown, min = 0) =>
  Number.isSafeInteger(value) && Number(value) >= min;
export function requireProjectPickerPage(
  value: unknown,
  projectID: string,
  kind: string,
  index: number,
  actorID?: string,
) {
  const page = value as API.LibraryPage | null;
  if (
    !page ||
    !isProjectID(page.current_actor_id) ||
    (actorID !== undefined && page.current_actor_id !== actorID) ||
    !isProjectID(page.current_org_id) ||
    !isProjectID(page.library_id) ||
    !integer(page.revision) ||
    page.scope?.kind !== "project" ||
    page.scope.project_id !== projectID ||
    page.page !== index ||
    page.page_size !== 40 ||
    !integer(page.total) ||
    !Array.isArray(page.items) ||
    page.items.length > 40 ||
    page.items.length > page.total! ||
    new Set(page.items.map((item) => item.asset_id)).size !==
      page.items.length ||
    page.items.some(
      (item) =>
        !isProjectID(item.id) ||
        !isProjectID(item.asset_id) ||
        item.asset_id !== item.media?.id ||
        item.kind !== kind ||
        item.media?.kind !== kind ||
        item.catalog_state !== "active" ||
        !integer(item.media?.revision, 1),
    )
  )
    throw new RequestError("invalid_receipt");
  return {
    items: page.items.map(
      (item) => ({ ...item.media!, project_id: projectID }) as API.AssetSummary,
    ),
    total: page.total!,
    page: index,
    hasMore: page.items.length > 0 && index * 40 < page.total!,
  };
}
