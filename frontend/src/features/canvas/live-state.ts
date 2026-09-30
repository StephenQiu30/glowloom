// 编辑器边界模型。公共 REST DTO 由在线 Swagger 生成，在 queries 中显式转换。
export type CanvasViewport = { x: number; y: number; zoom: number };
export type CanvasNode = {
  id: string;
  node_type: "text";
  node_action: "resource";
  config: { text: string };
  x: number;
  y: number;
  width?: number;
  height?: number;
};
export type CanvasEdge = {
  id: string;
  edge_type: "annotation";
  source_node_id: string;
  target_node_id: string;
};
export type CanvasDocument = {
  id: string;
  project_id: string;
  name: string;
  scope: Record<string, unknown>;
  revision: number;
  viewport: CanvasViewport;
  nodes: CanvasNode[];
  edges: CanvasEdge[];
};
export type CanvasCommand =
  | { type: "AddNodes"; nodes: CanvasNode[] }
  | { type: "MoveNodes"; moves: { id: string; x: number; y: number }[] }
  | { type: "UpdateNodeConfig"; id: string; config: { text: string } }
  | { type: "DeleteNodes"; ids: string[] }
  | { type: "Connect"; edge: CanvasEdge }
  | { type: "Disconnect"; id: string }
  | { type: "SetViewport"; viewport: CanvasViewport };

function position(value: number) {
  if (!Number.isFinite(value) || Math.abs(value) > 1_000_000) {
    throw new Error("坐标必须是有效数值，且在允许范围内。");
  }
}
function nodeExists(document: CanvasDocument, id: string) {
  const node = document.nodes.find((item) => item.id === id);
  if (!node) throw new Error("备注已不存在，请重新读取画布。");
  return node;
}
function validateNode(node: CanvasNode) {
  position(node.x);
  position(node.y);
  if (
    node.node_type !== "text" ||
    node.node_action !== "resource" ||
    Array.from(node.config.text).length > 10_000
  )
    throw new Error("本轮只支持不超过 10,000 个字符的纯文字备注。");
}
export function applyCommands(
  document: CanvasDocument,
  commands: CanvasCommand[],
): CanvasDocument {
  let next = document;
  for (const command of commands) {
    switch (command.type) {
      case "AddNodes": {
        const ids = new Set(next.nodes.map((node) => node.id));
        for (const node of command.nodes) {
          validateNode(node);
          if (ids.has(node.id)) throw new Error("备注 ID 已存在。");
          ids.add(node.id);
        }
        next = { ...next, nodes: [...next.nodes, ...command.nodes] };
        break;
      }
      case "MoveNodes": {
        const moves = new Map(
          command.moves.map((move) => {
            nodeExists(next, move.id);
            position(move.x);
            position(move.y);
            return [move.id, move];
          }),
        );
        next = {
          ...next,
          nodes: next.nodes.map((node) =>
            moves.has(node.id) ? { ...node, ...moves.get(node.id) } : node,
          ),
        };
        break;
      }
      case "UpdateNodeConfig": {
        const node = nodeExists(next, command.id);
        validateNode({ ...node, config: command.config });
        next = {
          ...next,
          nodes: next.nodes.map((item) =>
            item.id === command.id
              ? { ...item, config: { ...command.config } }
              : item,
          ),
        };
        break;
      }
      case "DeleteNodes": {
        command.ids.forEach((id) => nodeExists(next, id));
        const ids = new Set(command.ids);
        next = {
          ...next,
          nodes: next.nodes.filter((node) => !ids.has(node.id)),
          edges: next.edges.filter(
            (edge) =>
              !ids.has(edge.source_node_id) && !ids.has(edge.target_node_id),
          ),
        };
        break;
      }
      case "Connect": {
        nodeExists(next, command.edge.source_node_id);
        nodeExists(next, command.edge.target_node_id);
        if (
          command.edge.edge_type !== "annotation" ||
          command.edge.source_node_id === command.edge.target_node_id ||
          next.edges.some((edge) => edge.id === command.edge.id)
        )
          throw new Error("注释线需要两个不同的备注与唯一 ID。");
        next = { ...next, edges: [...next.edges, command.edge] };
        break;
      }
      case "Disconnect": {
        if (!next.edges.some((edge) => edge.id === command.id))
          throw new Error("注释线已不存在。");
        next = {
          ...next,
          edges: next.edges.filter((edge) => edge.id !== command.id),
        };
        break;
      }
      case "SetViewport": {
        position(command.viewport.x);
        position(command.viewport.y);
        if (
          !Number.isFinite(command.viewport.zoom) ||
          command.viewport.zoom < 0.1 ||
          command.viewport.zoom > 4
        )
          throw new Error("缩放比例应在 0.1～4 之间。");
        next = { ...next, viewport: { ...command.viewport } };
        break;
      }
    }
  }
  return next;
}

