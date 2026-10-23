// Attribution and source revision are recorded in /licenses/canvas-sources.md.
// Upstream notices: /licenses/canvas-sources.md.
import {
  CANVAS_ALIGNMENT_DISTANCE,
  CANVAS_DRAG_GRID_SIZE,
  CANVAS_DRAG_THRESHOLD,
} from "./drag-constants";
import {
  CanvasNodeType,
  type CanvasConnection,
  type CanvasNodeData,
  type Position,
} from "./types";
import { canvasConnectionGeometry } from "./connection-geometry";
import { findGroupDropTarget, nodeBounds } from "./geometry";

type DragOptions = {
  container: HTMLElement;
  nodes: CanvasNodeData[];
  connections: CanvasConnection[];
  movedIds: Set<string>;
  start: Position;
  scale: number;
  snap: boolean;
  onDropTarget?: (id: string | null) => void;
};
type Guide = { axis: "x" | "y"; value: number };
type PreviewEdge = {
  paths: SVGPathElement[];
  from: CanvasNodeData;
  to: CanvasNodeData;
};

function closestAlignment(points: number[], candidates: number[]) {
  let distance = CANVAS_ALIGNMENT_DISTANCE;
  let match: { offset: number; value: number } | undefined;
  for (const point of points) {
    let low = 0;
    let high = candidates.length;
    while (low < high) {
      const mid = (low + high) >>> 1;
      if (candidates[mid] < point) low = mid + 1;
      else high = mid;
    }
    for (const index of [low - 1, low]) {
      const value = candidates[index];
      if (value === undefined) continue;
      const nextDistance = Math.abs(value - point);
      if (nextDistance < distance) {
        distance = nextDistance;
        match = { offset: value - point, value };
      }
    }
  }
  return match;
}

/** Merge only positions into the latest graph; never resurrect deleted nodes. */
export function applyCanvasDragPositions(
  nodes: CanvasNodeData[],
  positions: ReadonlyMap<string, Position>,
) {
  return nodes.map((node) => {
    const position = positions.get(node.id);
    return position &&
      (node.position.x !== position.x || node.position.y !== position.y)
      ? { ...node, position }
      : node;
  });
}

/** One gesture owns transient DOM writes. Business state changes only on finish. */
export class CanvasNodeDragSession {
  readonly initialPositions: Map<string, Position>;
  readonly movedIds: Set<string>;
  hasMoved = false;
  private active = true;
  private pointer: Position;
  private scale: number;
  private snap: boolean;
  private frame: number | null = null;
  private moving: CanvasNodeData[] = [];
  private groups: CanvasNodeData[] = [];
  private elements = new Map<string, HTMLElement>();
  private edges: PreviewEdge[] = [];
  private xCandidates: number[] = [];
  private yCandidates: number[] = [];
  private stationaryBounds = nodeBounds([]);
  private guideLayer: SVGElement | null = null;
  private selection: SVGElement | null = null;
  private dropTarget: string | null = null;

  constructor(private options: DragOptions) {
    this.pointer = options.start;
    this.scale = options.scale;
    this.snap = options.snap;
    this.movedIds = new Set(options.movedIds);
    this.initialPositions = new Map(
      options.nodes
        .filter((node) => this.movedIds.has(node.id))
        .map((node) => [node.id, { ...node.position }]),
    );
    this.refresh(options.nodes, options.connections);
  }

  refresh(nodes: CanvasNodeData[], connections: CanvasConnection[]) {
    if (!this.active) return;
    this.captureGraph(nodes, connections);
    if (this.hasMoved) this.paint();
  }

