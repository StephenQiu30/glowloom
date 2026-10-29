import manifest from "../../../../public/director/catalog/manifest.json";

export const DIRECTOR_MODEL_CATEGORIES = [
  ["furniture", "家具"],
  ["architecture", "建筑"],
  ["nature", "自然"],
  ["street", "街道"],
  ["kitchen", "厨房"],
  ["bathroom", "卫浴"],
  ["office", "办公"],
  ["everyday", "日用"],
] as const;
export type DirectorModelCategory =
  (typeof DIRECTOR_MODEL_CATEGORIES)[number][0];
export type DirectorBuiltinModel = {
  id: string;
  name: string;
  category: DirectorModelCategory;
  categoryLabel: string;
  url: string;
  thumbnail: string;
  heightM: number;
  source: string;
  license: string;
};
/** Local, versioned CC0 assets; persisted documents store only an allowlisted id. */
export const DIRECTOR_MODELS: readonly DirectorBuiltinModel[] = manifest.map(
  (model) => ({ ...model, category: model.category as DirectorModelCategory }),
);
export function directorBuiltinModel(id: string): DirectorBuiltinModel {
  const model = DIRECTOR_MODELS.find((model) => model.id === id);
  if (!model) throw new Error("简模预设不存在");
  return model;
}
