import { toCanvasGraph } from "./canvas-document-adapter";
import { canvasConnectionKey as connectionKey } from "./canvas-connection-key";
import type { CanvasEditorState } from "./canvas-document-adapter";
import type { CanvasNodeData, ConnectionHandle } from "../types/canvas";
import { reconcileCanvasReferences } from "./canvas-reference-state";
import { isFixedCanvasMedia } from "./canvas-media-source";
import {
  canvasInputPort,
  canvasPortsFor,
} from "../constants/canvas-input-ports";

export function canvasConnectionDirection(
  handle: ConnectionHandle,
  otherId: string,
) {
  return handle.handleType === "target"
    ? { fromNodeId: otherId, toNodeId: handle.nodeId }
    : { fromNodeId: handle.nodeId, toNodeId: otherId };
}

/** Build the whole selection before the host accepts any graph change. */
export function connectCanvasDocumentSources(
  state: CanvasEditorState,
  sourceIDs: string[],
  targetID: string,
): CanvasEditorState {
  if (!sourceIDs.length) throw new Error("请选择来源节点");
  if (new Set(sourceIDs).size !== sourceIDs.length)
    throw new Error("来源节点重复，请调整选择");
  return sourceIDs.reduce((next, sourceID) => {
    // A batch is one relation-creation gesture: an existing relation rejects
    // the entire selection, even when another source could add a fresh edge.
    if (
      next.connections.some(
        (edge) => edge.fromNodeId === sourceID && edge.toNodeId === targetID,
      )
    )
      throw new Error("这项内容已经关联，请勿重复连接");
    return connectCanvasDocument(next, sourceID, targetID);
  }, state);
}

/** 草稿引用能力来自图协议，不由当前模型或执行通道缩窄。 */
export function supportsCanvasConnection(source: string, target: string) {
  return canvasPortsFor(source, target).length > 0;
}

export function validateCanvasConnection(
  edge: CanvasContract.CanvasDocumentConnectionDTO,
  connections: CanvasContract.CanvasDocumentConnectionDTO[],
) {
  if (edge.fromNodeId === edge.toNodeId) throw new Error("不能连接节点自身");
  if (connections.some((item) => connectionKey(item) === connectionKey(edge)))
    throw new Error("这项内容已经关联，请勿重复连接");
  const samePort = connections.filter(
    (item) =>
      item.toNodeId === edge.toNodeId && item.toPortId === edge.toPortId,
  );
  if (!canvasInputPort(edge.toPortId)?.multiple && samePort.length)
    throw new Error("这个输入已有连接，请先移除原连接");
  if (connections.length >= 1000 || edge.order > 999)
    throw new Error("画布连线数量已达上限，请先移除不需要的连接");
  const downstream = new Map<string, string[]>();
  for (const item of connections) {
    const children = downstream.get(item.fromNodeId) ?? [];
    children.push(item.toNodeId);
    downstream.set(item.fromNodeId, children);
  }
  const visited = new Set<string>();
  const pending = [edge.toNodeId];
  while (pending.length) {
    const id = pending.pop()!;
    if (id === edge.fromNodeId)
      throw new Error("这条连接会形成循环，请调整连接方向");
    if (visited.has(id)) continue;
    visited.add(id);
    pending.push(...(downstream.get(id) ?? []));
  }
}

