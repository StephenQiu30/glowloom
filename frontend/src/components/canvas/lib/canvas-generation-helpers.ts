import {
  defaultConfig,
  resolveModelForCapability,
  type AiConfig,
} from "@/components/canvas/runtime/store/use-config-store";
import { resolveImageUrl } from "@/components/canvas/runtime/lib/media/image-storage";
import { referenceUrl } from "@/components/canvas/lib/canvas-node-factory";
import type { NodeGenerationInput } from "@/components/canvas/lib/canvas-node-generation";
import type { CanvasNodeGenerationMode } from "@/components/canvas/nodes/canvas-node-prompt-panel";
import type { CanvasImageAngleParams } from "@/components/canvas/nodes/canvas-node-angle-dialog";
import type { ReferenceImage } from "@/components/canvas/runtime/types/image";
import {
  CanvasNodeType,
  type CanvasConnection,
  type CanvasNodeData,
  type CanvasNodeMetadata,
} from "@/components/canvas/types/canvas";

export function imageExtension(dataUrl: string) {
  return (
    dataUrl.match(/^data:image[/]([^;]+)/)?.[1] ||
    dataUrl.match(/image[/]([^;]+)/)?.[1] ||
    "png"
  );
}

export function audioExtension(mimeType?: string) {
  if (mimeType?.includes("wav")) return "wav";
  if (mimeType?.includes("opus")) return "opus";
  if (mimeType?.includes("aac")) return "aac";
  if (mimeType?.includes("flac")) return "flac";
  if (mimeType?.includes("pcm")) return "pcm";
  return "mp3";
}

export function generationReferenceUrls(context: {
  referenceImages: ReferenceImage[];
  referenceVideos: Array<{ storageKey?: string; url?: string }>;
  referenceAudios?: Array<{ storageKey?: string; url?: string }>;
}) {
  return [
    ...context.referenceImages
      .map(referenceUrl)
      .filter((url): url is string => Boolean(url)),
    ...context.referenceVideos
      .map((video) => video.storageKey || video.url)
      .filter((url): url is string => Boolean(url)),
    ...(context.referenceAudios || [])
      .map((audio) => audio.storageKey || audio.url)
      .filter((url): url is string => Boolean(url)),
  ];
}

export async function resolveMetadataReferences(metadata: CanvasNodeMetadata) {
  if (metadata.generationType !== "edit") return [];
  if (!metadata.references?.length) return null;
  const references = await Promise.all(
    metadata.references.map(async (url, index) => {
      const dataUrl = url.startsWith("image:")
        ? await resolveImageUrl(url, "")
        : url;
      return dataUrl
        ? {
            id: `${index}`,
            name: `reference-${index}.png`,
            type: "image/png",
            dataUrl,
            storageKey: url.startsWith("image:") ? url : undefined,
          }
        : null;
    }),
  );
  return references.every(Boolean) ? (references as ReferenceImage[]) : null;
}

export function getGenerationCount(count: string) {
  return Math.max(1, Math.min(15, Math.floor(Math.abs(Number(count)) || 1)));
}

export function getInputSummary(inputs: NodeGenerationInput[]) {
  const resources = [
    ...new Map(
      inputs
        .flatMap((input) => (input.type === "group" ? input.children : [input]))
        .map((input) => [input.nodeId, input]),
    ).values(),
  ];
  return {
    textCount: resources.filter((input) => input.type === "text").length,
    imageCount: resources.filter((input) => input.type === "image").length,
    videoCount: resources.filter((input) => input.type === "video").length,
    audioCount: resources.filter((input) => input.type === "audio").length,
  };
}

