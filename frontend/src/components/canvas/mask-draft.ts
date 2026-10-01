import {
  CanvasNodeType,
  type CanvasNodeData,
  type CanvasCommand,
} from "./model";
import { mediaAssetsToCanvasNodes } from "./media-import";
import { createNode } from "./document";
import type { MediaAsset } from "./queries";

/** 蒙版上传后仅创建草稿，实际用途映射及费用由目录与正式表单确认。 */
export function maskDraftCommands(
  source: CanvasNodeData,
  mask: MediaAsset,
  projectId: string,
  prompt: string,
  nodeCount: number,
): { commands: CanvasCommand[]; generationNodeId: string } {
  if (
    source.type !== CanvasNodeType.Image ||
    !source.assetId ||
    mask.kind !== "image" ||
    mask.project_id !== projectId ||
    !prompt.trim() ||
    prompt.length > 10000 ||
    !Number.isInteger(nodeCount) ||
    nodeCount < 0 ||
    nodeCount + 2 > 2000
  )
    throw new Error("原图、蒙版、修改要求或画布容量无效。");
  const maskNode = mediaAssetsToCanvasNodes([mask], {
    x: source.position.x + source.width + 48,
    y: source.position.y,
  })[0];
  const generation = createNode(CanvasNodeType.Generation, {
    x: maskNode.position.x + maskNode.width + 48,
    y: source.position.y,
  });
  generation.title = "局部重绘草稿";
  generation.generation = {
    ...generation.generation!,
    capability: "image.edit",
    prompt: prompt.trim(),
    inputs: [
      { role: "image", mediaAssetId: source.assetId },
      { role: "mask", mediaAssetId: mask.id },
    ],
  };
  return {
    generationNodeId: generation.id,
    commands: [
      { type: "AddNodes", nodes: [maskNode, generation] },
      {
        type: "Connect",
        edges: [source.id, maskNode.id].map((fromNodeId) => ({
          id: crypto.randomUUID(),
          fromNodeId,
          toNodeId: generation.id,
        })),
      },
    ],
  };
}
