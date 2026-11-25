import type { PromptDraft } from "@/components/generation/prompt-document";
// Editing subset of basketikun/infinite-canvas's canvas model (MIT).
export const CanvasNodeType = {
  Text: "text",
  Image: "image",
  Video: "video",
  Audio: "audio",
  Group: "group",
} as const;
export type NodeType = (typeof CanvasNodeType)[keyof typeof CanvasNodeType];
export type Position = { x: number; y: number };
export type ViewportTransform = Position & { k: number };
export type CanvasNodeData = {
  id: string;
  type: NodeType;
  title: string;
  position: Position;
  width: number;
  height: number;
  metadata?: {
    content?: string;
    composer?: PromptDraft;
    groupId?: string;
    collapsed?: boolean;
    naturalWidth?: number;
    naturalHeight?: number;
    mimeType?: string;
    assetId?: string;
  };
};
export type CanvasConnection = {
  id: string;
  fromNodeId: string;
  toNodeId: string;
};
export type ConnectionHandle = {
  nodeId: string;
  handleType: "source" | "target";
};
export type CanvasGraph = {
  nodes: CanvasNodeData[];
  connections: CanvasConnection[];
};
export type CanvasDocument = CanvasGraph & {
  serviceRevision?: string;
  format: "glowloom-canvas";
  version: 1;
  viewport: ViewportTransform;
};
