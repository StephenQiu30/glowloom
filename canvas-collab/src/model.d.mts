import type * as Y from "yjs";

export type CanvasGraphShape = {
  schemaVersion: number;
  nodes: { id: string }[];
  connections: { id: string }[];
};
export function canvasGraphHash(graph: CanvasGraphShape): Promise<string>;
export type CanvasSnapshot<G extends CanvasGraphShape = CanvasGraphShape> = {
  title: string;
  graph: G;
};
export type NodeTextBinding = {
  fragment: Y.XmlFragment;
  markChanged: () => void | boolean;
  isAttached?: () => boolean;
  protectDraft?: (document: CanvasTextDraft | null) => void;
  registerDraftReader?: (reader: () => CanvasTextDraft | null) => () => void;
};
export type CanvasTextDraft = {
  schemaVersion: 1;
  content: import("@tiptap/core").JSONContent;
};
export function createCanvasDocument<G extends CanvasGraphShape>(
  value: CanvasSnapshot<G>,
): Y.Doc;
export function readCanvasDocument<
  G extends CanvasGraphShape = CanvasGraphShape,
>(doc: Y.Doc): CanvasSnapshot<G>;
export function applyCanvasChanges<G extends CanvasGraphShape>(
  doc: Y.Doc,
  before: CanvasSnapshot<G>,
  after: CanvasSnapshot<G>,
  origin?: unknown,
): void;
export function nodeTextBinding(doc: Y.Doc, nodeId: string): NodeTextBinding;
export function applyRecoveredCanvasChanges<G extends CanvasGraphShape>(
  doc: Y.Doc,
  before: CanvasSnapshot<G>,
  after: CanvasSnapshot<G>,
): void;
export function canvasUndoManager(doc: Y.Doc): Y.UndoManager;
export type CanvasRestoration = {
  id: string;
  epoch: string;
  sourceCommitSeq: string;
  nodes: string[];
  connections: string[];
};
export function applyCanvasHistory(
  doc: Y.Doc,
  manager: Y.UndoManager,
  direction: "undo" | "redo",
  scope: { epoch: string; commitSeq: string },
): { nodes: string[]; connections: string[] };
export function inspectCanvasDocument(doc: Y.Doc): unknown;
export function validateCanvasTransition(
  before: unknown,
  doc: Y.Doc,
): CanvasRestoration | null;
