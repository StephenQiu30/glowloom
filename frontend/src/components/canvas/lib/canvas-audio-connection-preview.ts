import type {
  PromptDraft,
  MediaKind,
} from "@/components/generation/prompt-document";
import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
import type { CanvasNodeData } from "../types/canvas";
import { subjectConfig } from "../subject/subject-node-state";
import {
  assertCanvasAudioConnectionCapability,
  createCanvasAudioConnectionDraft,
  type CanvasAudioCatalogIdentity,
  type CanvasAudioConnectionInput,
  type CanvasAudioModelCatalog,
} from "./canvas-audio-connection-capability";

/** Empty ordinary nodes are valid planning sources; subjects need actual resolved media. */
export function canvasAudioSourceInputs(
  sources: readonly CanvasNodeData[],
  pinnedSubjects?: ReadonlyMap<string, UnifiedSubject>,
): CanvasAudioConnectionInput[] {
  return sources.map((source) => {
    if (["image", "video", "audio"].includes(source.type))
      return {
        role: `reference_${source.type}`,
        mediaKinds: [source.type as MediaKind],
      };
    if (source.type !== "subject") throw new Error("此内容无法连接到音频节点");
    const config = subjectConfig(source);
    const pinned = pinnedSubjects?.get(source.id);
    const subject =
      config.local ??
      (pinned?.id === config.subject_id &&
      pinned?.version_id === config.version_id
        ? pinned
        : undefined);
    if (!subject?.primary) throw new Error("主体素材能力尚未就绪，请稍后重试");
    const media = [
      subject.primary,
      ...subject.references,
      ...(subject.voice ? [subject.voice.media] : []),
    ];
    if (
      media.some(
        (item) =>
          !item?.asset_id ||
          !Number.isSafeInteger(item.asset_revision) ||
          item.asset_revision! < 1 ||
          !["image", "video", "audio"].includes(item.kind ?? ""),
      )
    )
      throw new Error("主体参考素材无效");
    return {
      role: "subject",
      mediaKinds: media.map((item) => item!.kind as MediaKind),
    };
  });
}

/** Return exactly the composer the caller must persist with a newly connected audio node. */
export function previewCanvasAudioConnection(
  catalog: CanvasAudioModelCatalog,
  identity: CanvasAudioCatalogIdentity,
  sources: readonly CanvasNodeData[],
  target?: CanvasNodeData,
  pinnedSubjects?: ReadonlyMap<string, UnifiedSubject>,
): PromptDraft {
  if (target && target.type !== "audio")
    throw new Error("连接目标不是音频节点");
  const draft = target
    ? (target.serviceData?.config?.draft?.composer as PromptDraft | undefined)
    : createCanvasAudioConnectionDraft(catalog, identity).draft;
  assertCanvasAudioConnectionCapability(
    catalog,
    identity,
    draft,
    canvasAudioSourceInputs(sources, pinnedSubjects),
  );
  return structuredClone(draft!);
}
