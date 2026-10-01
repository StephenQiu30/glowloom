import {
  createGenerationConfig,
  generationConfigSchema,
  encodeGenerationConfig,
  decodeGenerationConfig,
} from "./generation-config";
import {
  createDirectorConfig,
  directorSchema,
  encodeDirector,
  decodeDirector,
} from "./director/model";
import {
  createBatchTable,
  batchTableSchema,
  encodeBatchConfig,
  decodeBatchConfig,
} from "./batch-table";
import {
  createTimeline,
  timelineSchema,
  encodeTimeline,
  decodeTimeline,
} from "./timeline";
import {
  CanvasNodeType,
  type CanvasCommand,
  type CanvasDocument,
  type CanvasNodeData,
  type Position,
} from "./model";
import {
  applyCanvasHistoryPatch,
  createCanvasHistoryPatch,
} from "./engine/history";

export function createNode(
  type: CanvasNodeType,
  position: Position,
  id: string = crypto.randomUUID(),
  extra?: { assetId?: string; media?: CanvasNodeData["media"] },
): CanvasNodeData {
  const title = {
    text: "文字",
    image: "图片",
    video: "视频",
    audio: "音频",
    model: "3D 模型",
    group: "分组",
    batch_table: "批量创作",
    timeline: "时间线",
    director: "导演台",
    generation: "生成任务",
  }[type];
  return {
    id,
    type,
    title,
    position,
    width: type === CanvasNodeType.Frame ? 640 : 320,
    height: type === CanvasNodeType.Frame ? 420 : 220,
    zIndex: 0,
    ...extra,
    ...(type === CanvasNodeType.BatchTable
      ? { batchTable: createBatchTable() }
      : {}),
    ...(type === CanvasNodeType.Generation
      ? { generation: createGenerationConfig() }
      : {}),
    ...(type === CanvasNodeType.Director
      ? { director: createDirectorConfig() }
      : {}),
    ...(type === CanvasNodeType.Timeline ? { timeline: createTimeline() } : {}),
    metadata:
      type === CanvasNodeType.Text
        ? { content: "" }
        : type === CanvasNodeType.Frame
          ? {
              frame: {
                collapsed: false,
                expandedWidth: 640,
                expandedHeight: 420,
              },
            }
          : {},
  };
}
export function nodeConfig(node: CanvasNodeData) {
  if (node.type === CanvasNodeType.Generation)
    return { generation: encodeGenerationConfig(node.generation!) };
  if (node.type === CanvasNodeType.Director)
    return { director: encodeDirector(node.director!) };
  if (node.type === CanvasNodeType.BatchTable)
    return { batch_table: encodeBatchConfig(node.batchTable!) };
  if (node.type === CanvasNodeType.Timeline)
    return { timeline: encodeTimeline(node.timeline!) };
  return node.type === CanvasNodeType.Text
    ? { text: node.metadata?.content ?? "" }
    : node.type === CanvasNodeType.Frame
      ? { collapsed: node.metadata?.frame?.collapsed ?? false }
      : {};
}
function validPosition(value: number) {
  if (!Number.isFinite(value) || Math.abs(value) > 1e6)
    throw new Error("坐标超出允许范围。");
}
function validateNode(node: CanvasNodeData) {
  if (node.type === CanvasNodeType.Generation)
    generationConfigSchema.parse(node.generation);
  if (node.type === CanvasNodeType.Director)
    directorSchema.parse(node.director);
  if (node.type === CanvasNodeType.BatchTable)
    batchTableSchema.parse(node.batchTable);
  if (node.type === CanvasNodeType.Timeline)
    timelineSchema.parse(node.timeline);
  validPosition(node.position.x);
  validPosition(node.position.y);
  const maximum = node.type === CanvasNodeType.Frame ? 100000 : 2000;
  if (
    !Number.isFinite(node.width) ||
    !Number.isFinite(node.height) ||
    node.width < 40 ||
    node.height < 40 ||
    node.width > maximum ||
    node.height > maximum
  )
    throw new Error("节点尺寸超出允许范围。");
  if (!node.title.trim() || Array.from(node.title).length > 128)
    throw new Error("名称需为 1～128 字符。");
  if (Array.from(node.metadata?.content ?? "").length > 10000)
    throw new Error("文字最多 10,000 字符。");
  if (
    [
      CanvasNodeType.Image,
      CanvasNodeType.Video,
      CanvasNodeType.Audio,
      CanvasNodeType.Model,
    ].includes(node.type) &&
    !node.assetId
  )
    throw new Error("媒体节点需要有权访问的真实媒体资产。");
}
export function applyCommands(
  document: CanvasDocument,
  commands: CanvasCommand[],
): CanvasDocument {
  if (!commands.length || commands.length > 100)
    throw new Error("单次操作需为 1～100 条命令。");
  for (const command of commands) {
    const items =
      "nodes" in command
        ? command.nodes
        : "moves" in command
          ? command.moves
          : "sizes" in command
            ? command.sizes
            : "names" in command
              ? command.names
              : "parents" in command
                ? command.parents
                : "z_indices" in command
                  ? command.z_indices
                  : "edges" in command
                    ? command.edges
                    : "ids" in command
                      ? command.ids
                      : undefined;
    if (items && (!items.length || items.length > 2000))
      throw new Error("单条命令需为 1～2,000 个项目。");
  }
  let next = {
    ...document,
    nodes: [...document.nodes],
    connections: [...document.connections],
  };
  const requireNode = (id: string) => {
    const node = next.nodes.find((item) => item.id === id);
    if (!node) throw new Error("节点已不存在，请读取最新文档。");
    return node;
  };
  const patch = (id: string, update: Partial<CanvasNodeData>) => {
    requireNode(id);
    next.nodes = next.nodes.map((node) =>
      node.id === id ? { ...node, ...update } : node,
    );
  };
  for (const command of commands) {
    switch (command.type) {
      case "AddNodes":
        for (const node of command.nodes) {
          if (next.nodes.some((item) => item.id === node.id))
            throw new Error("节点 ID 已存在。");
          validateNode(node);
          next.nodes.push(node);
        }
        break;
      case "DeleteNodes": {
        const removed = new Set(command.ids);
        command.ids.forEach(requireNode);
        next.nodes = next.nodes
          .filter((node) => !removed.has(node.id))
          .map((node) =>
            node.parentId && removed.has(node.parentId)
              ? { ...node, parentId: undefined }
              : node,
          );
        next.nodes = next.nodes.map((node) => ({
          ...node,
          ...(node.batchTable
            ? {
                batchTable: {
                  ...node.batchTable,
                  rows: node.batchTable.rows.map((row) => ({
                    ...row,
                    inputNodeIds: row.inputNodeIds.map((id) =>
                      id && removed.has(id) ? null : id,
                    ),
                  })),
                },
              }
            : {}),
          ...(node.director
            ? {
                director: {
                  ...node.director,
                  objects: node.director.objects.map((object) => {
                    if (
                      !object.sourceNodeId ||
                      !removed.has(object.sourceNodeId)
                    )
                      return object;
                    const { sourceNodeId, ...retained } = object;
                    return {
                      ...retained,
                      assetId:
                        object.assetId ??
                        document.nodes.find(
                          (value) => value.id === sourceNodeId,
                        )?.assetId,
                    };
                  }),
                },
              }
            : {}),
          ...(node.timeline
            ? {
                timeline: {
                  ...node.timeline,
                  clips: node.timeline.clips.map((clip) =>
                    clip.nodeId && removed.has(clip.nodeId)
                      ? {
                          ...clip,
                          nodeId: null,
                          assetId:
                            clip.assetId ??
                            document.nodes.find(
                              (value) => value.id === clip.nodeId,
                            )?.assetId ??
                            null,
                        }
                      : clip,
                  ),
                },
              }
            : {}),
        }));
        next.connections = next.connections.filter(
          (edge) =>
            !removed.has(edge.fromNodeId) && !removed.has(edge.toNodeId),
        );
        break;
      }
      case "MoveNodes":
        command.moves.forEach((move) => {
          validPosition(move.x);
          validPosition(move.y);
          patch(move.id, { position: { x: move.x, y: move.y } });
        });
        break;
      case "ResizeNodes":
        command.sizes.forEach((size) => {
          const node = {
            ...requireNode(size.id),
            width: size.width,
            height: size.height,
          };
          validateNode(node);
          patch(node.id, node);
        });
        break;
      case "RenameNodes":
        command.names.forEach((name) => {
          const node = { ...requireNode(name.id), title: name.title };
          validateNode(node);
          patch(node.id, node);
        });
        break;
      case "SetNodeParents":
        command.parents.forEach((parent) => {
          const child = requireNode(parent.id);
          if (parent.parent_id) {
            const group = requireNode(parent.parent_id);
            if (group.type !== CanvasNodeType.Frame || group.id === child.id)
              throw new Error("分组关系无效。");
          }
          patch(child.id, { parentId: parent.parent_id ?? undefined });
        });
        break;
      case "SetNodeZIndex":
        command.z_indices.forEach((item) => {
          if (
            !Number.isSafeInteger(item.z_index) ||
            Math.abs(item.z_index) > 1e6
          )
            throw new Error("层次无效。");
          patch(item.id, { zIndex: item.z_index });
        });
        break;
      case "UpdateNodeConfig": {
        const node = requireNode(command.id);
        if (requireNode(command.id).type === CanvasNodeType.Generation) {
          patch(command.id, {
            generation: decodeGenerationConfig(command.config.generation),
          });
          break;
        }
        if (node.type === CanvasNodeType.Director && command.config.director) {
          patch(node.id, { director: decodeDirector(command.config.director) });
          break;
        }
        if (
          node.type === CanvasNodeType.BatchTable &&
          command.config.batch_table
        ) {
          patch(node.id, {
            batchTable: decodeBatchConfig(command.config.batch_table),
          });
          break;
        }
        if (node.type === CanvasNodeType.Timeline && command.config.timeline) {
          patch(node.id, { timeline: decodeTimeline(command.config.timeline) });
          break;
        }
        if (
          node.type === CanvasNodeType.Text &&
          typeof command.config.text === "string"
        ) {
          const updated = {
            ...node,
            metadata: { ...node.metadata, content: command.config.text },
          };
          validateNode(updated);
          patch(node.id, updated);
        } else if (
          node.type === CanvasNodeType.Frame &&
          typeof command.config.collapsed === "boolean"
        )
          patch(node.id, {
            metadata: {
              frame: {
                collapsed: command.config.collapsed,
                expandedWidth: node.width,
                expandedHeight: node.height,
              },
            },
          });
        else throw new Error("配置与节点类型不匹配。");
        break;
      }
      case "Connect":
        command.edges.forEach((edge) => {
          requireNode(edge.fromNodeId);
          requireNode(edge.toNodeId);
          if (
            edge.fromNodeId === edge.toNodeId ||
            next.connections.some((item) => item.id === edge.id)
          )
            throw new Error("连接无效。");
          next.connections.push(edge);
        });
        break;
      case "Disconnect": {
        command.ids.forEach((id) => {
          if (!next.connections.some((edge) => edge.id === id))
            throw new Error("连接已不存在。");
        });
        const removed = new Set(command.ids);
        next.connections = next.connections.filter(
          (edge) => !removed.has(edge.id),
        );
        break;
      }
      case "SetViewport":
        validPosition(command.viewport.x);
        validPosition(command.viewport.y);
        if (
          !Number.isFinite(command.viewport.k) ||
          command.viewport.k < 0.05 ||
          command.viewport.k > 4
        )
          throw new Error("缩放超出允许范围。");
        next = { ...next, viewport: command.viewport };
        break;
    }
    const byId = new Map(next.nodes.map((node) => [node.id, node]));
    for (const node of next.nodes) {
      if (node.batchTable)
        for (const row of node.batchTable.rows)
          for (const id of row.inputNodeIds) {
            if (id && byId.get(id)?.type !== CanvasNodeType.Image)
              throw new Error("批量参考必须是本画布图片。");
          }
      if (node.director)
        for (const object of node.director.objects) {
          if (!object.sourceNodeId) continue;
          const source = byId.get(object.sourceNodeId);
          const kind =
            object.kind === "billboard"
              ? CanvasNodeType.Image
              : CanvasNodeType.Model;
          if (
            !source ||
            source.type !== kind ||
            (object.assetId && object.assetId !== source.assetId)
          )
            throw new Error("导演台参考必须与本画布素材类型和资产匹配。");
        }
      if (node.timeline)
        for (const clip of node.timeline.clips) {
          if (clip.nodeId && byId.get(clip.nodeId)?.type !== clip.kind)
            throw new Error("时间线参考必须与本画布素材类型匹配。");
        }
      const seen = new Set([node.id]);
      let parentId = node.parentId;
      while (parentId) {
        if (seen.has(parentId)) throw new Error("分组关系不能形成循环。");
        seen.add(parentId);
        const parent = byId.get(parentId);
        if (!parent || parent.type !== CanvasNodeType.Frame)
          throw new Error("分组关系无效。");
        parentId = parent.parentId;
      }
    }
    if (next.nodes.length > 2000) throw new Error("最多 2,000 个节点。");
    if (next.connections.length > 4000) throw new Error("最多 4,000 个连接。");
  }
  return next;
}
function chunks<T>(items: T[]): T[][] {
  const result: T[][] = [];
  for (let index = 0; index < items.length; index += 2000)
    result.push(items.slice(index, index + 2000));
  return result;
}
/** 同一原子请求内分批，不将一个用户操作拆成多次 HTTP 保存。 */
export function normalizeCommands(commands: CanvasCommand[]): CanvasCommand[] {
  return commands.flatMap((command): CanvasCommand[] => {
    switch (command.type) {
      case "AddNodes":
        return chunks(command.nodes).map((nodes) => ({ ...command, nodes }));
      case "MoveNodes":
        return chunks(command.moves).map((moves) => ({ ...command, moves }));
      case "ResizeNodes":
        return chunks(command.sizes).map((sizes) => ({ ...command, sizes }));
      case "RenameNodes":
        return chunks(command.names).map((names) => ({ ...command, names }));
      case "SetNodeParents":
        return chunks(command.parents).map((parents) => ({
          ...command,
          parents,
        }));
      case "SetNodeZIndex":
        return chunks(command.z_indices).map((z_indices) => ({
          ...command,
          z_indices,
        }));
      case "Connect":
        return chunks(command.edges).map((edges) => ({ ...command, edges }));
      case "Disconnect":
      case "DeleteNodes":
        return chunks(command.ids).map((ids) => ({ ...command, ids }));
      default:
        return [command];
    }
  });
}
export function diffNodes(
  before: CanvasNodeData[],
  after: CanvasNodeData[],
): CanvasCommand[] {
  const old = new Map(before.map((node) => [node.id, node]));
  const nextIds = new Set(after.map((node) => node.id));
  const commands: CanvasCommand[] = [];
  const removed = before
    .filter((node) => !nextIds.has(node.id))
    .map((node) => node.id);
  const added = after.filter((node) => !old.has(node.id));
  if (removed.length) commands.push({ type: "DeleteNodes", ids: removed });
  if (added.length) commands.push({ type: "AddNodes", nodes: added });
  const moves: Extract<CanvasCommand, { type: "MoveNodes" }>["moves"] = [],
    sizes: Extract<CanvasCommand, { type: "ResizeNodes" }>["sizes"] = [],
    names: Extract<CanvasCommand, { type: "RenameNodes" }>["names"] = [],
    parents: Extract<CanvasCommand, { type: "SetNodeParents" }>["parents"] = [],
    z_indices: Extract<CanvasCommand, { type: "SetNodeZIndex" }>["z_indices"] =
      [];
  for (const node of after) {
    const previous = old.get(node.id);
    if (!previous) continue;
    if (
      previous.position.x !== node.position.x ||
      previous.position.y !== node.position.y
    )
      moves.push({ id: node.id, ...node.position });
    if (previous.width !== node.width || previous.height !== node.height)
      sizes.push({ id: node.id, width: node.width, height: node.height });
    if (previous.title !== node.title)
      names.push({ id: node.id, title: node.title });
    if (previous.parentId !== node.parentId)
      parents.push({ id: node.id, parent_id: node.parentId ?? null });
    if (previous.zIndex !== node.zIndex)
      z_indices.push({ id: node.id, z_index: node.zIndex });
    if (
      JSON.stringify(nodeConfig(previous)) !== JSON.stringify(nodeConfig(node))
    )
      commands.push({
        type: "UpdateNodeConfig",
        id: node.id,
        config: nodeConfig(node),
      });
  }
  if (moves.length) commands.push({ type: "MoveNodes", moves });
  if (sizes.length) commands.push({ type: "ResizeNodes", sizes });
  if (names.length) commands.push({ type: "RenameNodes", names });
  if (parents.length) commands.push({ type: "SetNodeParents", parents });
  if (z_indices.length) commands.push({ type: "SetNodeZIndex", z_indices });
  return commands;
}
export function inverseCommands(
  before: CanvasDocument,
  commands: CanvasCommand[],
): CanvasCommand[] {
  const after = applyCommands(before, commands);
  const restored = applyCanvasHistoryPatch(
    after,
    createCanvasHistoryPatch(before, after),
    "before",
  );
  const inverse = diffNodes(after.nodes, restored.nodes);
  const beforeIds = new Set(before.connections.map((edge) => edge.id)),
    afterIds = new Set(after.connections.map((edge) => edge.id));
  const removedEdges = after.connections
    .filter((edge) => !beforeIds.has(edge.id))
    .map((edge) => edge.id);
  const restoredEdges = before.connections.filter(
    (edge) => !afterIds.has(edge.id),
  );
  if (removedEdges.length)
    inverse.unshift({ type: "Disconnect", ids: removedEdges });
  if (restoredEdges.length)
    inverse.push({ type: "Connect", edges: restoredEdges });
  if (JSON.stringify(after.viewport) !== JSON.stringify(before.viewport))
    inverse.push({ type: "SetViewport", viewport: before.viewport });
  const normalized = normalizeCommands(inverse);
  if (normalized.length > 100)
    throw new Error("此操作需要超过 100 条撤销命令，请减少一次操作的内容。");
  if (normalized.length) applyCommands(after, normalized);
  return normalized;
}
/** 保存前验证正向和逆向预算，防止已保存的操作无法原子撤销。 */
export function prepareEdit(before: CanvasDocument, commands: CanvasCommand[]) {
  const forward = normalizeCommands(commands);
  return { forward, inverse: inverseCommands(before, forward) };
}
