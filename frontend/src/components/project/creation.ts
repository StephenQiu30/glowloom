export const styleSubtypes = [
  ["anime_jp", "日系动漫"],
  ["guofeng_xianxia", "国风仙侠"],
  ["cartoon_3d", "3D 卡通"],
  ["manhwa", "韩漫"],
] as const;
export type StyleSubtype = (typeof styleSubtypes)[number][0];
export type ProjectDraft = {
  name: string;
  description: string;
  aspect_ratio: "9:16" | "16:9";
  style_type: "realistic" | "stylized";
  style_subtype: StyleSubtype | "";
  style_preset_id: string;
};
export type StylePreset = {
  id: string;
  name: string;
  style_type: ProjectDraft["style_type"];
  style_subtype?: StyleSubtype;
};
export type CreationBody = Omit<
  ProjectDraft,
  "style_subtype" | "style_preset_id"
> & {
  style_subtype?: StyleSubtype;
  style_preset_id?: string;
};
export const initialProjectDraft: ProjectDraft = {
  name: "",
  description: "",
  aspect_ratio: "16:9",
  style_type: "realistic",
  style_subtype: "",
  style_preset_id: "",
};
export function updateProjectDraft(
  draft: ProjectDraft,
  patch: Partial<ProjectDraft>,
): ProjectDraft {
  const changedStyle =
    (patch.style_type !== undefined && patch.style_type !== draft.style_type) ||
    (patch.style_subtype !== undefined &&
      patch.style_subtype !== draft.style_subtype);
  const next = { ...draft, ...patch };
  if (changedStyle) next.style_preset_id = "";
  if (next.style_type === "realistic") next.style_subtype = "";
  return next;
}
export function matchesPreset(draft: ProjectDraft, preset: StylePreset) {
  return (
    draft.style_type === preset.style_type &&
    (draft.style_type === "realistic" ||
      draft.style_subtype === preset.style_subtype)
  );
}
export function creationBody(
  draft: ProjectDraft,
  presets: readonly StylePreset[],
): {
  body?: CreationBody;
  errors: Partial<Record<keyof ProjectDraft, string>>;
} {
  const errors: Partial<Record<keyof ProjectDraft, string>> = {};
  const name = draft.name.trim();
  if (!name || Array.from(name).length > 50)
    errors.name = "项目名称需为 1–50 个字符。";
  if (
    draft.style_type === "stylized" &&
    !styleSubtypes.some(([value]) => value === draft.style_subtype)
  )
    errors.style_subtype = "请选择一种子风格。";
  if (
    draft.style_preset_id &&
    !presets.some(
      (preset) =>
        preset.id === draft.style_preset_id && matchesPreset(draft, preset),
    )
  )
    errors.style_preset_id = "这个预设当前不可用，请重新选择或不使用预设。";
  if (Object.keys(errors).length) return { errors };
  return {
    errors,
    body: {
      name,
      description: draft.description,
      aspect_ratio: draft.aspect_ratio,
      style_type: draft.style_type,
      ...(draft.style_type === "stylized" && draft.style_subtype
        ? { style_subtype: draft.style_subtype }
        : {}),
      ...(draft.style_preset_id
        ? { style_preset_id: draft.style_preset_id }
        : {}),
    },
  };
}
