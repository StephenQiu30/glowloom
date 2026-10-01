import { mediaAssetsToCanvasNodes } from "./media-import";
import {
  CanvasNodeType,
  type CanvasCommand,
  type CanvasDocument,
} from "./model";
import type { MediaAsset } from "./queries";

export function exportResultCommands(
  document: CanvasDocument,
  sourceId: string,
  asset: MediaAsset,
): CanvasCommand[] {
  const source = document.nodes.find((node) => node.id === sourceId);
  if (!source || source.type !== CanvasNodeType.Timeline)
    throw new Error("时间线已不存在。");
  if (
    asset.project_id !== document.projectId ||
    !["video", "audio"].includes(asset.kind)
  )
    throw new Error("输出必须是当前项目的正式视频或音频素材。");
  const existing = document.nodes.filter(
    (node) => node.type === asset.kind && node.assetId === asset.id,
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
        {
          id: crypto.randomUUID(),
          fromNodeId: source.id,
          toNodeId: node.id,
        },
      ],
    },
  ];
}
