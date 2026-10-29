import type { CanvasEditorState } from "../lib/canvas-document-adapter";
import { isProjectID } from "@/components/projects/project";
export type TimelineResult = {
  jobID: string;
  assetID: string;
  fileName: string;
};
/** A job has one result node; retries never duplicate a confirmed delivery. */
export function timelineResultGraph(
  graph: CanvasEditorState,
  sourceID: string,
  result: TimelineResult,
): CanvasEditorState {
  const source = graph.nodes.find((node) => node.id === sourceID);
  if (
    !source ||
    source.type !== "timeline" ||
    !isProjectID(result.jobID) ||
    !isProjectID(result.assetID)
  )
    throw new Error("时间线或导出结果身份无效");
  const existing = graph.nodes.find((node) => node.id === result.jobID);
  if (existing) {
    if (
      existing.type !== "video" ||
      !existing.documentData?.fileReferences?.some(
        (ref) => ref.purpose === "CONTENT" && ref.fileId === result.assetID,
      )
    )
      throw new Error("导出结果节点身份冲突");
    return graph;
  }
  const position = {
    x: source.position.x + source.width + 80,
    y: source.position.y,
  };
  const title = result.fileName.trim().slice(0, 128) || "时间线导出";
  return {
    ...graph,
    nodes: [
      ...graph.nodes,
      {
        id: result.jobID,
        type: "video",
        title,
        position,
        width: 420,
        height: 236,
        metadata: { content: "", status: "success" },
        documentData: {
          id: result.jobID,
          type: "video",
          nodeTypeVersion: 1,
          title,
          position,
          width: 420,
          height: 236,
          zIndex: graph.nodes.length,
          parameters: { mode: "MANUAL" },
          fileReferences: [
            {
              referenceKey: result.assetID,
              purpose: "CONTENT",
              fileId: result.assetID,
              version: "1",
            },
          ],
        },
      },
    ],
  };
}
