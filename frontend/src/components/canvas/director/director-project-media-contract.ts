import { DEFAULT_PAGE_SIZE } from "@/lib/pagination";
import { isProjectID } from "@/components/projects/project";
import { RequestError } from "@/lib/request";
export function directorProjectQuery(
  projectID: string,
  kind: string,
  search: string,
  page: number,
  pageSize: number = DEFAULT_PAGE_SIZE,
): API.listMediaLibraryParams {
  return {
    scope: "project",
    project_id: projectID,
    kind,
    search: search.trim() || undefined,
    page,
    page_size: pageSize,
    catalog_state: "active",
    order: "updated_desc",
  };
}
const integer = (value: unknown, min = 0) =>
  Number.isSafeInteger(value) && Number(value) >= min;
export function requireDirectorProjectPage(
  value: unknown,
  projectID: string,
  kind: string,
  index: number,
  actorID?: string,
  pageSize: number = DEFAULT_PAGE_SIZE,
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
    page.page_size !== pageSize ||
    !integer(page.total) ||
    !Array.isArray(page.items) ||
    page.items.length > pageSize ||
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
    hasMore: page.items.length > 0 && index * pageSize < page.total!,
  };
}
