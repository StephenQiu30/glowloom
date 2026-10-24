import type { CanvasSnapshot, CanvasGraphShape } from "./model.mjs";
export type ServiceDocument = {
  id: string;
  project_id: string;
  name: string;
  schema_version: 1;
  operation_version: 1;
  nodes: Record<string, unknown>[];
  edges: Record<string, unknown>[];
  [key: string]: unknown;
};
export type ServiceGraph = CanvasGraphShape & {
  serviceVersion: 1;
  documentId: string;
  projectId: string;
  nodes: {
    id: string;
    type: string;
    nodeTypeVersion: 1;
    title: string;
    position: { x: number; y: number };
    width?: number;
    height?: number;
    parentId: string | null;
    zIndex: number;
    native: Record<string, unknown>;
    references?: Record<string, unknown>[];
    labelState?: Record<string, unknown>;
    parameters?: Record<string, unknown>;
    textDocument?: import("./model.mjs").CanvasTextDraft;
  }[];
  connections: {
    id: string;
    fromNodeId: string;
    toNodeId: string;
    native: Record<string, unknown>;
  }[];
};
export function serviceSnapshot(
  document: ServiceDocument,
): CanvasSnapshot<ServiceGraph>;
export function serviceProjection<T extends ServiceDocument>(
  snapshot: CanvasSnapshot<ServiceGraph>,
  base: T,
): T;
