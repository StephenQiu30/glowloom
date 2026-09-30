// 编辑器消费模型；服务端 DTO 只由生成客户端提供，queries 显式转换。
export enum CanvasNodeType {
  Text = "text",
  Image = "image",
  Video = "video",
  Audio = "audio",
  Frame = "group",
}
export type Position = { x: number; y: number };
export type ViewportTransform = Position & { k: number };
export type CanvasNodeData = {
  id: string;
  type: CanvasNodeType;
  title: string;
  position: Position;
  width: number;
  height: number;
  parentId?: string;
  zIndex: number;
  assetId?: string;
  media?: {
    url: string;
    name: string;
    mimeType: string;
    width?: number;
    height?: number;
    bytes?: number;
    durationMs?: number;
    expiresAt?: string;
  };
  metadata?: {
    content?: string;
    locked?: boolean;
    frame?: {
      collapsed: boolean;
      expandedWidth: number;
      expandedHeight: number;
    };
  };
};
export type CanvasConnection = {
  id: string;
  fromNodeId: string;
  toNodeId: string;
};
/** 折叠只改变显示投影，服务端仍保存展开尺寸。 */
export function canvasNodeRenderHeight(node: {
  height: number;
  type?: CanvasNodeType;
  metadata?: CanvasNodeData["metadata"];
}) {
  return node.type === CanvasNodeType.Frame && node.metadata?.frame?.collapsed
    ? 84
    : node.height;
}
export type ConnectionHandle = {
  nodeId: string;
  handleType: "source" | "target";
};
export type CanvasSelectionStrategy = "replace" | "add" | "toggle" | "subtract";
export type CanvasSelectionHitMode = "contain" | "intersect";
export type SelectionBox = {
  startWorldX: number;
  startWorldY: number;
  currentWorldX: number;
  currentWorldY: number;
  strategy: CanvasSelectionStrategy;
  hitMode: CanvasSelectionHitMode;
  initialSelectedNodeIds: string[];
};
export type CanvasDocument = {
  id: string;
  projectId: string;
  name: string;
  revision: number;
  scope: Record<string, unknown>;
  viewport: ViewportTransform;
  nodes: CanvasNodeData[];
  connections: CanvasConnection[];
};
export type CanvasCommand =
  | { type: "AddNodes"; nodes: CanvasNodeData[] }
  | { type: "MoveNodes"; moves: { id: string; x: number; y: number }[] }
  | {
      type: "UpdateNodeConfig";
      id: string;
      config: { text?: string; collapsed?: boolean };
    }
  | { type: "DeleteNodes"; ids: string[] }
  | { type: "Connect"; edges: CanvasConnection[] }
  | { type: "Disconnect"; ids: string[] }
  | { type: "SetViewport"; viewport: ViewportTransform }
  | {
      type: "ResizeNodes";
      sizes: { id: string; width: number; height: number }[];
    }
  | { type: "RenameNodes"; names: { id: string; title: string }[] }
  | {
      type: "SetNodeParents";
      parents: { id: string; parent_id: string | null }[];
    }
  | { type: "SetNodeZIndex"; z_indices: { id: string; z_index: number }[] };
