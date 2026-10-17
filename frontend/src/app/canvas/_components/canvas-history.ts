// Operation boundaries inspired by BeefTV ddaf5f9 use-canvas-history.ts (MIT).
// Immutable graph references only; persistence and Tiptap history stay with their owners.
import { canonical } from "@/components/generation/draft-save";
import { commandBatch, isDraft, type Canvas, type Node } from "./editor-state";
export type EditableGraph = { nodes: Node[]; edges: API.Edge[] };
type Entry = {
  before: EditableGraph;
  after: EditableGraph;
  key?: string;
  at: number;
};
export function createHistory() {
  const past: Entry[] = [],
    future: Entry[] = [];
  const prepare = (graph: EditableGraph, base: Canvas): EditableGraph => ({
    ...graph,
    nodes: graph.nodes.map((node) => {
      const saved = base.nodes.find((n) => n.id === node.id);
      if (!saved || !isDraft(node) || !isDraft(saved)) return node;
      const composer = node.config.draft.composer,
        known = saved.config.draft.composer;
      return {
        ...node,
        input_revision: saved.input_revision,
        ...(composer && known
          ? {
              config: {
                draft: {
                  ...node.config.draft,
                  composer: {
                    ...composer,
                    labelState: {
                      ...composer.labelState,
                      highWater: {
                        image: Math.max(
                          composer.labelState.highWater.image,
                          known.labelState.highWater.image,
                        ),
                        video: Math.max(
                          composer.labelState.highWater.video,
                          known.labelState.highWater.video,
                        ),
                        audio: Math.max(
                          composer.labelState.highWater.audio,
                          known.labelState.highWater.audio,
                        ),
                        subject: Math.max(
                          composer.labelState.highWater.subject,
                          known.labelState.highWater.subject,
                        ),
                      },
                      allocations: {
                        ...composer.labelState.allocations,
                        ...known.labelState.allocations,
                      },
                    },
                  },
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
    },
  };
}
