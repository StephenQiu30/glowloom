import {
  assetDateRange,
  assetKinds,
  defaultAssetFilters,
  type AssetFilters,
  type AssetKind,
  type AssetSource,
} from "./asset-query";
import type { LibraryFilter } from "./library-toolbar";

export type AssetLocation = {
  view: AssetSource | "reference";
  kind: AssetKind;
  collection: "current" | "legacy";
  filters: AssetFilters;
  reference?: LibraryFilter;
};

const filterChoices = {
  image: {
    processTypes: ["super_resolution"],
    resolutions: ["1K", "2K", "4K", "8K"],
    ratios: ["21:9", "16:9", "3:2", "4:3", "1:1", "3:4", "2:3", "9:16"],
    generationModes: [],
  },
  video: {
    processTypes: ["super_resolution", "frame_interpolation"],
    resolutions: ["720P", "1080P", "2K", "4K"],
    ratios: ["21:9", "16:9", "4:3", "1:1", "3:4", "9:16"],
    generationModes: [
      "all_reference",
      "first_last_frame",
      "multi_frame",
      "lip_sync",
      "motion_mimic",
    ],
  },
  audio: {
    processTypes: [],
    resolutions: [],
    ratios: [],
    generationModes: ["song", "instrumental", "tts"],
  },
  document: {
    processTypes: [],
    resolutions: [],
    ratios: [],
    generationModes: [],
  },
} satisfies Record<AssetKind, Record<string, string[]>>;

const arrayParams = {
  processTypes: "process_types",
  resolutions: "resolutions",
  ratios: "ratios",
  generationModes: "generation_modes",
} as const;
const locationKeys = [
  "view",
  "kind",
  "collection",
  "q",
  "from",
  "through",
  "order",
  "sort",
  "favorite",
  ...Object.values(arrayParams),
  "reference_kind",
  "reference_order",
  "reference_folder",
  "reference_trash",
  "reference_recent",
];

export function readReferenceLocation(params: URLSearchParams): LibraryFilter {
  const kind = params.get("reference_kind") ?? "image";
  const order = params.get("reference_order") ?? "updated_desc";
  const trash = params.get("reference_trash") === "1";
  return {
    kind: ["image", "video", "audio", "document", "model"].includes(kind)
      ? kind
      : "image",
    search: params.get("q")?.trim() ?? "",
    order: ["updated_desc", "updated_asc", "name_asc"].includes(order)
      ? order
      : "updated_desc",
    favorite: !trash && params.get("favorite") === "1",
    trash,
    recent: params.get("reference_recent") === "1",
    folder: params.get("reference_folder") ?? "",
  };
}

export function writeReferenceLocation(
  value: LibraryFilter,
  existing: URLSearchParams,
) {
  const params = new URLSearchParams(existing);
  for (const key of locationKeys) params.delete(key);
  params.set("view", "reference");
  if (value.kind !== "image") params.set("reference_kind", value.kind);
  if (value.search) params.set("q", value.search);
  if (value.order !== "updated_desc")
    params.set("reference_order", value.order);
  if (value.favorite && !value.trash) params.set("favorite", "1");
  if (value.trash) params.set("reference_trash", "1");
  if (value.recent) params.set("reference_recent", "1");
  if (value.folder) params.set("reference_folder", value.folder);
  return params.toString();
}

/** Only known, type-compatible conditions may be restored from a bookmark. */
export function readAssetLocation(params: URLSearchParams): AssetLocation {
  const requestedView = params.get("view");
  const view =
    requestedView === "canvas" ||
    requestedView === "short_drama" ||
    requestedView === "reference"
      ? requestedView
      : "history";
  const kind =
    assetKinds.find((item) => item.id === params.get("kind"))?.id ?? "image";
  const filters: AssetFilters = {
    ...defaultAssetFilters,
    search: params.get("q")?.trim() ?? "",
    from: params.get("from") ?? "",
    through: params.get("through") ?? "",
    order: params.get("order") === "asc" ? "asc" : "desc",
    sort:
      view !== "history" && params.get("sort") === "created"
        ? "created"
        : "updated",
    favorite: params.get("favorite") === "1",
  };
  try {
    assetDateRange(filters.from, filters.through);
  } catch {
    filters.from = "";
    filters.through = "";
  }
  for (const field of Object.keys(
    arrayParams,
  ) as (keyof typeof arrayParams)[]) {
    const choices: string[] =
      view === "history" ? filterChoices[kind][field] : [];
    filters[field] = [
      ...new Set(
        (params.get(arrayParams[field]) ?? "")
          .split(",")
          .filter((value) => choices.includes(value)),
      ),
    ];
  }
  return {
    view,
    kind,
    collection:
      view === "canvas" && params.get("collection") === "legacy"
        ? "legacy"
        : "current",
    filters,
    ...(view === "reference"
      ? { reference: readReferenceLocation(params) }
      : {}),
  };
}

/** Preserve unrelated parameters (including standalone acceptance-fixture scenarios). */
export function writeAssetLocation(
  value: AssetLocation,
  existing = new URLSearchParams(),
) {
  if (value.view === "reference" && value.reference)
    return writeReferenceLocation(value.reference, existing);
  const params = new URLSearchParams(existing);
  for (const key of locationKeys) params.delete(key);
  if (value.view !== "history") params.set("view", value.view);
  if (value.view === "history" && value.kind !== "image")
    params.set("kind", value.kind);
  if (value.view === "canvas" && value.collection === "legacy")
    params.set("collection", "legacy");
  const { filters } = value;
  if (filters.search) params.set("q", filters.search);
  if (filters.from) params.set("from", filters.from);
  if (filters.through) params.set("through", filters.through);
  if (filters.order === "asc") params.set("order", "asc");
  if (value.view !== "history" && filters.sort === "created")
    params.set("sort", "created");
  if (filters.favorite) params.set("favorite", "1");
  if (value.view === "history")
    for (const field of Object.keys(
      arrayParams,
    ) as (keyof typeof arrayParams)[]) {
      if (filters[field].length)
        params.set(arrayParams[field], filters[field].join(","));
    }
  return params.toString();
}
