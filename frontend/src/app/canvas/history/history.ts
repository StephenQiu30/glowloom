// Operation boundaries inspired by BeefTV ddaf5f9 use-canvas-history.ts (MIT).
// Immutable graph references only; persistence and Tiptap history stay with their owners.
import type { LabelState } from "@/components/generation/prompt-document";
import { canonical } from "@/components/generation/draft-save";
import { commandBatch, isDraft, type Canvas, type Node } from "../document";
export type EditableGraph = { nodes: Node[]; edges: API.Edge[] };
type Entry = {
  before: EditableGraph;
  after: EditableGraph;
  key?: string;
  at: number;
};
function mergeLabels(a: LabelState, b: LabelState): LabelState {
  return {
    highWater: {
      image: Math.max(a.highWater.image, b.highWater.image),
      video: Math.max(a.highWater.video, b.highWater.video),
      audio: Math.max(a.highWater.audio, b.highWater.audio),
      subject: Math.max(a.highWater.subject, b.highWater.subject),
    },
    allocations: { ...a.allocations, ...b.allocations },
  };
}
export function createHistory() {
  const past: Entry[] = [],
    future: Entry[] = [];
  // Allocation tombstones outlive graph snapshots, including edits not yet saved.
  const labels = new Map<string, LabelState>();
  const remember = (graph: EditableGraph) => {
    for (const node of graph.nodes) {
      if (!isDraft(node) || !node.config.draft.composer) continue;
      const state = node.config.draft.composer.labelState;
      const known = labels.get(node.id);
      labels.set(node.id, known ? mergeLabels(state, known) : state);
    }
  };
  const prepare = (graph: EditableGraph, base: Canvas): EditableGraph => ({
    ...graph,
    nodes: graph.nodes.map((node) => {
      if (!isDraft(node)) return node;
      const saved = base.nodes.find((n) => n.id === node.id);
      const composer = node.config.draft.composer;
      let state = composer?.labelState;
      const remembered = labels.get(node.id);
      const persisted =
        saved && isDraft(saved)
          ? saved.config.draft.composer?.labelState
          : undefined;
      if (state && remembered) state = mergeLabels(state, remembered);
      if (state && persisted) state = mergeLabels(state, persisted);
      return {
        ...node,
        ...(saved ? { input_revision: saved.input_revision } : {}),
        ...(composer && state
          ? {
              config: {
                draft: {
                  ...node.config.draft,
                  composer: { ...composer, labelState: state },
                },
              },
            }
          : {}),
      };
    }),
  });
  const legal = (
    entry: Entry | undefined,
    direction: "before" | "after",
    base: Canvas,
  ) => {
    if (!entry) return false;
    try {
      const graph = prepare(entry[direction], base);
      if (
        graph.nodes.some(
          (node) =>
            isDraft(node) && labels.has(node.id) && !node.config.draft.composer,
        )
      )
        return false;
      commandBatch(base, graph.nodes, { edges: graph.edges });
      return true;
    } catch {
      return false;
    }
  };
  return {
    record(
      before: EditableGraph,
      after: EditableGraph,
      key?: string,
      at = Date.now(),
    ) {
      remember(before);
      remember(after);
      if (canonical(before) === canonical(after)) return;
      const last = past.at(-1);
      if (
        key &&
        last?.key === key &&
        at - last.at < 500 &&
        canonical(last.after) === canonical(before)
      ) {
        last.after = after;
        last.at = at;
      } else {
        past.push({ before, after, key, at });
        if (past.length > 50) past.shift();
      }
      future.length = 0;
    },
    canUndo(base: Canvas) {
      return legal(past.at(-1), "before", base);
    },
    canRedo(base: Canvas) {
      return legal(future.at(-1), "after", base);
    },
    undo(base: Canvas) {
      if (!legal(past.at(-1), "before", base)) return;
      const entry = past.pop()!;
      future.push(entry);
      return prepare(entry.before, base);
    },
    redo(base: Canvas) {
      if (!legal(future.at(-1), "after", base)) return;
      const entry = future.pop()!;
      past.push(entry);
      return prepare(entry.after, base);
    },
    reset() {
      past.length = 0;
      future.length = 0;
      labels.clear();
    },
  };
}
