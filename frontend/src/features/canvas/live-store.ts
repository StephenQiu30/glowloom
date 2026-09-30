import { createStore } from "zustand/vanilla";
import { immer } from "zustand/middleware/immer";
import type { Node } from "@xyflow/react";
import {
  createEditor,
  type CanvasDocument,
  type CanvasEditor,
} from "./live-state";

export type NoteNode = Node<{ text: string }, "note">;
export function flowNodes(
  document: CanvasDocument,
  selectedId?: string,
): NoteNode[] {
  return document.nodes.map((node) => ({
    id: node.id,
    type: "note",
    position: { x: node.x, y: node.y },
    data: { text: node.config.text },
    width: node.width ?? 240,
    height: node.height ?? 160,
    selected: node.id === selectedId,
  }));
}
type LiveState = {
  editor: CanvasEditor;
  nodes: NoteNode[];
  selectedId?: string;
  selectedEdge?: string;
  text: string;
  setNodes: (nodes: NoteNode[]) => void;
  setText: (text: string) => void;
  selectNote: (id: string) => void;
  selectEdge: (id: string) => void;
  calibrate: (
    editor: CanvasEditor,
    resetSelection?: boolean,
    syncText?: boolean,
  ) => void;
};
// 每个挂载的画布独立创建；不存浏览器存储，不跨账号共享。
export function createCanvasStore(document: CanvasDocument) {
  return createStore<LiveState>()(
    immer((set) => ({
      editor: createEditor(document),
      nodes: flowNodes(document),
      text: "",
      setNodes: (nodes) =>
        set((state) => {
          state.nodes = nodes;
        }),
      setText: (text) =>
        set((state) => {
          state.text = text;
        }),
      selectNote: (id) =>
        set((state) => {
          state.selectedId = id;
          state.selectedEdge = undefined;
          state.text =
            state.editor.document.nodes.find((node) => node.id === id)?.config
              .text ?? "";
          state.nodes = flowNodes(state.editor.document, id);
        }),
      selectEdge: (id) =>
        set((state) => {
          state.selectedEdge = id;
          state.selectedId = undefined;
        }),
      calibrate: (editor, resetSelection = false, syncText = false) =>
        set((state) => {
          state.editor = editor;
          if (
            resetSelection ||
            !editor.document.nodes.some((node) => node.id === state.selectedId)
          ) {
            state.selectedId = undefined;
            state.selectedEdge = undefined;
            state.text = "";
          }
          state.nodes = flowNodes(editor.document, state.selectedId);
          if (syncText && state.selectedId)
            state.text =
              editor.document.nodes.find((node) => node.id === state.selectedId)
                ?.config.text ?? "";
        }),
    })),
  );
}