  private captureGraph(
    nodes: CanvasNodeData[],
    connections: CanvasConnection[],
  ) {
    this.moving = nodes.filter((node) => this.movedIds.has(node.id));
    this.groups = nodes.filter(
      (node) =>
        node.type === CanvasNodeType.Group && !this.movedIds.has(node.id),
    );
    const stationary = nodes.filter((node) => !this.movedIds.has(node.id));
    this.stationaryBounds = nodeBounds(stationary);
    this.xCandidates = [
      ...new Set(
        stationary.flatMap((node) => [
          node.position.x,
          node.position.x + node.width / 2,
          node.position.x + node.width,
        ]),
      ),
    ].sort((a, b) => a - b);
    this.yCandidates = [
      ...new Set(
        stationary.flatMap((node) => [
          node.position.y,
          node.position.y + node.height / 2,
          node.position.y + node.height,
        ]),
      ),
    ].sort((a, b) => a - b);
    const { container } = this.options;
    this.elements = new Map(
      [...container.querySelectorAll<HTMLElement>("[data-node-id]")]
        .filter((el) => this.movedIds.has(el.dataset.nodeId ?? ""))
        .map((el) => [el.dataset.nodeId!, el]),
    );
    const nodeById = new Map(nodes.map((node) => [node.id, node]));
    const edgeElements = new Map(
      [...container.querySelectorAll<SVGGElement>("[data-canvas-edge-id]")].map(
        (el) => [el.dataset.canvasEdgeId, el],
      ),
    );
    this.edges = connections.flatMap((connection) => {
      if (
        !this.movedIds.has(connection.fromNodeId) &&
        !this.movedIds.has(connection.toNodeId)
      )
        return [];
      const el = edgeElements.get(connection.id);
      const from = nodeById.get(connection.fromNodeId);
      const to = nodeById.get(connection.toNodeId);
      return el && from && to
        ? [
            {
              paths: [...el.querySelectorAll<SVGPathElement>("path")],
              from,
              to,
            },
          ]
        : [];
    });
    this.guideLayer = container.querySelector<SVGElement>(
      "[data-canvas-drag-guides]",
    );
    this.selection =
      container.parentElement?.querySelector<SVGElement>(
        "[data-canvas-drag-selection]",
      ) ?? null;
  }

  move(pointer: Position, scale = this.scale, snap = this.snap) {
    if (!this.active) return;
    this.updatePointer(pointer, scale, snap);
    if (!this.hasMoved || this.frame !== null) return;
    this.frame = requestAnimationFrame(() => {
      this.frame = null;
      if (this.active) this.paint();
    });
  }

  finish(pointer?: Position, scale = this.scale, snap = this.snap) {
    if (!this.active) return null;
    if (pointer) this.updatePointer(pointer, scale, snap);
    this.clearFrame();
    const positions = this.hasMoved ? this.positions().positions : null;
    if (positions) this.paint();
    this.clearDecorations();
    this.active = false;
    return positions;
  }

  /** Run after React commits, including unchanged props after group correction. */
  settle(nodes: CanvasNodeData[], connections: CanvasConnection[]) {
    if (this.active) return;
    this.captureGraph(nodes, connections);
    this.restoreGraph();
  }

  cancel() {
    if (!this.active) return;
    this.clearFrame();
    this.restoreGraph();
    this.clearDecorations();
    this.active = false;
  }

  private restoreGraph() {
    for (const node of this.moving) {
      const el = this.elements.get(node.id);
      if (el)
        el.style.transform = `translate(${node.position.x}px, ${node.position.y}px)`;
    }
    this.paintEdges(new Map());
  }

  private updatePointer(pointer: Position, scale: number, snap: boolean) {
    this.pointer = pointer;
    this.scale = scale;
    this.snap = snap;
    if (
      Math.abs(pointer.x - this.options.start.x) > CANVAS_DRAG_THRESHOLD ||
      Math.abs(pointer.y - this.options.start.y) > CANVAS_DRAG_THRESHOLD
    )
      this.hasMoved = true;
  }

