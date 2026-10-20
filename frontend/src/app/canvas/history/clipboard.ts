import {
  commandBatch,
  duplicateNode,
  isAnnotation,
  isEditableNode,
  type Node,
} from "../document";
import { descendants } from "../viewport/render-model";
import { isProjectID } from "@/components/projects/project";

const maximumBytes = 2 * 1024 * 1024;
const canvasID = "00000000-0000-4000-8000-000000000001";
function validate(projectID: string, nodes: Node[], edges: API.Edge[]) {
  if (
    !isProjectID(projectID) ||
    !Array.isArray(nodes) ||
    !nodes.length ||
    !Array.isArray(edges)
  )
    throw new Error("请选择可编辑节点后复制。");
  if (
    nodes.some(
      (n) =>
        !isEditableNode(n) ||
        n.config.draft?.composer?.references.some(
          (r) => r.source.kind === "pending_upload",
        ),
    )
  )
    throw new Error("只读节点或尚未完成的上传不能复制。");
  commandBatch(
    { id: canvasID, project_id: projectID, revision: 1, nodes: [], edges: [] },
    nodes,
    { edges },
  );
}
/** Preserve durable media identities while giving copied tools independent node references. */
export function remapCopiedNodeReferences(
  originals: readonly Node[],
  copies: readonly Node[],
  sourceNodes: readonly Node[] = originals,
): Node[] {
  if (
    originals.length !== copies.length ||
    new Set(originals.map((node) => node.id)).size !== originals.length ||
    new Set(copies.map((node) => node.id)).size !== copies.length ||
    copies.some((node) => !isProjectID(node.id))
  )
    throw new Error("节点副本身份不完整，无法复制。");
  const identities = new Map(
    originals.map((node, index) => [node.id, copies[index].id]),
  );
  const sources = new Map(
    [...sourceNodes, ...originals].map((node) => [node.id, node]),
  );
  function resolve(
    reference: string,
    assetID: string | undefined,
    kind: string | undefined,
  ) {
    const source = sources.get(reference);
    if (
      !source ||
      source.node_type !== kind ||
      source.ref_type !== "media_asset" ||
      !isProjectID(source.ref_id)
    )
      throw new Error("无法解析工具引用的素材来源，请重新选择来源后复制。");
    if (assetID != null && assetID !== source.ref_id)
      throw new Error("工具素材身份与来源不一致，无法复制。");
    return { reference: identities.get(reference), assetID: source.ref_id };
  }
  return copies.map((copy) => {
    if (!copy.config.director && !copy.config.timeline) return copy;
    const config = { ...copy.config };
    if (config.director)
      config.director = {
        ...config.director,
        objects: config.director.objects?.map((object) => {
          if (object.source_node_id == null) return object;
          const binding = resolve(
            object.source_node_id,
            object.asset_id,
            object.kind === "billboard" ? "image" : "model",
          );
          return {
            ...object,
            source_node_id: binding.reference,
            asset_id: binding.assetID,
          };
        }),
      };
    if (config.timeline)
      config.timeline = {
        ...config.timeline,
        clips: config.timeline.clips?.map((clip) => {
          if (clip.node_id == null) return clip;
          const binding = resolve(clip.node_id, clip.asset_id, clip.kind);
          return {
            ...clip,
            node_id: binding.reference,
            asset_id: binding.assetID,
          };
        }),
      };
    return { ...copy, config };
  });
}

export function encodeCanvasClipboard(
  projectID: string,
  nodes: Node[],
  edges: API.Edge[],
  selected: Set<string>,
) {
  const ids = descendants(nodes, selected);
  const copied = nodes
    .filter((n) => ids.has(n.id))
    .map((n) => ({
      ...n,
      parent_id: n.parent_id && ids.has(n.parent_id) ? n.parent_id : undefined,
    }));
  const lines = edges.filter(
    (e) =>
      isAnnotation(e) &&
      ids.has(e.source_node_id ?? "") &&
      ids.has(e.target_node_id ?? ""),
  );
  validate(projectID, copied, lines);
  const text = JSON.stringify({
    type: "glowloom.canvas",
    version: 1,
    project_id: projectID,
    nodes: remapCopiedNodeReferences(copied, copied, nodes),
    edges: lines,
  });
  if (new TextEncoder().encode(text).length > maximumBytes)
    throw new Error("所选节点过大，请分批复制。");
  return text;
}
export function pasteCanvasClipboard(
  text: string,
  projectID: string,
  point?: { x: number; y: number },
) {
  if (
    text.length > maximumBytes ||
    new TextEncoder().encode(text).length > maximumBytes
  )
    throw new Error("剪贴板内容过大。");
  let value;
  try {
    value = JSON.parse(text);
  } catch {
    throw new Error("剪贴板中没有画布节点。");
  }
  if (
    value?.type !== "glowloom.canvas" ||
    value.version !== 1 ||
    value.project_id !== projectID
  )
    throw new Error("只能粘贴当前项目的画布节点。");
  validate(projectID, value.nodes, value.edges);
  const originals = value.nodes as Node[],
    copies = remapCopiedNodeReferences(originals, originals.map(duplicateNode));
  const identities = new Map(originals.map((n, i) => [n.id, copies[i].id]));
  const minX = Math.min(...originals.map((n) => n.x)),
    minY = Math.min(...originals.map((n) => n.y));
  for (let i = 0; i < copies.length; i++) {
    copies[i].parent_id = originals[i].parent_id
      ? identities.get(originals[i].parent_id!)
      : undefined;
    if (point) {
      copies[i].x = originals[i].x - minX + point.x;
      copies[i].y = originals[i].y - minY + point.y;
    }
  }
  const edges = (value.edges as API.Edge[]).map((e) => ({
    ...e,
    id: crypto.randomUUID(),
    source_node_id: identities.get(e.source_node_id!),
    target_node_id: identities.get(e.target_node_id!),
  }));
  validate(projectID, copies, edges);
  return { nodes: copies, edges };
}