/** 只生成显式图编辑；不读取模型、不调用生成或保存接口。 */
export function connectCanvasDocument(
  state: CanvasEditorState,
  fromNodeId: string,
  toNodeId: string,
): CanvasEditorState {
  const legacy = state.connections.filter(
    (edge) => !edge.documentData && edge.serviceData,
  );
  const graph = toCanvasGraph({
    ...state,
    connections: state.connections.filter(
      (edge) => edge.documentData || !edge.serviceData,
    ),
  });
  if (graph.nodes.length > 500)
    throw new Error("画布节点数量已达上限，请先移除不需要的节点");
  const source = graph.nodes.find((node) => node.id === fromNodeId);
  const target = graph.nodes.find((node) => node.id === toNodeId);
  if (!source || !target) throw new Error("连接的节点已不存在");
  if (fromNodeId === toNodeId) throw new Error("不能连接节点自身");
  const input = canvasPortsFor(source.type, target.type)[0];
  if (!input) throw new Error("这两种节点之间没有可用的输入用途");
  if (target.parameters?.mode === "AGENT" && input.id !== "prompt")
    throw new Error("当前 Agent（Mock）支持连接文字资料");
  const { id: toPortId, purpose } = input;
  const refs =
    source.fileReferences?.filter((ref) => ref.purpose === "CONTENT") ?? [];
  const manualValue =
    source.type === "config" ||
    (source.type === "text" && source.parameters?.mode === "MANUAL");
  const bindings: CanvasContract.CanvasBindingDTO[] = manualValue
    ? [{ kind: "NODE_VALUE" }]
    : refs.length
      ? refs.map((ref) => ({
          kind: "FILE_REFERENCE",
          referenceKey: ref.referenceKey,
        }))
      : [
          {
            kind: "RUN_OUTPUT",
            selection: input.allOutputs
              ? { mode: "ALL" }
              : { mode: "ITEM", index: 1 },
          },
        ];
  const edges = [...graph.connections];
  for (const binding of bindings) {
    const edge: CanvasContract.CanvasDocumentConnectionDTO = {
      id: crypto.randomUUID(),
      fromNodeId,
      toNodeId,
      fromPortId: source.type === "config" ? "settings" : source.type,
      toPortId,
      purpose,
      binding,
      order:
        Math.max(
          -1,
          ...edges
            .filter(
              (item) =>
                item.toNodeId === toNodeId && item.toPortId === toPortId,
            )
            .map((item) => item.order),
        ) + 1,
    };
    if (edges.some((item) => connectionKey(item) === connectionKey(edge)))
      continue;
    validateCanvasConnection(edge, edges);
    edges.push(edge);
  }
  if (isFixedCanvasMedia(state.nodes.find((node) => node.id === toNodeId)!))
    throw new Error("固定素材不接收生成参考，请先移除素材");
  if (edges.length === graph.connections.length) return state;
  const generateIds = new Set([toNodeId]);
  if (!manualValue && !refs.length) generateIds.add(fromNodeId);
  const nodes = state.nodes.map((node, index) =>
    generateIds.has(node.id)
      ? {
          ...node,
          documentData: {
            ...graph.nodes[index],
            parameters: {
              ...graph.nodes[index].parameters,
              mode: ["PROCESS", "AGENT"].includes(
                graph.nodes[index].parameters?.mode ?? "",
              )
                ? graph.nodes[index].parameters?.mode
                : "GENERATE",
            },
          },
        }
      : node,
  );
  // 给已有下游的手工文字接入上游时，一并转换出边，避免产生不可保存的半张图。
  const connections = [
    ...legacy,
    ...edges.map((edge) => ({
      id: edge.id,
      fromNodeId: edge.fromNodeId,
      toNodeId: edge.toNodeId,
      documentData:
        generateIds.has(edge.fromNodeId) && edge.binding.kind === "NODE_VALUE"
          ? {
              ...edge,
              binding: {
                kind: "RUN_OUTPUT",
                selection: { mode: "ITEM", index: 1 },
              },
            }
          : edge,
    })),
  ];
  return {
    ...state,
    nodes: reconcileCanvasReferences(nodes, connections),
    connections,
  };
}

