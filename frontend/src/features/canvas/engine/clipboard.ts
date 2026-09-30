// Adapted from BeefTV use-canvas-node-operations: ID remap, world offset, internal wires.
import type { CanvasConnection, CanvasNodeData } from "../model";
import { getFrameChildIds } from "./frame";
export type CanvasClipboard = {
  nodes: CanvasNodeData[];
  connections: CanvasConnection[];
};
export function copySelection(
  nodes: CanvasNodeData[],
  connections: CanvasConnection[],
  selected: Set<string>,
): CanvasClipboard {
  const ids = new Set(selected);
  selected.forEach((id) =>
    getFrameChildIds(id, nodes).forEach((child) => ids.add(child)),
  );
  return {
    nodes: structuredClone(nodes.filter((node) => ids.has(node.id))),
    connections: structuredClone(
      connections.filter(
        (edge) => ids.has(edge.fromNodeId) && ids.has(edge.toNodeId),
      ),
    ),
  };
}
export function pasteSelection(
  clipboard: CanvasClipboard,
  offset = 40,
  newId: () => string = () => crypto.randomUUID(),
): CanvasClipboard {
  const ids = new Map(clipboard.nodes.map((node) => [node.id, newId()]));
  return {
    nodes: clipboard.nodes.map((node) => ({
      ...node,
      id: ids.get(node.id)!,
      title: `${node.title.slice(0, 120)} 副本`,
      position: { x: node.position.x + offset, y: node.position.y + offset },
      parentId: node.parentId ? ids.get(node.parentId) : undefined,
    })),
    connections: clipboard.connections.map((edge) => ({
      id: newId(),
      fromNodeId: ids.get(edge.fromNodeId)!,
      toNodeId: ids.get(edge.toNodeId)!,
    })),
  };
}