export function buildGenerationConfig(
  config: AiConfig,
  node: CanvasNodeData | undefined,
  mode: CanvasNodeGenerationMode,
): AiConfig {
  return {
    ...config,
    model: resolveModelForCapability(config, node?.metadata?.model, mode),
    reasoningEffort:
      node?.metadata?.reasoningEffort ||
      config.reasoningEffort ||
      defaultConfig.reasoningEffort,
    quality: node?.metadata?.quality || config.quality || defaultConfig.quality,
    size: node?.metadata?.size || config.size || defaultConfig.size,
    background:
      node?.metadata?.background ??
      config.background ??
      defaultConfig.background,
    videoSeconds:
      node?.metadata?.seconds ||
      config.videoSeconds ||
      defaultConfig.videoSeconds,
    vquality:
      node?.metadata?.vquality || config.vquality || defaultConfig.vquality,
    videoGenerateAudio:
      node?.metadata?.generateAudio ||
      config.videoGenerateAudio ||
      defaultConfig.videoGenerateAudio,
    videoWatermark:
      node?.metadata?.watermark ||
      config.videoWatermark ||
      defaultConfig.videoWatermark,
    videoMode:
      node?.metadata?.videoMode || config.videoMode || defaultConfig.videoMode,
    audioVoice:
      node?.metadata?.audioVoice ||
      config.audioVoice ||
      defaultConfig.audioVoice,
    audioFormat:
      node?.metadata?.audioFormat ||
      config.audioFormat ||
      defaultConfig.audioFormat,
    audioSpeed:
      node?.metadata?.audioSpeed ||
      config.audioSpeed ||
      defaultConfig.audioSpeed,
    audioInstructions:
      node?.metadata?.audioInstructions ||
      config.audioInstructions ||
      defaultConfig.audioInstructions,
    count: String(
      node?.metadata?.count ||
        (mode === "image"
          ? config.canvasImageCount || config.count
          : config.count) ||
        defaultConfig.count,
    ),
  };
}

export function hasResumableVideoTask(node: CanvasNodeData) {
  return (
    node.type === CanvasNodeType.Video &&
    Boolean(node.metadata?.videoTaskId) &&
    !node.metadata?.content
  );
}

export function isGenerationCanceled(error: unknown) {
  return (
    error instanceof Error &&
    (error.message === "请求已取消" || error.name === "AbortError")
  );
}

export function findRetrySourceNode(
  nodeId: string,
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
) {
  const queue = connections
    .filter((connection) => connection.toNodeId === nodeId)
    .map((connection) => connection.fromNodeId);
  const visited = new Set<string>();
  while (queue.length) {
    const id = queue.shift()!;
    if (visited.has(id)) continue;
    visited.add(id);
    const node = nodes.find((item) => item.id === id);
    if (node?.type === CanvasNodeType.Config) return node;
    connections
      .filter((connection) => connection.toNodeId === id)
      .forEach((connection) => queue.push(connection.fromNodeId));
  }
  return null;
}

export function sourceNodeReferenceImages(node: CanvasNodeData | null) {
  if (!node || node.type !== CanvasNodeType.Image || !node.metadata?.content)
    return [];
  return [
    {
      id: node.id,
      name: `${node.title || node.id}.png`,
      type: node.metadata.mimeType || "image/png",
      dataUrl: node.metadata.content,
      storageKey: node.metadata.storageKey,
    },
  ];
}

export function isAudioFile(file: File) {
  return file.type.startsWith("audio/") || /\.(mp3|wav)$/i.test(file.name);
}

export function buildAngleLabel(params: CanvasImageAngleParams) {
  const horizontal =
    params.horizontalAngle === 0
      ? "正面视角"
      : params.horizontalAngle > 0
        ? `向右旋转 ${params.horizontalAngle} 度`
        : `向左旋转 ${Math.abs(params.horizontalAngle)} 度`;
  const pitch =
    params.pitchAngle === 0
      ? "水平视角"
      : params.pitchAngle > 0
        ? `俯视 ${params.pitchAngle} 度`
        : `仰视 ${Math.abs(params.pitchAngle)} 度`;
  return `AI 多角度：${horizontal}，${pitch}，镜头距离 ${params.cameraDistance.toFixed(1)}，${params.wideAngle ? "广角" : "标准"}镜头`;
}

export function buildAnglePrompt(params: CanvasImageAngleParams) {
  return `基于参考图重新生成同一主体的新视角，保持主体、颜色、材质和画面风格一致，不要只做透视变形。${buildAngleLabel(params)}。`;
}
