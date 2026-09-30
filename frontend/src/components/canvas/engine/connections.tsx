// SVG path and selection layer adapted from BeefTV canvas-connections.tsx (MIT).
import { memo } from "react";
import { canvasNodeRenderHeight, type CanvasDocument } from "../model";
import { collapsedConnectionEndpoint } from "./frame";
import { subscribeCanvasNodeDragPreview } from "./viewport-dom";
import type {
  CanvasConnection,
  CanvasNodeData,
  ConnectionHandle,
  Position,
} from "../model";
export function canvasConnectionPath(
  edge: CanvasConnection,
  from: CanvasNodeData,
  to: CanvasNodeData,
) {
  const startX = from.position.x + from.width,
    startY = from.position.y + canvasNodeRenderHeight(from) / 2,
    endX = to.position.x,
    endY = to.position.y + canvasNodeRenderHeight(to) / 2;
  const curve = Math.max(Math.abs(endX - startX) * 0.5, 50);
  return `M ${startX} ${startY} C ${startX + curve} ${startY}, ${endX - curve} ${endY}, ${endX} ${endY}`;
}
export function activeConnectionPath(
  node: CanvasNodeData,
  handle: ConnectionHandle,
  mouse: Position,
) {
  const startX =
      handle.handleType === "source" ? node.position.x + node.width : mouse.x,
    startY =
      handle.handleType === "source"
        ? node.position.y + canvasNodeRenderHeight(node) / 2
        : mouse.y;
  const endX = handle.handleType === "source" ? mouse.x : node.position.x,
    endY =
      handle.handleType === "source"
        ? mouse.y
        : node.position.y + canvasNodeRenderHeight(node) / 2;
  const curve = Math.max(50, Math.abs(endX - startX) * 0.5);
  return `M ${startX} ${startY} C ${startX + curve} ${startY}, ${endX - curve} ${endY}, ${endX} ${endY}`;
}
/** 复用源节点预览事件；拖动只改 SVG，松手才发送正式文档命令。 */
export function bindCanvasConnectionPreview(
  container: HTMLDivElement,
  document: CanvasDocument,
) {
  const byId = new Map(document.nodes.map((node) => [node.id, node]));
  const paths = new Map<string, SVGPathElement[]>();
  container
    .querySelectorAll<SVGPathElement>("[data-connection-path-id]")
    .forEach((path) => {
      const id = path.dataset.connectionPathId;
      if (id) paths.set(id, [...(paths.get(id) ?? []), path]);
    });
  return subscribeCanvasNodeDragPreview(container, (preview) => {
    for (const edge of document.connections) {
      const targets = paths.get(edge.id);
      if (!targets) continue;
      const from = collapsedConnectionEndpoint(
          byId.get(edge.fromNodeId),
          document.nodes,
          byId,
        ),
        to = collapsedConnectionEndpoint(
          byId.get(edge.toNodeId),
          document.nodes,
          byId,
        );
      if (!from || !to || from.id === to.id) continue;
      const offset = (node: CanvasNodeData) =>
        preview?.nodeIds.has(node.id)
          ? {
              ...node,
              position: {
                x: node.position.x + preview.x,
                y: node.position.y + preview.y,
              },
            }
          : node;
      const d = canvasConnectionPath(edge, offset(from), offset(to));
      targets.forEach((path) => path.setAttribute("d", d));
    }
  });
}
export const ConnectionPath = memo(function ConnectionPath({
  edge,
  from,
  to,
  selected,
  onSelect,
}: {
  edge: CanvasConnection;
  from: CanvasNodeData;
  to: CanvasNodeData;
  selected: boolean;
  onSelect: () => void;
}) {
  const d = canvasConnectionPath(edge, from, to);
  return (
    <g>
      <path
        data-connection-id={edge.id}
        data-connection-path-id={edge.id}
        d={d}
        fill="none"
        stroke="transparent"
        strokeWidth={18}
        vectorEffect="non-scaling-stroke"
        style={{ pointerEvents: "stroke", cursor: "pointer" }}
        onClick={(event) => {
          event.stopPropagation();
          onSelect();
        }}
      />
      <path
        data-connection-path-id={edge.id}
        d={d}
        fill="none"
        stroke={selected ? "var(--foreground)" : "var(--muted-foreground)"}
        strokeOpacity={selected ? 0.9 : 0.5}
        strokeWidth={selected ? 2.5 : 1.5}
        vectorEffect="non-scaling-stroke"
        style={{ pointerEvents: "none" }}
      />
    </g>
  );
});
