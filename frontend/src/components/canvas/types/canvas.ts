import type { RichTextDocument } from "@/components/canvas/runtime/components/editor/types/rich-text";
export type Position = {
  x: number;
  y: number;
};

export type ViewportTransform = {
  x: number;
  y: number;
  k: number;
};

export enum CanvasNodeType {
  Image = "image",
  Text = "text",
  Config = "config",
  Video = "video",
  Audio = "audio",
  Group = "group",
}

// Node types are open strings: built-ins use CanvasNodeType and plugins use "<pluginId>:<name>".
export type CanvasNodeTypeId = CanvasNodeType | (string & {});

export type CanvasNodeStatus = "idle" | "success" | "loading" | "error";
export type CanvasGenerationMode = "text" | "image" | "video" | "audio";
export type CanvasImageGenerationType = "generation" | "edit";

export type CanvasNodeImage = {
  id: string;
  status: CanvasNodeStatus;
  errorDetails?: string;
  content: string;
  storageKey?: string;
  naturalWidth: number;
  naturalHeight: number;
  bytes: number;
  mimeType: string;
};

export type CanvasNodeText = {
  richText?: RichTextDocument;
  id: string;
  status: CanvasNodeStatus;
  errorDetails?: string;
  content: string;
};

export type CanvasNodeMetadata = {
  generationOutput?: {
    commandId: string;
    files: CanvasContract.CanvasFileReferenceDTO[];
  };
  richText?: RichTextDocument;
  content?: string;
  composerContent?: string;
  prompt?: string;
  status?: CanvasNodeStatus;
  errorDetails?: string;
  fontSize?: number;
  textBackground?: "default" | "cyan" | "blue" | "purple" | "orange" | "yellow";
  colorTags?: ("cyan" | "blue" | "purple" | "orange" | "yellow")[];
  generationMode?: CanvasGenerationMode;
  generationType?: CanvasImageGenerationType;
  model?: string;
  reasoningEffort?: "auto" | "low" | "medium" | "high" | "xhigh";
  size?: string;
  quality?: string;
  background?: string;
  count?: number;
  textCount?: number;
  texts?: CanvasNodeText[];
  primaryTextId?: string;
  seconds?: string;
  vquality?: string;
  generateAudio?: string;
  watermark?: string;
  videoMode?: string;
  audioVoice?: string;
  audioFormat?: string;
  audioSpeed?: string;
  audioInstructions?: string;
  references?: string[];
  naturalWidth?: number;
  naturalHeight?: number;
  freeResize?: boolean;
  images?: CanvasNodeImage[];
  primaryImageId?: string;
  storageKey?: string;
  mimeType?: string;
  bytes?: number;
  durationMs?: number;
  videoTaskId?: string;
  videoTaskProvider?: "agent";
  groupId?: string;
  interactive?: boolean; // Plugin node interaction/move state; see CanvasNodeDefinition.interactionToggle.
};

export type CanvasNodeData = {
  serviceData?: API.NodeDTO;
  /** Target-local display projection; never persisted in the graph. */
  referenceProjection?: {
    kind: "image" | "video" | "audio" | "text";
    label: string;
    files: CanvasContract.CanvasFileReferenceDTO[];
  };
  documentData?: CanvasContract.CanvasDocumentNodeDTO;
  id: string;
  type: CanvasNodeTypeId;
  title: string;
  position: Position;
  width: number;
  height: number;
  metadata?: CanvasNodeMetadata;
};

export type CanvasConnection = {
  serviceData?: NonNullable<API.DocumentDTO["edges"]>[number];
  documentData?: CanvasContract.CanvasDocumentConnectionDTO;
  id: string;
  fromNodeId: string;
  toNodeId: string;
};

export type CanvasAssistantReference = {
  id: string;
  type: CanvasNodeTypeId;
  title: string;
  dataUrl?: string;
  storageKey?: string;
  text?: string;
};

export type CanvasAssistantMessage = {
  id: string;
  role: "user" | "assistant" | "system" | "tool" | "error";
  title?: string;
  text: string;
  meta?: string;
  detail?: unknown;
  references?: CanvasAssistantReference[];
};

export type CanvasAssistantSession = {
  id: string;
  title: string;
  messages: CanvasAssistantMessage[];
  createdAt: string;
  updatedAt: string;
};

export type ConnectionHandle = {
  nodeId: string;
  handleType: "source" | "target";
};

export type SelectionBox = {
  startWorldX: number;
  startWorldY: number;
  currentWorldX: number;
  currentWorldY: number;
  additive: boolean;
  initialSelectedNodeIds: string[];
};

export type ContextMenuState =
  | { type: "canvas"; x: number; y: number }
  | {
      type: "node";
      x: number;
      y: number;
      nodeId: string;
    }
  | {
      type: "connection";
      x: number;
      y: number;
      connectionId: string;
    };
