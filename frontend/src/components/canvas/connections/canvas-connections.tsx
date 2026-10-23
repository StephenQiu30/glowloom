import type { MouseEvent as ReactMouseEvent } from "react";

import { canvasThemes } from "@/components/canvas/runtime/lib/theme";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";
import { canvasConnectionGeometry } from "../lib/canvas-connection-geometry";
import type {
  CanvasConnection,
  CanvasNodeData,
  ConnectionHandle,
  Position,
} from "@/components/canvas/types/canvas";

export function ConnectionPath({
  connection,
  from,
  to,
  active,
  generating = false,
  selected = false,
  onSelect,
  onContextMenu,
}: {
  connection: CanvasConnection;
  from: CanvasNodeData;
  to: CanvasNodeData;
  active: boolean;
  generating?: boolean;
  selected?: boolean;
  onSelect: () => void;
  onContextMenu?: (event: ReactMouseEvent<SVGPathElement>) => void;
}) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  const highlighted = active || selected || generating;
  const startX = from.position.x + from.width;
  const startY = from.position.y + from.height / 2;
  const endX = to.position.x;
  const endY = to.position.y + to.height / 2;
  const { path: pathD } = canvasConnectionGeometry(
    { x: startX, y: startY },
    { x: endX, y: endY },
  );

  return (
    <g
      className="canvas-connection"
      data-canvas-edge-id={connection.id}
      data-selected={selected || undefined}
    >
      <path
        data-connection-id={connection.id}
        d={pathD}
        stroke="transparent"
        strokeWidth="16"
        vectorEffect="non-scaling-stroke"
        fill="none"
        style={{ cursor: "pointer", pointerEvents: "stroke" }}
        onClick={(event) => {
          event.stopPropagation();
          onSelect();
        }}
        onContextMenu={(event) => {
          event.preventDefault();
          event.stopPropagation();
          onContextMenu?.(event);
        }}
      />
      <path
        className="canvas-connection-line"
        d={pathD}
        stroke={highlighted ? theme.node.activeStroke : theme.node.muted}
        strokeWidth={selected ? 2 : 1.5}
        vectorEffect="non-scaling-stroke"
        strokeOpacity={selected ? 1 : highlighted ? 0.72 : 0.55}
        strokeLinecap="round"
        fill="none"
        style={{
          pointerEvents: "none",
        }}
      />
      {generating && !selected && (
        <path
          className="canvas-connection-flow"
          d={pathD}
          pathLength={1000}
          stroke={theme.node.activeStroke}
          strokeWidth={2}
          strokeDasharray="80 1120"
          strokeDashoffset={80}
          strokeLinecap="round"
          vectorEffect="non-scaling-stroke"
          fill="none"
          style={{
            pointerEvents: "none",
          }}
        />
      )}
    </g>
  );
}

export function ActiveConnectionPath({
  node,
  handle,
  mouseWorld,
  target,
}: {
  node?: CanvasNodeData;
  handle: ConnectionHandle;
  mouseWorld: Position;
  target?: CanvasNodeData;
}) {
  const theme = canvasThemes[useThemeStore((state) => state.theme)];
  if (!node) return null;

  const startX =
    handle.handleType === "source"
      ? node.position.x + node.width
      : mouseWorld.x;
  const startY =
    handle.handleType === "source"
      ? node.position.y + node.height / 2
      : mouseWorld.y;
  const endX = handle.handleType === "source" ? mouseWorld.x : node.position.x;
  const endY =
    handle.handleType === "source"
      ? mouseWorld.y
      : node.position.y + node.height / 2;
  const snappedStartX =
    handle.handleType === "target" && target
      ? target.position.x + target.width
      : startX;
  const snappedStartY =
    handle.handleType === "target" && target
      ? target.position.y + target.height / 2
      : startY;
  const snappedEndX =
    handle.handleType === "source" && target ? target.position.x : endX;
  const snappedEndY =
    handle.handleType === "source" && target
      ? target.position.y + target.height / 2
      : endY;
  const { path: pathD } = canvasConnectionGeometry(
    { x: snappedStartX, y: snappedStartY },
    { x: snappedEndX, y: snappedEndY },
  );

  return (
    <path
      d={pathD}
      stroke={theme.node.activeStroke}
      strokeWidth="2"
      vectorEffect="non-scaling-stroke"
      fill="none"
      strokeDasharray="5,5"
    />
  );
}
