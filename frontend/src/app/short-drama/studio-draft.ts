import {
  emptyPromptDraft,
  type MediaKind,
} from "@/components/generation/prompt-document";
import { studioStyleText, type StudioInput } from "./studio-input";

export function createStudioDraft(
  input?: StudioInput,
  assets: readonly API.AssetSummary[] = [],
) {
  const draft = emptyPromptDraft();
  const text = [
    "请根据我的创作想法和参考素材写一部短剧。请按「第1集」这样的标题分集，并为每集写出场景、动作和对白。",
    input?.kind === "create" ? input.prompt : "",
    studioStyleText(input?.stylePreset),
  ]
    .filter(Boolean)
    .join("\n\n");
  draft.document.content = [
    { type: "paragraph", content: [{ type: "text", text }] },
  ];
  for (const asset of assets) {
    if (
      !asset.id ||
      !Number.isSafeInteger(asset.revision) ||
      !["image", "video", "document"].includes(asset.kind ?? "")
    )
      throw new Error("参考素材尚未完成项目绑定。");
    const kind = asset.kind as MediaKind;
    const id = crypto.randomUUID();
    const ordinal = (draft.labelState.highWater[kind] ?? 0) + 1;
    draft.labelState.highWater[kind] = ordinal;
    draft.labelState.allocations[id] = { labelKind: kind, ordinal };
    draft.references.push({
      id,
      bindingGeneration: 1,
      mediaKind: kind,
      role: `reference_${kind}`,
      order: draft.references.length,
      source: {
        kind: "media",
        sourceId: asset.id,
        sourceVersion: String(asset.revision),
      },
    });
  }
  return draft;
}
