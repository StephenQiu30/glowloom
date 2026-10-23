import type { CanvasConnection, CanvasNodeData } from "../types/canvas";
import { isFixedCanvasMedia } from "./canvas-media-source";

/** A new paid attempt owns a new node; retries of unfinished work keep its identity. */
export function repeatCanvasMediaGeneration(
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
  nodeId: string,
  run: CanvasContract.CanvasGenerationVO | undefined,
  createId: () => string = () => crypto.randomUUID(),
) {
  const source = nodes.find((node) => node.id === nodeId);
  if (
    !source ||
    !["image", "video"].includes(source.type) ||
    isFixedCanvasMedia(source) ||
    run?.state !== "SUCCEEDED" ||
    !run.outputs?.length
  )
    return { nodes, connections, nodeId };
  if (
    nodes.length >= 500 ||
    connections.length +
      connections.filter((e) => e.toNodeId === nodeId).length >
      1000
  )
    throw new Error("画布节点或连线数量已达上限，请整理后生成");
  const id = createId();
  if (nodes.some((node) => node.id === id))
    throw new Error("新节点标识重复，请重试");
  const position = {
    x: source.position.x + source.width + 80,
    y: source.position.y,
  };
  while (
    nodes.some(
      (node) =>
        node.type !== "group" &&
        position.x < node.position.x + node.width + 24 &&
        position.x + source.width + 24 > node.position.x &&
        position.y < node.position.y + node.height + 24 &&
        position.y + source.height + 24 > node.position.y,
    )
  )
    position.y += source.height + 48;
  const copy = structuredClone(source);
  const clone: CanvasNodeData = {
    ...copy,
    id,
    position,
    referenceProjection: undefined,
    documentData: copy.documentData
      ? { ...copy.documentData, id, position }
      : undefined,
    metadata: {
      ...copy.metadata,
      content: undefined,
      status: "idle",
      generationOutput: undefined,
      errorDetails: undefined,
      images: undefined,
      primaryImageId: undefined,
      videoTaskId: undefined,
    },
  };
  const incoming = connections
    .filter((edge) => edge.toNodeId === nodeId)
    .map((edge, index) => {
      if (!edge.documentData) throw new Error("请先确认参考连线已保存");
      const edgeId = `edge-${createId()}-${index}`;
      return {
        ...structuredClone(edge),
        id: edgeId,
        toNodeId: id,
        documentData: {
          ...structuredClone(edge.documentData),
          id: edgeId,
          toNodeId: id,
        },
      };
    });
  return {
    nodeId: id,
    nodes: [...nodes, clone],
    connections: [...connections, ...incoming],
  };
}
