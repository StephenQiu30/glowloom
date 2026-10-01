import { createStore } from "zustand/vanilla";
import { immer } from "zustand/middleware/immer";
import { applyCommands, inverseCommands } from "./document";
import {
  createCanvasHistoryPatch,
  type CanvasHistoryPatch,
} from "./engine/history";
import type { CanvasCommand, CanvasDocument } from "./model";
type Entry = {
  forward: CanvasCommand[];
  inverse: CanvasCommand[];
  patch: CanvasHistoryPatch;
};
export type CanvasState = {
  document: CanvasDocument;
  selectedIds: string[];
  selectedEdge?: string;
  draft?: { id: string; text: string };
  past: Entry[];
  future: Entry[];
  select: (ids: Set<string>) => void;
  selectEdge: (id?: string) => void;
  editText: (id: string, text: string) => void;
  discardText: () => void;
  confirm: (
    saved: CanvasDocument,
    commands: CanvasCommand[],
    kind: "edit" | "undo" | "redo",
  ) => void;
  reset: (latest: CanvasDocument) => void;
};
// 每个编辑器实例独立，无 persist、localStorage 或跨用户全局状态。
export function createCanvasStore(document: CanvasDocument) {
  return createStore<CanvasState>()(
    immer((set) => ({
      document,
      selectedIds: [],
      past: [],
      future: [],
      select: (ids) =>
        set((state) => {
          state.selectedIds = [...ids];
          state.selectedEdge = undefined;
        }),
      selectEdge: (id) =>
        set((state) => {
          state.selectedIds = [];
          state.selectedEdge = id;
        }),
      editText: (id, text) =>
        set((state) => {
          state.draft = { id, text };
        }),
      discardText: () =>
        set((state) => {
          state.draft = undefined;
        }),
      confirm: (saved, commands, kind) =>
        set((state) => {
          if (kind === "undo") {
            const entry = state.past.pop();
            if (entry) state.future.push(entry);
          } else if (kind === "redo") {
            const entry = state.future.pop();
            if (entry) state.past.push(entry);
          } else {
            const before = state.document as CanvasDocument;
            state.past.push({
              forward: commands,
              inverse: inverseCommands(before, commands),
              patch: createCanvasHistoryPatch(
                before,
                applyCommands(before, commands),
              ),
            });
            state.past = state.past.slice(-50);
            state.future = [];
          }
          state.document = saved;
          state.selectedIds = state.selectedIds.filter((id) =>
            saved.nodes.some((node) => node.id === id),
          );
          if (
            state.selectedEdge &&
            !saved.connections.some((edge) => edge.id === state.selectedEdge)
          )
            state.selectedEdge = undefined;
          if (
            kind !== "edit" ||
            commands.some(
              (command) =>
                command.type === "UpdateNodeConfig" &&
                command.id === state.draft?.id,
            ) ||
            (state.draft &&
              !saved.nodes.some((node) => node.id === state.draft?.id))
          )
            state.draft = undefined;
        }),
      reset: (latest) =>
        set((state) => {
          state.document = latest;
          state.past = [];
          state.future = [];
          state.selectedIds = [];
          state.selectedEdge = undefined;
          state.draft = undefined;
        }),
    })),
  );
}
