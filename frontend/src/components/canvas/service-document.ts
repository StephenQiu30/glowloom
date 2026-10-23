import { isProjectID } from "@/components/projects/project";
import { emptyDocument } from "./document";
import type {
  CanvasDocument,
  CanvasGraph,
  CanvasNodeData,
  NodeType,
} from "./types";

export type Operation = Record<string, unknown> & { type: string };
export type Effect = {
  entity: string;
  id: string;
  path: string[];
  index: number;
  after_exists: boolean;
  after?: unknown;
};
export type Change = { revision: string; effects: Effect[]; op_id?: string };
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
        (n.config?.format !== "plain" || n.config?.format_version !== 1))
    )
      throw new Error("此画布包含尚未适配的业务节点，原数据已保留");
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
          !["text", "format", "format_version", "collapsed"].includes(key),
      )
    )
      throw new Error("节点配置尚未适配，原数据已保留");
    return {
      id: n.id,
      type,
      title: n.title ?? "",
      position: { x: n.x!, y: n.y! },
      width: n.width!,
      height: n.height!,
      metadata: {
        content: type === "text" ? (n.config?.text ?? "") : "",
        groupId: n.parent_id ?? undefined,
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
      throw new Error("画布包含尚未适配的关系");
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

/** Minimal contiguous edit in UTF-16 units; never splits surrogate pairs. */
export function textEdit(before: string, after: string) {
  const a = Array.from(before),
    b = Array.from(after);
  let start = 0,
    end = 0;
  while (start < a.length && start < b.length && a[start] === b[start]) start++;
  while (
    end < a.length - start &&
    end < b.length - start &&
    a[a.length - 1 - end] === b[b.length - 1 - end]
  )
    end++;
  const retain = a.slice(0, start).join("").length;
  const removed = a.slice(start, a.length - end).join("").length;
  const inserted = b.slice(start, b.length - end).join("");
  return [
    ...(retain ? [{ retain }] : []),
    ...(removed ? [{ delete: removed }] : []),
    ...(inserted ? [{ insert: [{ type: "text", text: inserted }] }] : []),
  ];
}
function nodeDTO(n: CanvasNodeData) {
  const media = ["image", "video", "audio"].includes(n.type);
  if (media && !isProjectID(n.metadata?.assetId))
    throw new Error("请先将素材上传到项目，再加入画布");
  return {
    id: n.id,
    node_type: n.type,
    node_action: "resource",
    title: n.title,
    x: n.position.x,
    y: n.position.y,
    width: n.width,
    height: n.height,
    parent_id: n.metadata?.groupId ?? null,
    z_index: n.type === "group" ? 0 : 1,
    config:
      n.type === "text"
        ? {
            text: n.metadata?.content ?? "",
            format: "plain",
            format_version: 1,
          }
        : {},
    ...(media ? { media_ref: { asset_id: n.metadata!.assetId } } : {}),
  };
}
export function graphOperations(
  before: CanvasGraph,
  after: CanvasGraph,
): Operation[] {
  const operations: Operation[] = [];
  const old = new Map(before.nodes.map((n) => [n.id, n]));
  const next = new Map(after.nodes.map((n) => [n.id, n]));
  for (const edge of before.connections)
    if (
      !after.connections.some((n) => n.id === edge.id) &&
      next.has(edge.fromNodeId) &&
      next.has(edge.toNodeId)
    )
      operations.push({ type: "disconnect", node_id: edge.id });
  // Create groups before members; delete members before groups.
  for (const n of [...after.nodes].sort(
    (a, b) => Number(b.type === "group") - Number(a.type === "group"),
  )) {
    const previous = old.get(n.id);
    if (!previous) {
      operations.push({ type: "create_node", node: nodeDTO(n) });
      continue;
    }
    if (n.type !== previous.type) throw new Error("不能改变已有节点类型");
    const set = (path: string[], value: unknown) =>
      operations.push({ type: "set", node_id: n.id, path, value });
    if (n.title !== previous.title) set(["title"], n.title);
    if (
      n.position.x !== previous.position.x ||
      n.position.y !== previous.position.y
    )
      set(["position"], n.position);
    if (n.width !== previous.width || n.height !== previous.height)
      set(["size"], { width: n.width, height: n.height });
    if (n.metadata?.groupId !== previous.metadata?.groupId)
      set(["parent_id"], n.metadata?.groupId ?? null);
    if (n.type === "text" && n.metadata?.content !== previous.metadata?.content)
      operations.push({
        type: "text",
        node_id: n.id,
        path: ["config", "text"],
        text: textEdit(
          previous.metadata?.content ?? "",
          n.metadata?.content ?? "",
        ),
      });
    if (n.metadata?.assetId !== previous.metadata?.assetId) {
      if (
        !isProjectID(n.metadata?.assetId) ||
        !isProjectID(previous.metadata?.assetId)
      )
        throw new Error("素材替换缺少持久身份");
      operations.push({
        type: "replace_media",
        node_id: n.id,
        expected_media_ref: { asset_id: previous.metadata.assetId },
        new_media_ref: { asset_id: n.metadata.assetId },
      });
    }
    if (
      n.type !== "text" &&
      n.metadata?.content !== previous.metadata?.content &&
      n.metadata?.assetId === previous.metadata?.assetId
    )
      throw new Error("处理后的素材需要先上传，不能将本地地址保存为素材");
  }
  for (const n of [...before.nodes].sort(
    (a, b) => Number(a.type === "group") - Number(b.type === "group"),
  ))
    if (!next.has(n.id))
      operations.push({ type: "delete_node", node_id: n.id });
  for (const e of after.connections)
    if (!before.connections.some((n) => n.id === e.id))
      operations.push({
        type: "connect",
        edge: {
          id: e.id,
          edge_type: "annotation",
          source_node_id: e.fromNodeId,
          target_node_id: e.toNodeId,
        },
      });
  if (operations.length > 100)
    throw new Error("单次修改超过 100 项，请减少选择后重试");
  return operations;
}
export function applyChanges(
  base: API.DocumentDTO,
  changes: Change[],
): API.DocumentDTO {
  const result = structuredClone(base);
  for (const change of changes) {
    if (
      BigInt(revision(change.revision)) !==
      BigInt(revision(result.revision)) + BigInt(1)
    )
      throw new Error("画布历史不连续，需要重新同步");
    for (const effect of change.effects) {
      if (
        !["node", "edge"].includes(effect.entity) ||
        !Array.isArray(effect.path) ||
        effect.path.some((key) =>
          ["__proto__", "constructor", "prototype"].includes(key),
        )
      )
        throw new Error("画布变化格式无效");
      const list = (effect.entity === "node" ? result.nodes : result.edges)!;
      const index = list.findIndex((item) => item.id === effect.id);
      if (!effect.path.length) {
        if (index >= 0) list.splice(index, 1);
        if (effect.after_exists)
          list.splice(
            Math.max(0, Math.min(effect.index, list.length)),
            0,
            structuredClone(effect.after) as API.NodeDTO & API.Edge,
          );
      } else {
        if (index < 0) throw new Error("画布变化引用了不存在的节点");
        let object = list[index] as Record<string, unknown>;
        for (const key of effect.path.slice(0, -1)) {
          if (object[key] == null) object[key] = {};
          if (typeof object[key] !== "object" || Array.isArray(object[key]))
            throw new Error("画布变化路径无效");
          object = object[key] as Record<string, unknown>;
        }
        const key = effect.path.at(-1)!;
        if (effect.after_exists) object[key] = structuredClone(effect.after);
        else delete object[key];
      }
    }
    result.revision = change.revision;
  }
  return result;
}
