import { expect, it } from "vitest";
import { createHistory } from "./canvas-history";
import { emptyPromptDraft } from "@/components/generation/prompt-document";
import {
  createDraftNode,
  richCanvasDraft,
  createResourceNode,
} from "./editor-state";
const node = createResourceNode("text", { x: 0, y: 0 });
const graph = { nodes: [node], edges: [] };
const base = { id: "canvas", project_id: "project", revision: 1, ...graph };
it("一次操作一条记录，连续同字段合并，撤销后新操作清空重做", () => {
  const h = createHistory();
  const moved = { ...graph, nodes: [{ ...node, x: 20 }] };
  const renamed = { ...graph, nodes: [{ ...node, x: 20, title: "新名字" }] };
  h.record(graph, moved, "move", 0);
  h.record(moved, renamed, "title", 100);
  h.record(
    renamed,
    { ...renamed, nodes: [{ ...renamed.nodes[0], title: "新名字2" }] },
    "title",
    200,
  );
  expect(h.undo(base)?.nodes[0]).toMatchObject({ x: 20, title: "文本" });
  expect(h.redo(base)?.nodes[0].title).toBe("新名字2");
  expect(h.undo(base)?.nodes[0].x).toBe(20);
  h.record(moved, graph, "move", 400);
  expect(h.redo(base)).toBeUndefined();
});
it("保存后不能通过撤销移除节点，但布局可形成新命令", () => {
  const h = createHistory();
  h.record({ nodes: [], edges: [] }, graph);
  expect(h.canUndo(base)).toBe(false);
  expect(h.undo(base)).toBeUndefined();
  const empty = { ...base, nodes: [] };
  expect(h.undo(empty)?.nodes).toEqual([]);
});
it("只保留50次历史，清理后不跨画布撤销", () => {
  const h = createHistory();
  let before = graph;
  for (let i = 1; i <= 60; i++) {
    const next = { ...graph, nodes: [{ ...node, x: i }] };
    h.record(before, next, undefined, i * 1000);
    before = next;
  }
  let last;
  for (let i = 0; i < 50; i++) last = h.undo(base);
  expect(last?.nodes[0].x).toBe(10);
  expect(h.undo(base)).toBeUndefined();
  h.reset();
  expect(h.redo(base)).toBeUndefined();
});

it("撤销草稿编辑保留已保存引用编号的高水位，避免重新分配历史标签", () => {
  const before = emptyPromptDraft();
  const after = emptyPromptDraft();
  after.labelState.highWater.image = 2;
  after.labelState.allocations["retired"] = { labelKind: "image", ordinal: 2 };
  const n = {
    ...createDraftNode("image_generation", "1:1", 0),
    config: { draft: richCanvasDraft(before) },
    input_revision: 1,
  };
  const saved = {
    ...n,
    config: { draft: richCanvasDraft(after) },
    input_revision: 2,
  };
  const h = createHistory();
  h.record({ nodes: [n], edges: [] }, { nodes: [saved], edges: [] });
  const result = h.undo({ ...base, nodes: [saved] });
  expect(
    result?.nodes[0].config.draft?.composer?.labelState.highWater.image,
  ).toBe(2);
  expect(
    result?.nodes[0].config.draft?.composer?.labelState.allocations.retired
      .ordinal,
  ).toBe(2);
});
it("未保存的引用分配也不随画布撤销回退，重复撤销保留高水位", () => {
  const a = emptyPromptDraft(),
    b = emptyPromptDraft(),
    c = emptyPromptDraft();
  b.labelState.highWater.image = 1;
  b.labelState.allocations.first = { labelKind: "image", ordinal: 1 };
  c.labelState.highWater.image = 2;
  c.labelState.allocations = {
    ...b.labelState.allocations,
    second: { labelKind: "image", ordinal: 2 },
  };
  const n = {
    ...createDraftNode("image_generation", "1:1", 0),
    config: { draft: richCanvasDraft(a) },
    input_revision: 1,
  };
  const h = createHistory();
  h.record(
    { nodes: [n], edges: [] },
    { nodes: [{ ...n, config: { draft: richCanvasDraft(b) } }], edges: [] },
    undefined,
    0,
  );
  h.record(
    { nodes: [{ ...n, config: { draft: richCanvasDraft(b) } }], edges: [] },
    { nodes: [{ ...n, config: { draft: richCanvasDraft(c) } }], edges: [] },
    undefined,
    1000,
  );
  const saved = { ...base, nodes: [n] };
  expect(
    h.undo(saved)?.nodes[0].config.draft?.composer?.labelState.highWater.image,
  ).toBe(2);
  expect(
    h.undo(saved)?.nodes[0].config.draft?.composer?.labelState.allocations
      .second.ordinal,
  ).toBe(2);
});
it("composer转化不允许被画布历史撤回旧格式，使用正文自己的历史", () => {
  const legacy = {
    ...createDraftNode("image_generation", "1:1", 0),
    input_revision: 1,
  };
  const rich = {
    ...legacy,
    config: { draft: richCanvasDraft(emptyPromptDraft()) },
  };
  const h = createHistory();
  h.record({ nodes: [legacy], edges: [] }, { nodes: [rich], edges: [] });
  expect(h.canUndo({ ...base, nodes: [legacy] })).toBe(false);
  expect(h.canUndo({ ...base, nodes: [rich] })).toBe(false);
});
