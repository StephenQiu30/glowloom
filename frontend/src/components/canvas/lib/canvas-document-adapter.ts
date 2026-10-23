import type { RichTextDocument } from "@/components/canvas/runtime/components/editor/types/rich-text";
export { canvasGraphFingerprint } from "./canvas-json";
import { reconcileCanvasReferences } from "./canvas-reference-state";
import {
  documentText,
  textToDocument,
  parseRichTextDocument,
} from "@/components/canvas/runtime/components/editor/lib/rich-text-document";
import type {
  CanvasConnection,
  CanvasNodeData,
  ViewportTransform,
} from "@/components/canvas/types/canvas";

export type CanvasEditorState = {
  nodes: CanvasNodeData[];
  connections: CanvasConnection[];
  backgroundMode: "dots" | "lines" | "blank";
  showImageInfo: boolean;
  viewport: ViewportTransform;
};

export function toCanvasContentGraph(
  state: CanvasEditorState,
  defaults: CanvasContract.CanvasDocumentGraphDTO,
): CanvasContract.CanvasDocumentGraphDTO {
  return {
    ...toCanvasGraph(state),
    viewport: defaults.viewport,
    backgroundMode: defaults.backgroundMode,
    showImageInfo: defaults.showImageInfo,
  };
}

export function fromCanvasGraph(
  graph: CanvasContract.CanvasDocumentGraphDTO,
): CanvasEditorState {
  if (
    graph.schemaVersion !== 2 ||
    !Array.isArray(graph.nodes) ||
    !Array.isArray(graph.connections)
  )
    throw new Error("画布格式不受支持，已禁止编辑和覆盖");
  const nodes = graph.nodes.map((node): CanvasNodeData => {
    if (node.nodeTypeVersion !== 1) throw new Error("节点格式不受支持");
    const document = node.textDocument
      ? parseRichTextDocument(node.textDocument)
      : undefined;
    return {
      id: node.id,
      type: node.type,
      title: node.title,
      position: { x: node.position.x, y: node.position.y },
      width: node.width,
      height: node.height,
      documentData: structuredClone(node),
      metadata: {
        ...(node.parentId ? { groupId: node.parentId } : {}),
        ...(document
          ? {
              richText: document,
              ...(node.type === "text"
                ? { content: documentText(document.content) }
                : { prompt: documentText(document.content) }),
            }
          : {}),
      },
    };
  });
  const connections = graph.connections.map((edge) => ({
    id: edge.id,
    fromNodeId: edge.fromNodeId,
    toNodeId: edge.toNodeId,
    documentData: structuredClone(edge),
  }));
  return {
    nodes: reconcileCanvasReferences(nodes, connections),
    connections,
    backgroundMode:
      graph.backgroundMode === "lines" || graph.backgroundMode === "blank"
        ? graph.backgroundMode
        : "dots",
    showImageInfo: graph.showImageInfo ?? false,
    viewport: {
      x: graph.viewport?.x ?? 0,
      y: graph.viewport?.y ?? 0,
      k: graph.viewport?.zoom ?? 1,
    },
  };
}

function textDocument(node: CanvasNodeData): RichTextDocument | undefined {
  if (["group", "config"].includes(node.type)) return undefined;
  const metadata = node.metadata;
  if (metadata?.richText) return parseRichTextDocument(metadata.richText);
  const text =
    node.type === "text"
      ? metadata?.content
      : (metadata?.prompt ?? metadata?.composerContent);
  return text ? { schemaVersion: 1, content: textToDocument(text) } : undefined;
}

// 临时预览只留在编辑器；没有固定版本的媒体必须显式处理，不能假称已保存。
export function toCanvasGraph(
  state: CanvasEditorState,
): CanvasContract.CanvasDocumentGraphDTO {
  const nodes = state.nodes.map(
    (node, index): CanvasContract.CanvasDocumentNodeDTO => {
      if (
        !["text", "image", "video", "audio", "config", "group"].includes(
          node.type,
        )
      )
        throw new Error("节点类型不支持保存");
      const source = node.documentData;
      const media = ["image", "video", "audio"].includes(node.type);
      const refs = source?.fileReferences ?? [];
      if (
        media &&
        node.metadata?.content &&
        !refs.length &&
        !node.metadata?.generationOutput
      )
        throw new Error("请先上传文件并选择已发布的固定版本，再保存媒体节点");
      const document = textDocument(node);
      return {
        id: node.id,
        type: node.type,
        nodeTypeVersion: 1,
        title: node.title,
        position: node.position,
        width: node.width,
        height: node.height,
        zIndex: source?.zIndex ?? index,
        ...(node.metadata?.groupId ? { parentId: node.metadata.groupId } : {}),
        ...(node.type === "group"
          ? { collapsed: source?.collapsed ?? false }
          : {
              parameters:
                source?.parameters ??
                (node.type === "config"
                  ? { values: {} }
                  : {
                      mode: ["image", "video"].includes(node.type)
                        ? "GENERATE"
                        : "MANUAL",
                    }),
            }),
        ...(document ? { textDocument: document } : {}),
        ...(media ? { fileReferences: refs } : {}),
      };
    },
  );
  const connections = state.connections.map((edge) => {
    if (!edge.documentData)
      throw new Error("请为连线明确选择输入端口和内容绑定，再保存");
    return {
      ...edge.documentData,
      id: edge.id,
      fromNodeId: edge.fromNodeId,
      toNodeId: edge.toNodeId,
    };
  });
  return {
    schemaVersion: 2,
    nodes,
    connections,
    backgroundMode: state.backgroundMode,
    showImageInfo: state.showImageInfo,
    viewport: {
      x: state.viewport.x,
      y: state.viewport.y,
      zoom: state.viewport.k,
    },
  };
}
