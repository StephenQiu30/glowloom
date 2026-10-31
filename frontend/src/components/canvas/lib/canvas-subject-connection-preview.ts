import type { UnifiedSubject } from "@/components/subjects/unified-contracts";
import type { CanvasNodeData } from "../types/canvas";
import {
  materializeSubject,
  subjectConfig,
  subjectLocalMedia,
  type CanvasSubjectLocal,
} from "../subject/subject-node-state";
import { canvasConnectionMediaAssetId } from "./canvas-timeline-connections";

export type CanvasSubjectPreviewOptions = {
  target?: CanvasNodeData;
  pinnedSubject?: UnifiedSubject;
  assets?: ReadonlyMap<string, API.AssetSummary>;
};

/** Reject known conflicts synchronously; unresolved facts remain mandatory command checks. */
export function assertCanvasSubjectSourcePreview(
  sources: readonly CanvasNodeData[],
  options: CanvasSubjectPreviewOptions = {},
): { requiresMediaValidation: boolean; requiresSubjectValidation: boolean } {
  if (
    !sources.length ||
    sources.length > 5 ||
    new Set(sources.map((source) => source.id)).size !== sources.length
  )
    throw new Error("请选择1至5个不重复的来源节点");
  let local: CanvasSubjectLocal = { description: "", references: [] };
  let requiresSubjectValidation = false;
  if (options.target) {
    const config = subjectConfig(options.target);
    requiresSubjectValidation =
      !!config.subject_id && !config.local && !options.pinnedSubject;
    if (!requiresSubjectValidation)
      local = materializeSubject(config, options.pinnedSubject);
  }
  const assets = new Set(
    subjectLocalMedia(local).map((media) => media.asset_id),
  );
  let requiresMediaValidation = false;
  for (const source of sources) {
    if (
      source.id === options.target?.id ||
      source.metadata?.status === "loading" ||
      source.metadata?.status === "error"
    )
      throw new Error("来源素材尚未就绪");
    const id = canvasConnectionMediaAssetId(source);
    if (assets.has(id)) throw new Error("主体素材重复，请调整选择");
    assets.add(id);
    const asset = options.assets?.get(id);
    if (
      asset &&
      (asset.id !== id ||
        asset.kind !== source.type ||
        !Number.isSafeInteger(asset.revision) ||
        asset.revision! < 1)
    )
      throw new Error("素材身份或版本尚未确认");
    const duration = asset ? asset.duration_ms : source.metadata?.durationMs;
    if (source.type === "image") continue;
    if (duration === undefined) {
      requiresMediaValidation = true;
      continue;
    }
    if (
      !Number.isSafeInteger(duration) ||
      (source.type === "audio"
        ? duration < 2000 || duration > 5000
        : duration <= 0 || Math.round(duration / 1000) > 15)
    )
      throw new Error(
        source.type === "audio"
          ? "音色素材时长应为2至5秒"
          : "视频素材时长尚未确认或超过15秒",
      );
  }
  const imageCount = sources.filter((source) => source.type === "image").length;
  const audioCount = sources.filter((source) => source.type === "audio").length;
  const videoCount = sources.filter((source) => source.type === "video").length;
  if (requiresSubjectValidation) {
    // A pinned library subject already has a primary. Its exact remaining
    // capacity is resolved asynchronously; it can never accept a second video.
    if (videoCount || imageCount > 3 || audioCount > 1)
      throw new Error("当前来源组合超出主体连接容量");
    return { requiresMediaValidation, requiresSubjectValidation };
  }
  const mainIndex = local.primary
    ? -1
    : sources.findIndex((source) => source.type !== "audio");
  const mainKind = local.primary?.kind ?? sources[mainIndex]?.type;
  if (!mainKind) throw new Error("请先添加图片或视频主来源");
  const remaining = sources.filter((_, index) => index !== mainIndex);
  if (mainKind === "video") {
    if (remaining.length || local.references.length || local.voice)
      throw new Error("视频主来源不能追加辅助素材或音色");
  } else {
    if (remaining.some((source) => source.type === "video"))
      throw new Error("图片主来源只能追加图片辅助来源或音色");
    if (
      local.references.length +
        remaining.filter((source) => source.type === "image").length >
      3
    )
      throw new Error("图片主体最多添加3个辅助来源");
    if (
      Number(!!local.voice) +
        remaining.filter((source) => source.type === "audio").length >
      1
    )
      throw new Error("主体已有音色，请先移除原音色");
  }
  return { requiresMediaValidation, requiresSubjectValidation };
}
