import type { CanvasNodeData } from "../types/canvas";
import type { CanvasEditorState } from "../lib/canvas-document-adapter";
import {
  createCanvasNode,
  nextCanvasNodeTitle,
} from "../lib/canvas-node-factory";
import {
  readCinemaKit,
  kitKind,
  cinemaKinds,
  cinemaLabels,
  cinemaSupports,
  type CinemaKit,
  type CinemaKind,
} from "./cinema-kit-model";
export function makeCinemaNode(
  kit: CinemaKit,
  position: { x: number; y: number },
  nodes: CanvasNodeData[],
): CanvasNodeData {
  const value = readCinemaKit(kit),
    kind = kitKind(value),
    node = createCanvasNode("cinema_studio", position, {}, nodes);
  return {
    ...node,
    title: nextCanvasNodeTitle(cinemaLabels[kind], "cinema_studio", nodes),
    serviceData: {
      id: node.id,
      node_type: "cinema_studio",
      node_action: "tool",
      config: { cinema_kit: value },
    },
  };
}
export function effectiveCinema(
  graph: CanvasEditorState,
  nodeID: string,
): CinemaKit | undefined {
  const node = graph.nodes.find((n) => n.id === nodeID);
  if (!node) return;
  const out: CinemaKit = node.serviceData?.config?.cinema_kit
    ? readCinemaKit(node.serviceData.config.cinema_kit)
    : { version: 1 };
  for (const edge of graph.connections.filter((e) => e.toNodeId === nodeID)) {
    const source = graph.nodes.find((n) => n.id === edge.fromNodeId);
    if (source?.type !== "cinema_studio") continue;
    const value = readCinemaKit(source.serviceData?.config?.cinema_kit),
      kind = kitKind(value);
    if (!cinemaSupports(kind, node.type) || out[kind] !== undefined)
      throw new Error("工具箱引用存在冲突");
    Object.assign(out, { [kind]: value[kind] });
  }
  return cinemaKinds.some((k) => out[k] !== undefined) ? out : undefined;
}
export function updateCinemaSetting(
  graph: CanvasEditorState,
  nodeID: string,
  kind: CinemaKind,
  value?: CinemaKit,
): CanvasEditorState {
  const node = graph.nodes.find((n) => n.id === nodeID);
  if (!node || !cinemaSupports(kind, node.type))
    throw new Error("该节点不支持此设置");
  const next: CinemaKit = node.serviceData?.config?.cinema_kit
    ? readCinemaKit(node.serviceData.config.cinema_kit)
    : { version: 1 };
  delete next[kind];
  if (value) {
    const clean = readCinemaKit(value);
    if (kitKind(clean) !== kind) throw new Error("设置类别不一致");
    Object.assign(next, { [kind]: clean[kind] });
  }
  const settings = cinemaKinds.some((k) => next[k] !== undefined)
    ? next
    : undefined;
  return {
    ...graph,
    nodes: graph.nodes.map((n) =>
      n.id !== nodeID
        ? n
        : {
            ...n,
            serviceData: {
              ...n.serviceData,
              id: n.id,
              config: { ...n.serviceData?.config, cinema_kit: settings },
            },
          },
    ),
    connections: graph.connections.filter(
      (e) =>
        e.toNodeId !== nodeID ||
        kitKindSafe(graph.nodes.find((n) => n.id === e.fromNodeId)) !== kind,
    ),
  };
}
function kitKindSafe(node?: CanvasNodeData) {
  return node?.type === "cinema_studio"
    ? kitKind(readCinemaKit(node.serviceData?.config?.cinema_kit))
    : undefined;
}
