import type { CanvasEditorState } from "./canvas-document-adapter";

export type CanvasGraphResult = Pick<
  CanvasEditorState,
  "nodes" | "connections"
>;

/** Admit the complete graph before exposing any part of a connection operation. */
export function commitCanvasGraphResult(
  result: CanvasGraphResult,
  admit: (result: CanvasGraphResult) => boolean,
  commit: (result: CanvasGraphResult) => void,
): boolean {
  if (!admit(result)) return false;
  commit(result);
  return true;
}
