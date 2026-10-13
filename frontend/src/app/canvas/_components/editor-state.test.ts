import { expect, it } from "vitest";
import {
  commandBatch,
  createDraftNode,
  requireCanvas,
  requireSaveResult,
  sameEditable,
} from "./editor-state";
const projectID = "00000000-0000-4000-8000-000000000001";
const canvasID = "00000000-0000-4000-8000-000000000002";
const empty = {
  id: canvasID,
  project_id: projectID,
  revision: 1,
  nodes: [],
  edges: [],
  viewport: { x: 0, y: 0, zoom: 1 },
};
it("新增命令不传服务端输入版本，移动和改稿明确分开", () => {
  const base = requireCanvas(empty, projectID, canvasID);
  const n = createDraftNode("image_generation", "1:1", 0);
  const batch = commandBatch(base, [n]);
  expect(batch.expected_revision).toBe(1);
  expect(batch.commands?.[0].type).toBe("AddNodes");
  expect(batch.commands?.[0].nodes?.[0]).not.toHaveProperty("input_revision");
  const saved = { ...n, input_revision: 1 };
  const next = { ...base, revision: 2, nodes: [saved] };
  const moved = { ...saved, x: 60 };
  expect(commandBatch(next, [moved]).commands?.map((c) => c.type)).toEqual([
    "MoveNodes",
  ]);
  const edited = {
    ...moved,
    config: { draft: { ...moved.config.draft, prompt: "New" } },
  };
  expect(commandBatch(next, [edited]).commands?.map((c) => c.type)).toEqual([
    "MoveNodes",
    "UpdateNodeConfig",
  ]);
});
it("无效回执、错误revision和只读节点改写不能确认保存", () => {
  const base = requireCanvas(empty, projectID, canvasID);
  const n = createDraftNode("video_generation", "9:16", 0);
  const body = commandBatch(base, [n]);
  const bad = { ...empty, revision: 2, results: [{ ok: true }] };
  expect(() => requireSaveResult(bad, base, [n], body)).toThrow();
  const valid = { ...bad, nodes: [{ ...n, input_revision: 1 }] };
  expect(requireSaveResult(valid, base, [n], body).revision).toBe(2);
  expect(() =>
    requireSaveResult({ ...valid, revision: 4 }, base, [n], body),
  ).toThrow();
  const readonly = {
    id: crypto.randomUUID(),
    title: "Legacy",
    node_type: "text",
    node_action: "resource",
    config: { text: "Original" },
    x: 0,
    y: 0,
    z_index: 0,
  };
  expect(() =>
    commandBatch({ ...base, nodes: [readonly] }, [
      { ...readonly, title: "Changed" },
    ]),
  ).toThrow();
});

it("尺寸通过ResizeNodes保存，错误尺寸回执不能确认保存", () => {
  const n = {
    ...createDraftNode("image_generation", "1:1", 0),
    input_revision: 2,
  };
  const base = { ...empty, revision: 5, nodes: [n] };
  const resized = { ...n, width: 400, height: 260 };
  expect(sameEditable(n, resized)).toBe(false);
  const body = commandBatch(base, [resized]);
  expect(body.commands).toEqual([
    { type: "ResizeNodes", sizes: [{ id: n.id, width: 400, height: 260 }] },
  ]);
  expect(() =>
    requireSaveResult(
      { ...base, revision: 6, results: [{ ok: true }] },
      base,
      [resized],
      body,
    ),
  ).toThrow();
  expect(
    requireSaveResult(
      { ...base, revision: 6, nodes: [resized], results: [{ ok: true }] },
      base,
      [resized],
      body,
    ).nodes[0].input_revision,
  ).toBe(2);
  for (const width of [0, 2001, Number.NaN]) {
    expect(() => commandBatch(base, [{ ...resized, width }])).toThrow();
    expect(() =>
      requireCanvas(
        { ...base, nodes: [{ ...resized, width }] },
        projectID,
        canvasID,
      ),
    ).toThrow();
  }
});
it("回执输入版本必须与本次草稿变更一致", () => {
  const n = {
    ...createDraftNode("image_generation", "1:1", 0),
    input_revision: 2,
  };
  const base = { ...empty, revision: 5, nodes: [n] };
  const edited = {
    ...n,
    config: { draft: { ...n.config.draft, prompt: "新草稿" } },
  };
  const body = commandBatch(base, [edited]);
  const receipt = {
    ...base,
    revision: 6,
    nodes: [edited],
    results: [{ ok: true }],
  };
  expect(() => requireSaveResult(receipt, base, [edited], body)).toThrow();
  expect(
    requireSaveResult(
      { ...receipt, nodes: [{ ...edited, input_revision: 3 }] },
      base,
      [edited],
      body,
    ).nodes[0].input_revision,
  ).toBe(3);
  expect(() =>
    requireSaveResult(
      { ...receipt, nodes: [{ ...edited, input_revision: 4 }] },
      base,
      [edited],
      body,
    ),
  ).toThrow();
});

it("生成结果以真实媒体引用添加，不接受缺失或任意引用", () => {
  const base = requireCanvas(empty, projectID, canvasID);
  const result = {
    id: crypto.randomUUID(),
    node_type: "image",
    node_action: "resource",
    ref_type: "media_asset",
    ref_id: crypto.randomUUID(),
    config: {},
    x: 100,
    y: 100,
    z_index: 1,
    title: "生成结果",
  };
  expect(commandBatch(base, [result]).commands?.[0].nodes?.[0].ref_id).toBe(
    result.ref_id,
  );
  expect(() => commandBatch(base, [{ ...result, ref_id: "bad" }])).toThrow();
  expect(() =>
    commandBatch(base, [{ ...result, node_type: "script" }]),
  ).toThrow();
});