/** Update execution mode and bindings atomically when the node's source changes. */
export function updateCanvasDocumentNode(
  state: CanvasEditorState,
  updated: CanvasNodeData,
): CanvasEditorState {
  const previous = state.nodes.find((node) => node.id === updated.id);
  if (
    !["PROCESS", "AGENT"].includes(
      updated.documentData?.parameters?.mode ?? "",
    ) &&
    state.connections.some(
      (edge) =>
        edge.toNodeId === updated.id &&
        edge.documentData?.binding.selection?.field,
    )
  )
    throw new Error("请先将上游字段引用改为摘要或移除，再切换执行方式");
  if (
    (previous?.documentData?.parameters?.mode !==
      updated.documentData?.parameters?.mode ||
      previous?.documentData?.parameters?.agent?.profile !==
        updated.documentData?.parameters?.agent?.profile) &&
    state.connections.some(
      (edge) =>
        edge.fromNodeId === updated.id &&
        edge.documentData?.binding.selection?.field,
    )
  )
    throw new Error("请先将下游字段引用改为摘要或移除，再切换执行场景");
  const generatedText =
    updated.type === "text" &&
    ["GENERATE", "PROCESS", "AGENT"].includes(
      updated.documentData?.parameters?.mode ?? "",
    ) &&
    !["GENERATE", "PROCESS", "AGENT"].includes(
      previous?.documentData?.parameters?.mode ?? "",
    );
  const manualText =
    updated.type === "text" &&
    updated.documentData?.parameters?.mode === "MANUAL" &&
    previous?.documentData?.parameters?.mode !== "MANUAL";
  if (
    manualText &&
    state.connections.some((edge) => edge.toNodeId === updated.id)
  )
    throw new Error("请先移除输入连接，再切换为固定文字");
  let connections = generatedText
    ? state.connections.map((edge) =>
        edge.fromNodeId === updated.id &&
        edge.documentData?.binding.kind === "NODE_VALUE"
          ? {
              ...edge,
              documentData: {
                ...edge.documentData,
                binding: {
                  kind: "RUN_OUTPUT",
                  selection: { mode: "ITEM", index: 1 },
                },
              },
            }
          : edge,
      )
    : manualText
      ? state.connections.map((edge) =>
          edge.fromNodeId === updated.id &&
          edge.documentData?.binding.kind === "RUN_OUTPUT"
            ? {
                ...edge,
                documentData: {
                  ...edge.documentData,
                  binding: { kind: "NODE_VALUE" },
                },
              }
            : edge,
        )
      : state.connections;
  const fixed = isFixedCanvasMedia(updated);
  const removed = !!previous && isFixedCanvasMedia(previous) && !fixed;
  if (fixed || removed) {
    const references = updated.documentData?.fileReferences ?? [];
    const primary = references.find(
      (reference) => reference.purpose === "CONTENT",
    );
    const seen = new Set<string>();
    connections = connections.flatMap((edge) => {
      if (fixed && edge.toNodeId === updated.id) return [];
      if (edge.fromNodeId !== updated.id || !edge.documentData) return [edge];
      let data = edge.documentData;
      if (removed) {
        if (updated.type === "audio") return [];
        data = {
          ...data,
          binding: {
            kind: "RUN_OUTPUT",
            selection: { mode: "ITEM", index: 1 },
          },
        };
      } else if (data.binding.kind === "RUN_OUTPUT") {
        // A new video does not inherit the previous video's extracted last frame.
        if (data.fromPortId !== updated.type || !primary) return [];
        data = {
          ...data,
          binding: {
            kind: "FILE_REFERENCE",
            referenceKey: primary.referenceKey,
          },
        };
      } else if (
        data.binding.kind === "FILE_REFERENCE" &&
        !references.some(
          (reference) => reference.referenceKey === data.binding.referenceKey,
        )
      ) {
        return [];
      }
      const key = connectionKey(data);
      if (seen.has(key)) return [];
      seen.add(key);
      return [{ ...edge, documentData: data }];
    });
  }
  return {
    ...state,
    nodes: reconcileCanvasReferences(
      state.nodes.map((node) => (node.id === updated.id ? updated : node)),
      connections,
    ),
    connections,
  };
}
