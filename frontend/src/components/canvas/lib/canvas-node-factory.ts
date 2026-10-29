import { migrateDirectorDocument } from "../director/director-workspaces";
import { emptyDirectorConfig } from "../director/director-model";
import {
  getNodeSpec,
  NODE_DEFAULT_SIZE,
} from "@/components/canvas/constants/canvas";
import { nodeSizeFromRatio } from "@/components/canvas/lib/canvas-node-size";
import type { AiConfig } from "@/components/canvas/runtime/store/use-config-store";
import type { UploadedImage } from "@/components/canvas/runtime/lib/media/image-storage";
import type { UploadedFile } from "@/components/canvas/runtime/lib/media/file-storage";
import type { ReferenceImage } from "@/components/canvas/runtime/types/image";
import {
  CanvasNodeType,
  type CanvasImageGenerationType,
  type CanvasNodeData,
  type CanvasNodeMetadata,
  type CanvasNodeTypeId,
  type ConnectionHandle,
  type Position,
} from "@/components/canvas/types/canvas";

export function connectedNodeCenter(
  type: CanvasNodeTypeId,
  pending: {
    connection: ConnectionHandle;
    position: Position;
    placement?: "adjacent";
  },
  source?: CanvasNodeData,
): Position {
  if (pending.placement !== "adjacent" || !source) return pending.position;
  const spec = getNodeSpec(type);
  return {
    x:
      pending.connection.handleType === "source"
        ? source.position.x + source.width + 60 + spec.width / 2
        : source.position.x - 60 - spec.width / 2,
    y: source.position.y + source.height / 2,
  };
}

export function createCanvasNode(
  type: CanvasNodeTypeId,
  position: Position,
  metadata?: CanvasNodeMetadata,
  existingNodes: CanvasNodeData[] = [],
): CanvasNodeData {
  const spec = getNodeSpec(type);
  const id = crypto.randomUUID();

  return {
    id,
    type,
    ...(type === "director"
      ? {
          serviceData: {
            id,
            node_type: "director",
            node_action: "tool",
            config: {
              director: migrateDirectorDocument(emptyDirectorConfig()),
            },
          },
        }
      : {}),
    title: nextCanvasNodeTitle(spec.title, type, existingNodes),
    position: {
      x: position.x - spec.width / 2,
      y: position.y - spec.height / 2,
    },
    width: spec.width,
    height: spec.height,
    metadata: { ...spec.metadata, ...metadata },
  };
}

export function imageMetadata(image: UploadedImage): CanvasNodeMetadata {
  return {
    content: image.url,
    storageKey: image.storageKey,
    status: "success",
    naturalWidth: image.width,
    naturalHeight: image.height,
    bytes: image.bytes,
    mimeType: image.mimeType,
  };
}

export function videoMetadata(video: UploadedFile): CanvasNodeMetadata {
  return {
    content: video.url,
    storageKey: video.storageKey,
    status: "success",
    naturalWidth: video.width,
    naturalHeight: video.height,
    bytes: video.bytes,
    mimeType: video.mimeType || "video/mp4",
    durationMs: video.durationMs,
  };
}

export function audioMetadata(audio: UploadedFile): CanvasNodeMetadata {
  return {
    content: audio.url,
    storageKey: audio.storageKey,
    status: "success",
    bytes: audio.bytes,
    mimeType: audio.mimeType || "audio/mpeg",
    durationMs: audio.durationMs,
  };
}

export function referenceUrl(image: ReferenceImage) {
  return (
    image.storageKey ||
    image.url ||
    (!image.dataUrl.startsWith("data:") ? image.dataUrl : undefined)
  );
}

export function buildImageGenerationMetadata(
  type: CanvasImageGenerationType,
  config: AiConfig,
  count: number,
  references: ReferenceImage[],
): CanvasNodeMetadata {
  return {
    generationType: type,
    model: config.model,
    size: config.size,
    quality: config.quality,
    ...(config.background ? { background: config.background } : {}),
    count,
    references: references
      .map(referenceUrl)
      .filter((url): url is string => Boolean(url)),
  };
}

export function buildAudioGenerationMetadata(
  config: AiConfig,
): CanvasNodeMetadata {
  return {
    model: config.model,
    audioVoice: config.audioVoice,
    audioFormat: config.audioFormat,
    audioSpeed: config.audioSpeed,
    audioInstructions: config.audioInstructions,
  };
}

export function applyNodeConfigPatch(
  node: CanvasNodeData,
  patch: Partial<CanvasNodeData["metadata"]>,
) {
  const safePatch = patch || {};
  const next = { ...node, metadata: { ...node.metadata, ...safePatch } };
  const spec =
    node.type === CanvasNodeType.Video
      ? NODE_DEFAULT_SIZE[CanvasNodeType.Video]
      : NODE_DEFAULT_SIZE[CanvasNodeType.Image];
  const size =
    typeof safePatch.size === "string" && !node.metadata?.content
      ? nodeSizeFromRatio(safePatch.size, spec.width, spec.height)
      : null;
  return size &&
    (node.type === CanvasNodeType.Image || node.type === CanvasNodeType.Video)
    ? {
        ...next,
        ...size,
        position: {
          x: node.position.x + node.width / 2 - size.width / 2,
          y: node.position.y + node.height / 2 - size.height / 2,
        },
      }
    : next;
}

/** Names are display labels; UUIDs remain the only node identity. */
export function nextCanvasNodeTitle(
  title: string,
  type: CanvasNodeTypeId,
  nodes: CanvasNodeData[],
) {
  let greatest = 0;
  const prefix = `${title} `;
  for (const node of nodes) {
    if (node.type !== type || !node.title?.startsWith(prefix)) continue;
    const suffix = node.title.slice(prefix.length);
    if (/^[1-9]\d*$/.test(suffix)) {
      const value = Number(suffix);
      if (Number.isSafeInteger(value)) greatest = Math.max(greatest, value);
    }
  }
  return `${title} ${greatest + 1}`;
}
