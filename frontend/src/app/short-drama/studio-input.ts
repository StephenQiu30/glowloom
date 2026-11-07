import { subscribeContextChange } from "@/lib/request";
import type { PersonalReferenceItem } from "@/components/media/personal-reference-picker";
import type { StyleDetail } from "@/components/styles/style-api";

export const redrawMarkets = [
  ["原片语言", "沿用原片"],
  ["中国大陆", "中国大陆"],
  ["海外", "海外"],
] as const;

type StudioReferences = {
  references?: PersonalReferenceItem[];
  stylePreset?: Readonly<StyleDetail>;
};
export type StudioInput = StudioReferences &
  (
    | { kind: "create"; prompt: string }
    | { kind: "upload"; text: string; file?: File }
    | { kind: "redraw"; file?: File; style: string; market: string }
  );

export function studioStyleText(style?: Readonly<StyleDetail>) {
  return style
    ? [
        `风格：${style.name}`,
        style.prompt_fragment,
        style.negative_prompt && `避免：${style.negative_prompt}`,
      ]
        .filter(Boolean)
        .join("\n")
    : "";
}

export function studioReferenceItems(input?: StudioInput) {
  const refs = [
    ...(input?.references ?? []).map((item) => ({
      id: item.id,
      revision: item.revision,
      asset_id: item.asset_id,
      asset_revision: item.media.revision,
    })),
    ...(input?.stylePreset?.reference_items ?? []).map((item) => ({
      id: item.id,
      revision: item.revision,
      asset_id: item.asset_id,
      asset_revision: item.asset_revision,
    })),
  ];
  const unique = new Map<string, (typeof refs)[number]>();
  for (const item of refs) {
    const existing = unique.get(item.id);
    if (
      existing &&
      (existing.revision !== item.revision ||
        existing.asset_id !== item.asset_id ||
        existing.asset_revision !== item.asset_revision)
    )
      throw new Error("风格与参考素材版本不同，请重新选择。");
    unique.set(item.id, item);
  }
  return [...unique.values()];
}

/** Validate the combined snapshot before creating a project. */
export function validateStudioInputReferences(input?: StudioInput) {
  try {
    studioReferenceItems(input);
  } catch (error) {
    return error instanceof Error
      ? error.message
      : "参考素材版本不一致，请重新选择。";
  }
  const kinds = new Map<string, string>();
  for (const item of input?.references ?? []) kinds.set(item.id, item.kind);
  for (const item of input?.stylePreset?.reference_items ?? [])
    kinds.set(item.id, "image");
  for (const [kind, label] of [
    ["image", "张图片"],
    ["video", "个视频"],
    ["document", "份小说文档"],
  ] as const)
    if ([...kinds.values()].filter((value) => value === kind).length > 30)
      return `最多添加30${label}，包含风格中的参考图。`;
  return "";
}

let pending:
  { owner: string; projectID: string; input: StudioInput } | undefined;

subscribeContextChange(() => {
  pending = undefined;
});

export function saveStudioInput(
  owner: string,
  projectID: string,
  input: StudioInput,
) {
  pending = { owner, projectID, input };
}

export function readStudioInput(owner: string, projectID: string) {
  if (pending?.owner === owner && pending.projectID === projectID) {
    return pending.input;
  }
}

export function clearStudioInput(input: StudioInput) {
  if (pending?.input === input) pending = undefined;
}
