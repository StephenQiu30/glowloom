import { validateCanvasComposer } from "./text-content";
import { isProjectID } from "@/components/projects/project";
import { emptyDocument } from "./document";
import type { CanvasDocument, CanvasNodeData, NodeType } from "./types";

export class UnsupportedCanvasContentError extends Error {}
export function revision(value: unknown): string {
  if (
    typeof value !== "string" ||
    !/^(0|[1-9]\d*)$/.test(value) ||
    BigInt(value) > BigInt("9223372036854775807")
  )
    throw new Error("画布版本无效");
  return value;
}
export function fromServiceDocument(
  dto: API.DocumentDTO,
  canvasID: string,
  projectID: string,
): CanvasDocument {
  if (
    dto.id !== canvasID ||
    dto.project_id !== projectID ||
    dto.schema_version !== 1 ||
    dto.operation_version !== 1 ||
    !dto.permissions?.read ||
    !Array.isArray(dto.nodes) ||
    !Array.isArray(dto.edges)
  )
    throw new Error("画布身份或协议不匹配，已停止编辑");
  revision(dto.revision);
  const nodes = dto.nodes.map((n): CanvasNodeData => {
    const type = n.node_type as NodeType;
    if (
      !isProjectID(n.id) ||
      !["text", "image", "video", "audio", "group"].includes(type) ||
      n.node_action !== "resource" ||
      n.last_operation_id ||
      (type === "text" &&
        (!["plain", "tiptap"].includes(n.config?.format ?? "") ||
          n.config?.format_version !== 1))
    )
      throw new UnsupportedCanvasContentError(
        "此画布包含尚未适配的业务节点，原数据已保留",
      );
    if (
      ![n.x, n.y, n.width, n.height].every(
        (v) => typeof v === "number" && Number.isFinite(v),
      ) ||
      n.width! <= 0 ||
      n.height! <= 0
    )
      throw new Error("画布节点尺寸无效");
    if (
      ["image", "video", "audio"].includes(type) &&
      !isProjectID(n.media_ref?.asset_id)
    )
      throw new Error("素材身份缺失，已停止编辑");
    if (
      Object.keys(n.config ?? {}).some(
        (key) =>
          ![
            "text",
            "composer",
            "format",
            "format_version",
            "collapsed",
          ].includes(key),
      )
    )
      throw new UnsupportedCanvasContentError("节点配置尚未适配，原数据已保留");
    const composer: unknown = n.config?.composer;
    if (type === "text" && n.config?.format === "tiptap") {
      validateCanvasComposer(composer);
      if (n.config.text !== undefined)
        throw new Error("文本配置包含两个内容来源");
    } else if (composer !== undefined) throw new Error("节点引用格式无效");
    return {
      id: n.id,
      type,
      title: n.title ?? "",
      position: { x: n.x!, y: n.y! },
      width: n.width!,
      height: n.height!,
      metadata: {
        content: type === "text" ? (n.config?.text ?? "") : "",
        ...(type === "text" && n.config?.format === "tiptap"
          ? {
              composer:
                composer as import("@/components/generation/prompt-document").PromptDraft,
            }
          : {}),
        groupId: n.parent_id ?? undefined,
        ...(type === "group"
          ? { collapsed: n.config?.collapsed ?? false }
          : {}),
        assetId: n.media_ref?.asset_id,
      },
    };
  });
  const ids = new Set(nodes.map((n) => n.id));
  if (
    ids.size !== nodes.length ||
    nodes.some((n) => n.metadata?.groupId && !ids.has(n.metadata.groupId))
  )
    throw new Error("画布分组关系无效");
  const connections = dto.edges.map((e) => {
    if (
      !isProjectID(e.id) ||
      e.edge_type !== "annotation" ||
      e.role ||
      e.binding ||
      !ids.has(e.source_node_id!) ||
      !ids.has(e.target_node_id!)
    )
      throw new UnsupportedCanvasContentError("画布包含尚未适配的关系");
    return {
      id: e.id,
      fromNodeId: e.source_node_id!,
      toNodeId: e.target_node_id!,
    };
  });
  return {
    ...emptyDocument,
    serviceRevision: dto.revision,
    nodes,
    connections,
  };
}

/** A full read-only snapshot never becomes the editable graph or a save payload. */
export function validateHistoricalSnapshot(dto: API.DocumentDTO) {
  const ids = new Set<string>();
  for (const node of dto.nodes ?? []) {
    if (
      !isProjectID(node.id) ||
      ids.has(node.id) ||
      ![node.x, node.y, node.width, node.height].every(
        (v) => typeof v === "number" && Number.isFinite(v),
      ) ||
      node.width! <= 0 ||
      node.height! <= 0
    )
      throw new Error("画布节点身份或尺寸无效");
    ids.add(node.id);
  }
  const edgeIDs = new Set<string>();
  for (const edge of dto.edges ?? []) {
    if (
      !isProjectID(edge.id) ||
      edgeIDs.has(edge.id) ||
      !ids.has(edge.source_node_id!) ||
      !ids.has(edge.target_node_id!)
    )
      throw new Error("画布关系无效");
    edgeIDs.add(edge.id);
  }
}

/** Minimal contiguous edit in UTF-16 units; never splits surrogate pairs. */
