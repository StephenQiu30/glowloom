import { mediaAssetsToCanvasNodes } from "../media-import";
import {
  CanvasNodeType,
  type CanvasCommand,
  type CanvasDocument,
} from "../model";
import type { MediaAsset } from "../queries";
import {
  createDirectorCaptureContext,
  type DirectorCaptureContext,
} from "./outputs";

/** Converts only the actual reviewed canonical video, with the saved scene identity. */
export function directorRecordingResultCommands(
  document: CanvasDocument,
  sourceNodeId: string,
  context: DirectorCaptureContext,
  asset: MediaAsset,
  savedRevision: number,
): CanvasCommand[] {
  const source = document.nodes.find((node) => node.id === sourceNodeId);
  if (
    !Number.isInteger(savedRevision) ||
    savedRevision <= 0 ||
    document.revision < savedRevision ||
    source?.type !== CanvasNodeType.Director ||
    !source.director ||
    asset.project_id !== document.projectId ||
    asset.kind !== "video" ||
    asset.mime_type !== "video/mp4"
  )
    throw new Error("当前导演场景不能接收此正式白膜素材。");
  const current = createDirectorCaptureContext(source.director);
  if (
    current.sceneId !== context.sceneId ||
    current.shotId !== context.shotId ||
    current.renderKey !== context.renderKey
  )
    throw new Error("场景或镜头已改变；正式素材已保留，请从媒体库重新关联。");
  const existing = document.nodes.filter(
    (node) => node.type === CanvasNodeType.Video && node.assetId === asset.id,
  );
  if (
    existing.some((node) =>
      document.connections.some(
        (edge) => edge.fromNodeId === source.id && edge.toNodeId === node.id,
      ),
    )
  )
    return [];
  if (
    (!existing.length && document.nodes.length >= 2000) ||
    document.connections.length >= 4000
  )
    throw new Error("画布节点或关系已达到容量上限。");
  const node =
    existing[0] ??
    mediaAssetsToCanvasNodes([asset], {
      x: source.position.x + source.width + 48,
      y: source.position.y,
    })[0];
  return [
    ...(!existing.length ? [{ type: "AddNodes" as const, nodes: [node] }] : []),
    {
      type: "Connect",
      edges: [
        { id: crypto.randomUUID(), fromNodeId: source.id, toNodeId: node.id },
      ],
    },
  ];
}