  private positions() {
    let dx = (this.pointer.x - this.options.start.x) / this.scale;
    let dy = (this.pointer.y - this.options.start.y) / this.scale;
    const anchor = this.initialPositions.values().next().value;
    if (this.snap && anchor) {
      dx =
        Math.round((anchor.x + dx) / CANVAS_DRAG_GRID_SIZE) *
          CANVAS_DRAG_GRID_SIZE -
        anchor.x;
      dy =
        Math.round((anchor.y + dy) / CANVAS_DRAG_GRID_SIZE) *
          CANVAS_DRAG_GRID_SIZE -
        anchor.y;
    }
    const moving = this.moving.map((node) => {
      const initial = this.initialPositions.get(node.id)!;
      return { ...node, position: { x: initial.x + dx, y: initial.y + dy } };
    });
    const bounds = nodeBounds(moving);
    const guides: Guide[] = [];
    if (this.snap && moving.length) {
      const x = closestAlignment(
        [bounds.left, bounds.right, (bounds.left + bounds.right) / 2],
        this.xCandidates,
      );
      const y = closestAlignment(
        [bounds.top, bounds.bottom, (bounds.top + bounds.bottom) / 2],
        this.yCandidates,
      );
      if (x) {
        dx += x.offset;
        guides.push({ axis: "x", value: x.value });
      }
      if (y) {
        dy += y.offset;
        guides.push({ axis: "y", value: y.value });
      }
    }
    const positions = new Map(
      this.moving.map((node): [string, Position] => {
        const initial = this.initialPositions.get(node.id)!;
        return [node.id, { x: initial.x + dx, y: initial.y + dy }];
      }),
    );
    return { positions, guides, delta: { x: dx, y: dy } };
  }

  private paint() {
    const { positions, guides, delta } = this.positions();
    for (const [id, position] of positions) {
      const el = this.elements.get(id);
      if (el)
        el.style.transform = `translate(${position.x}px, ${position.y}px)`;
    }
    this.paintEdges(positions);
    if (this.selection)
      this.selection.style.transform = `translate(${delta.x * this.scale}px, ${delta.y * this.scale}px)`;
    this.paintGuides(guides, positions);
    const target =
      findGroupDropTarget(this.movedIds, [
        ...this.groups,
        ...applyCanvasDragPositions(this.moving, positions),
      ])?.id ?? null;
    if (target !== this.dropTarget) {
      this.dropTarget = target;
      this.options.onDropTarget?.(target);
    }
    return positions;
  }

  private paintEdges(positions: ReadonlyMap<string, Position>) {
    for (const { from, to, paths } of this.edges) {
      const start = positions.get(from.id) ?? from.position;
      const end = positions.get(to.id) ?? to.position;
      const { path } = canvasConnectionGeometry(
        { x: start.x + from.width, y: start.y + from.height / 2 },
        { x: end.x, y: end.y + to.height / 2 },
      );
      for (const el of paths) el.setAttribute("d", path);
    }
  }

  private paintGuides(
    guides: Guide[],
    positions: ReadonlyMap<string, Position>,
  ) {
    const layer = this.guideLayer;
    if (!layer) return;
    const bounds = nodeBounds(applyCanvasDragPositions(this.moving, positions));
    layer.replaceChildren(
      ...guides.map(({ axis, value }) => {
        const line = layer.ownerDocument.createElementNS(
          "http://www.w3.org/2000/svg",
          "line",
        );
        const coords =
          axis === "x"
            ? [
                value,
                Math.min(bounds.top, this.stationaryBounds.top),
                value,
                Math.max(bounds.bottom, this.stationaryBounds.bottom),
              ]
            : [
                Math.min(bounds.left, this.stationaryBounds.left),
                value,
                Math.max(bounds.right, this.stationaryBounds.right),
                value,
              ];
        ["x1", "y1", "x2", "y2"].forEach((name, index) =>
          line.setAttribute(name, String(coords[index])),
        );
        line.setAttribute("stroke", "currentColor");
        line.setAttribute("stroke-width", "1");
        line.setAttribute("vector-effect", "non-scaling-stroke");
        return line;
      }),
    );
  }

  private clearFrame() {
    if (this.frame !== null) cancelAnimationFrame(this.frame);
    this.frame = null;
  }

  private clearDecorations() {
    this.guideLayer?.replaceChildren();
    if (this.selection) this.selection.style.transform = "";
    if (this.dropTarget !== null) this.options.onDropTarget?.(null);
    this.dropTarget = null;
  }
}
