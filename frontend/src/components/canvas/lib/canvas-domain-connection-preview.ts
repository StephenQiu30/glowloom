import type { CanvasEditorState } from "./canvas-document-adapter";
import { assertCanvasSubjectSourcePreview } from "./canvas-subject-connection-preview";
import type { CanvasNodeData } from "../types/canvas";
import { canvasConnectionMediaAssetId } from "./canvas-timeline-connections";

export const isCanvasDomainTarget = (type: string) =>
  type === "timeline" || type === "subject";

/** Cheap synchronous preview; the final command must resolve and revalidate every asset. */
export function assertCanvasDomainSources(
  sources: readonly CanvasNodeData[],
  targetType: string,
) {
  if (
    !isCanvasDomainTarget(targetType) ||
    !sources.length ||
    sources.some((node) => !["image", "video", "audio"].includes(node.type))
  )
    throw new Error("无法连接这些节点");
  if (targetType === "timeline" && sources.length > 50)
    throw new Error("一次最多添加50个素材");
  for (const source of sources) canvasConnectionMediaAssetId(source);
}

export function validateCanvasDomainConnectionPreview(
  state: CanvasEditorState,
  sourceIDs: string[],
  targetID: string,
  resolveMedia: (node: CanvasNodeData) => CanvasNodeData = (node) => node,
) {
  const target = state.nodes.find((node) => node.id === targetID);
  if (!target) throw new Error("连接目标已不存在");
  const sources = sourceIDs.map((id) => {
    const node = state.nodes.find((item) => item.id === id);
    if (!node) throw new Error("来源节点已不存在");
    return resolveMedia(node);
  });
  assertCanvasDomainSources(sources, target.type);
  if (target.type === "subject")
    assertCanvasSubjectSourcePreview(sources, { target });
  const selected = new Set(sourceIDs);
  if (selected.has(targetID)) throw new Error("不能连接节点自身");
  if (
    state.connections.some(
      (edge) => selected.has(edge.fromNodeId) && edge.toNodeId === targetID,
    )
  )
    throw new Error("这项内容已经关联，请勿重复连接");
  const visited = new Set<string>();
  const pending = [targetID];
  while (pending.length) {
    const id = pending.pop()!;
    if (selected.has(id)) throw new Error("这条连接会形成循环，请调整连接方向");
    if (visited.has(id)) continue;
    visited.add(id);
    pending.push(
      ...state.connections
        .filter((edge) => edge.fromNodeId === id)
        .map((edge) => edge.toNodeId),
    );
  }
}
