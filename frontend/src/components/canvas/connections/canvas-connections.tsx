import { useState, useRef, type MouseEvent as ReactMouseEvent } from "react";

import { useConnectionFlow } from "./connection-flow-layer";
import { CONNECTION_FLOW_BASE } from "./connection-flow-model";

import { canvasThemes } from "@/components/canvas/runtime/lib/theme";
import { useThemeStore } from "@/components/canvas/runtime/store/use-theme-store";
import { canvasConnectionGeometry } from "../lib/canvas-connection-geometry";
import type {
  CanvasConnection,
  CanvasNodeData,
  ConnectionHandle,
  Position,
} from "@/components/canvas/types/canvas";

function ConnectionStroke({
  path,
  highlighted,
  muted = "var(--muted-foreground)",
  fanout = false,
}: {
  path: string;
  highlighted: boolean;
  muted?: string;
  fanout?: boolean;
}) {
  const pathRef = useRef<SVGPathElement>(null);
  useConnectionFlow(path, highlighted, pathRef, fanout);
  return (
    <g aria-hidden="true" style={{ pointerEvents: "none" }}>
      <path
        ref={pathRef}
        className="canvas-connection-line"
        d={path}
        stroke={highlighted ? CONNECTION_FLOW_BASE : muted}
        strokeWidth={1}
        strokeOpacity={highlighted ? 1 : 0.45}
        vectorEffect="non-scaling-stroke"
        strokeLinecap="round"
        fill="none"
      />
      {highlighted && <g className="canvas-connection-meteor" />}
    </g>
  );
}

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
  const [hovered, setHovered] = useState(false);
  const highlighted = active || selected || generating || hovered;
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
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
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
      <ConnectionStroke
        path={pathD}
        highlighted={highlighted}
        muted={theme.node.muted}
      />
    </g>
  );
}

export function ActiveConnectionPath({
  node,
  handle,
  mouseWorld,
  target,
  fanout = false,
  anchorOffset = 0,
}: {
  node?: CanvasNodeData;
  handle: ConnectionHandle;
  mouseWorld: Position;
  target?: CanvasNodeData;
  fanout?: boolean;
  anchorOffset?: number;
}) {
  if (!node) return null;

  const startX =
    handle.handleType === "source"
      ? node.position.x + node.width + anchorOffset
      : mouseWorld.x;
  const startY =
    handle.handleType === "source"
      ? node.position.y + node.height / 2
      : mouseWorld.y;
  const endX =
    handle.handleType === "source"
      ? mouseWorld.x
      : node.position.x - anchorOffset;
  const endY =
    handle.handleType === "source"
      ? mouseWorld.y
      : node.position.y + node.height / 2;
  const snappedStartX =
    handle.handleType === "target" && target
      ? target.position.x + target.width + anchorOffset
      : startX;
  const snappedStartY =
    handle.handleType === "target" && target
      ? target.position.y + target.height / 2
      : startY;
  const snappedEndX =
    handle.handleType === "source" && target
      ? target.position.x - anchorOffset
      : endX;
  const snappedEndY =
    handle.handleType === "source" && target
      ? target.position.y + target.height / 2
      : endY;
  const { path: pathD } = canvasConnectionGeometry(
    { x: snappedStartX, y: snappedStartY },
    { x: snappedEndX, y: snappedEndY },
  );

  return (
    <g data-canvas-connection-preview>
      <ConnectionStroke path={pathD} highlighted fanout={fanout} />
    </g>
  );
}