export function inverseCommands(
  document: CanvasDocument,
  commands: CanvasCommand[],
): CanvasCommand[] {
  let current = document;
  let inverses: CanvasCommand[] = [];
  for (const command of commands) {
    let inverse: CanvasCommand[];
    switch (command.type) {
      case "AddNodes":
        inverse = [
          { type: "DeleteNodes", ids: command.nodes.map((node) => node.id) },
        ];
        break;
      case "MoveNodes":
        inverse = [
          {
            type: "MoveNodes",
            moves: command.moves.map(({ id }) => {
              const node = nodeExists(current, id);
              return { id, x: node.x, y: node.y };
            }),
          },
        ];
        break;
      case "UpdateNodeConfig":
        inverse = [
          {
            type: "UpdateNodeConfig",
            id: command.id,
            config: { ...nodeExists(current, command.id).config },
          },
        ];
        break;
      case "DeleteNodes": {
        const ids = new Set(command.ids);
        inverse = [
          {
            type: "AddNodes",
            nodes: command.ids.map((id) => nodeExists(current, id)),
          },
          ...current.edges
            .filter(
              (edge) =>
                ids.has(edge.source_node_id) || ids.has(edge.target_node_id),
            )
            .map((edge): CanvasCommand => ({ type: "Connect", edge })),
        ];
        break;
      }
      case "Connect":
        inverse = [{ type: "Disconnect", id: command.edge.id }];
        break;
      case "Disconnect": {
        const edge = current.edges.find((item) => item.id === command.id);
        if (!edge) throw new Error("注释线已不存在。");
        inverse = [{ type: "Connect", edge }];
        break;
      }
      case "SetViewport":
        inverse = [{ type: "SetViewport", viewport: { ...current.viewport } }];
        break;
    }
    current = applyCommands(current, [command]);
    inverses = [...inverse, ...inverses];
  }
  return inverses;
}

export type HistoryEntry = {
  forward: CanvasCommand[];
  inverse: CanvasCommand[];
};
export type CanvasEditor = {
  document: CanvasDocument;
  past: HistoryEntry[];
  future: HistoryEntry[];
};
export function createEditor(document: CanvasDocument): CanvasEditor {
  return { document, past: [], future: [] };
}
// 只有收到服务端确认后才调用这些历史操作；未确认草稿由 UI 单独保留。
export function edit(
  editor: CanvasEditor,
  commands: CanvasCommand[],
  saved = applyCommands(editor.document, commands),
): CanvasEditor {
  return {
    document: saved,
    past: [
      ...editor.past.slice(-49),
      {
        forward: commands,
        inverse: inverseCommands(editor.document, commands),
      },
    ],
    future: [],
  };
}
export function undo(
  editor: CanvasEditor,
  saved?: CanvasDocument,
): CanvasEditor {
  const entry = editor.past.at(-1);
  if (!entry) return editor;
  return {
    document: saved ?? applyCommands(editor.document, entry.inverse),
    past: editor.past.slice(0, -1),
    future: [...editor.future, entry],
  };
}
export function redo(
  editor: CanvasEditor,
  saved?: CanvasDocument,
): CanvasEditor {
  const entry = editor.future.at(-1);
  if (!entry) return editor;
  return {
    document: saved ?? applyCommands(editor.document, entry.forward),
    past: [...editor.past, entry],
    future: editor.future.slice(0, -1),
  };
}
export function resetEditor(
  _editor: CanvasEditor,
  latest: CanvasDocument,
): CanvasEditor {
  return createEditor(latest);
}
