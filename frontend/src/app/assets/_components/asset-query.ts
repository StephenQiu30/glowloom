import { RequestError } from "@/lib/request";
import { isProjectID } from "@/components/projects/project";
import { taskLabels } from "@/components/generation/contracts";

export const assetKinds = [
  { id: "image", label: "图片" },
  { id: "video", label: "视频" },
  { id: "audio", label: "音频" },
  { id: "document", label: "文档" },
] as const;
export type AssetKind = (typeof assetKinds)[number]["id"];
export type AssetSource = "history" | "canvas" | "short_drama";
export type AssetFilters = {
  search: string;
  from: string;
  through: string;
  order: "asc" | "desc";
  sort: "created" | "updated";
  favorite: boolean;
  processTypes: string[];
  resolutions: string[];
  ratios: string[];
  generationModes: string[];
};

export const defaultAssetFilters: AssetFilters = {
  search: "",
  from: "",
  through: "",
  order: "desc",
  sort: "updated",
  favorite: false,
  processTypes: [],
  resolutions: [],
  ratios: [],
  generationModes: [],
};

export function hasAssetFilters(value: AssetFilters) {
  return !!(
    value.search ||
    value.from ||
    value.through ||
    value.favorite ||
    value.processTypes.length ||
    value.resolutions.length ||
    value.ratios.length ||
    value.generationModes.length
  );
}

export function historyFilterParams(value: AssetFilters) {
  return {
    search: value.search || undefined,
    ...assetDateRange(value.from, value.through),
    favorite_only: value.favorite || undefined,
    process_types: value.processTypes.join(",") || undefined,
    resolutions: value.resolutions.join(",") || undefined,
    ratios: value.ratios.join(",") || undefined,
    generation_modes: value.generationModes.join(",") || undefined,
    order: value.order,
  };
}

function calendarDate(value: string) {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) throw new Error("请选择有效日期。");
  const [, year, month, day] = match.map(Number);
  const date = new Date(year, month - 1, day);
  if (
    date.getFullYear() !== year ||
    date.getMonth() !== month - 1 ||
    date.getDate() !== day
  )
    throw new Error("请选择有效日期。");
  return date;
}

/** The displayed ending day is inclusive; the API upper bound is exclusive. */
export function assetDateRange(from: string, through: string) {
  const start = from ? calendarDate(from) : undefined;
  const end = through ? calendarDate(through) : undefined;
  if (start && end && start > end)
    throw new Error("开始日期不能晚于结束日期。");
  if (end) end.setDate(end.getDate() + 1);
  return {
    created_from: start?.toISOString(),
    created_before: end?.toISOString(),
  };
}

export type HistoryOutput = NonNullable<
  API.PersonalHistoryItem["outputs"]
>[number] & {
  id: string;
  favorite: boolean;
  metadata_revision: number;
  resolution: string;
  ratio: string;
  generation_mode: string;
  process_types: string[];
  can_download: boolean;
  can_publish: boolean;
};
type HistoryItem = Omit<API.PersonalHistoryItem, "outputs"> & {
  id: string;
  project_id: string;
  status: string;
  create_time: string;
  prompt_preview: string;
  history_revision: number;
  outputs: HistoryOutput[];
};

function validCursor(value: unknown): value is string | null {
  return (
    value === null ||
    (typeof value === "string" && value.length > 0 && value.length <= 2048)
  );
}

export function requirePersonalHistory(value: API.PersonalHistoryPageResponse) {
  if (
    !value ||
    !Array.isArray(value.items) ||
    value.items.length > 200 ||
    !validCursor(value.next_cursor)
  )
    throw new RequestError("invalid_receipt");
  const ids = new Set<string>();
  for (const item of value.items) {
    if (
      !item ||
      !isProjectID(item.id) ||
      ids.has(item.id) ||
      !isProjectID(item.project_id) ||
      typeof item.capability !== "string" ||
      !item.capability ||
      !Object.hasOwn(taskLabels, item.status ?? "") ||
      !Number.isFinite(Date.parse(item.create_time ?? "")) ||
      typeof item.prompt_preview !== "string" ||
      !Number.isSafeInteger(item.history_revision) ||
      item.history_revision! < 0 ||
      !Array.isArray(item.outputs) ||
      new Set(item.outputs.map((output) => output?.id)).size !==
        item.outputs.length ||
      item.outputs.some(
        (output) =>
          !output ||
          !isProjectID(output.id) ||
          typeof output.favorite !== "boolean" ||
          typeof output.can_download !== "boolean" ||
          typeof output.can_publish !== "boolean" ||
          !Number.isSafeInteger(output.metadata_revision) ||
          output.metadata_revision! < 0 ||
          typeof output.resolution !== "string" ||
          typeof output.ratio !== "string" ||
          typeof output.generation_mode !== "string" ||
          !Array.isArray(output.process_types) ||
          output.process_types.some((process) => typeof process !== "string") ||
          ![
            "image",
            "video",
            "audio",
            "model",
            "json",
            "text",
            "document",
          ].includes(output.kind ?? "") ||
          (output.media_asset_id != null &&
            (!isProjectID(output.media_asset_id) ||
              output.moderation_status !== "passed")),
      )
    )
      throw new RequestError("invalid_receipt");
    ids.add(item.id);
  }
  return {
    items: value.items as HistoryItem[],
    next_cursor: value.next_cursor,
  };
}

export function requireAssetCanvases(
  value: API.AssetListResponse,
  kind: "canvas" | "short_drama",
  collection: "current" | "legacy" = "current",
) {
  if (
    !value ||
    !Array.isArray(value.items) ||
    value.items.length > 200 ||
    !validCursor(value.next_cursor)
  )
    throw new RequestError("invalid_receipt");
  const ids = new Set<string>();
  for (const item of value.items) {
    if (
      !item ||
      !isProjectID(item.id) ||
      ids.has(item.id) ||
      !isProjectID(item.project_id) ||
      item.kind !== kind ||
      item.collection !== collection ||
      typeof item.favorite !== "boolean" ||
      typeof item.can_copy_project !== "boolean" ||
      typeof item.cover_unavailable !== "boolean" ||
      typeof item.is_main !== "boolean" ||
      typeof item.favorite_revision !== "string" ||
      !/^(0|[1-9]\d*)$/.test(item.favorite_revision) ||
      !Number.isSafeInteger(item.project_revision) ||
      item.project_revision! < 1 ||
      !(item.cover_asset_id === null || isProjectID(item.cover_asset_id)) ||
      typeof item.name !== "string" ||
      !item.name.trim() ||
      typeof item.revision !== "string" ||
      !/^[1-9]\d*$/.test(item.revision) ||
      !Number.isFinite(Date.parse(item.created_at ?? "")) ||
      !Number.isFinite(Date.parse(item.updated_at ?? ""))
    )
      throw new RequestError("invalid_receipt");
    ids.add(item.id);
  }
  return {
    items: value.items as AssetCanvasItem[],
    next_cursor: value.next_cursor,
  };
}

export type PersonalHistoryItem = ReturnType<
  typeof requirePersonalHistory
>["items"][number];

export type AssetCanvasItem = API.AssetCanvas & {
  id: string;
  project_id: string;
  name: string;
  revision: string;
  favorite_revision: string;
  project_revision: number;
  favorite: boolean;
  is_main: boolean;
  collection: string;
  can_copy_project: boolean;
  cover_unavailable: boolean;
  cover_asset_id: string | null;
  created_at: string;
  updated_at: string;
};
