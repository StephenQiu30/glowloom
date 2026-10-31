import type { CanvasEditorState } from "./canvas-document-adapter";

type Graph = Pick<CanvasEditorState, "nodes" | "connections">;
export type CanvasRemoteApplication = Graph & { previous: Graph };

/** Suppress the render preceding a remote layout effect, not later local edits. */
export function classifyCanvasRemoteApplication(
  application: CanvasRemoteApplication,
  rendered: Graph,
): "waiting" | "echo" | "local" {
  if (
    rendered.nodes === application.nodes &&
    rendered.connections === application.connections
  )
    return "echo";
  if (
    rendered.nodes === application.previous.nodes &&
    rendered.connections === application.previous.connections
  )
    return "waiting";
  return "local";
}
