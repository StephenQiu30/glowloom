import { mediaAssetsToCanvasNodes } from "../media-import";
import {
  CanvasNodeType,
  type CanvasCommand,
  type CanvasDocument,
} from "../model";
import type { MediaAsset } from "../queries";
import { encodeDirector } from "./model";
import {
  appendDirectorScreenshot,
  type DirectorCaptureContext,
} from "./outputs";

export function directorCaptureResultCommands(
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
    !source?.director ||
    asset.project_id !== document.projectId ||
    asset.kind !== "image"
  )
    throw new Error("当前导演场景不能接收此截图素材。");
  const scene = appendDirectorScreenshot(source.director, context, {
    id: crypto.randomUUID(),
    assetId: asset.id,
    name: asset.file_name.slice(0, 128),
    createdAt: new Date().toISOString(),
  });
  const commands: CanvasCommand[] = [];
  if (scene !== source.director)
    commands.push({
      type: "UpdateNodeConfig",
      id: source.id,
      config: { director: encodeDirector(scene) },
    });
  let target = document.nodes.find(
    (node) => node.type === CanvasNodeType.Image && node.assetId === asset.id,
  );
  if (!target) {
    if (document.nodes.length >= 2000)
      throw new Error("画布已达到节点容量上限。");
    [target] = mediaAssetsToCanvasNodes([asset], {
      x: source.position.x + source.width + 48,
      y: source.position.y,
    });
    commands.push({ type: "AddNodes", nodes: [target] });
  }
  if (
    !document.connections.some(
      (edge) => edge.fromNodeId === source.id && edge.toNodeId === target!.id,
    )
  ) {
    if (document.connections.length >= 4000)
      throw new Error("画布已达到连线容量上限。");
    commands.push({
      type: "Connect",
      edges: [
        { id: crypto.randomUUID(), fromNodeId: source.id, toNodeId: target.id },
      ],
    });
  }
  return commands;
}
