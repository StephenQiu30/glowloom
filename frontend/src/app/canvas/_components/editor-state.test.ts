import { expect, it } from "vitest";
import {
  createResourceNode,
  commandBatch,
  createDraftNode,
  requireCanvas,
  requireSaveResult,
  sameEditable,
  createMediaNode,
  duplicateNode,
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
    last_operation_id: canvasID,
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

it("项目素材节点保存几何与名称，拒绝跨项目及受业务操作锁定的改写", () => {
  const n = createMediaNode(
    {
      id: crypto.randomUUID(),
      project_id: projectID,
      kind: "image",
      file_name: "参考.png",
      width: 1200,
      height: 800,
    },
    projectID,
    0,
  );
  const base = { ...empty, nodes: [n] };
  expect(n.height).toBe(192);
  const unicodeTitle = { ...n, title: "🎬".repeat(128) };
  expect(commandBatch(empty, [unicodeTitle]).commands?.[0].type).toBe(
    "AddNodes",
  );
  const edited = { ...n, x: 150, width: 400, title: "海边香氛" };
  const body = commandBatch(base, [edited]);
  expect(body.commands?.map((c) => c.type)).toEqual([
    "MoveNodes",
    "RenameNodes",
    "ResizeNodes",
  ]);
  expect(
    requireSaveResult(
      {
        ...base,
        revision: 2,
        nodes: [edited],
        results: body.commands?.map(() => ({ ok: true })),
      },
      base,
      [edited],
      body,
    ).nodes[0].title,
  ).toBe("海边香氛");
  expect(() =>
    createMediaNode(
      { id: n.ref_id, project_id: canvasID, kind: "image" },
      projectID,
      0,
    ),
  ).toThrow();
  expect(() =>
    commandBatch({ ...base, nodes: [{ ...n, last_operation_id: canvasID }] }, [
      { ...edited, last_operation_id: canvasID },
    ]),
  ).toThrow();
  expect(() => commandBatch(base, [{ ...n, ref_id: canvasID }])).toThrow();
});

it("复制保留素材身份和草稿内容，但生成新的节点身份及输入版本", () => {
  const draft = {
    ...createDraftNode("image_generation", "1:1", 0),
    input_revision: 3,
  };
  const copy = duplicateNode(draft);
  expect(copy.id).not.toBe(draft.id);
  expect(copy.input_revision).toBeUndefined();
  expect(copy.x).toBe(draft.x + 32);
  expect(copy.config).toEqual(draft.config);
  const media = createMediaNode(
    { id: canvasID, project_id: projectID, kind: "video" },
    projectID,
    0,
  );
  expect(duplicateNode(media).ref_id).toBe(media.ref_id);
  expect(() =>
    duplicateNode({ ...media, last_operation_id: canvasID }),
  ).toThrow();
});

it("文字分组及父子关系通过已有命令原子保存，坐标保持世界坐标", () => {
  const text = createResourceNode("text", { x: -120, y: 80 });
  const group = createResourceNode("group", { x: -160, y: 40 });
  const base = { ...empty, nodes: [text] };
  const target = [{ ...text, parent_id: group.id }, group];
  const body = commandBatch(base, target);
  expect(body.commands?.map((c) => c.type)).toEqual([
    "AddNodes",
    "SetNodeParents",
  ]);
  expect(body.commands?.[1].parents).toEqual([
    { id: text.id, parent_id: group.id },
  ]);
  const receipt = {
    ...base,
    revision: 2,
    nodes: target,
    results: [{ ok: true }, { ok: true }],
  };
  expect(requireSaveResult(receipt, base, target, body).nodes[0].x).toBe(-120);
  expect(() => commandBatch(base, [])).toThrow();
});
it("标注连线及相机生成差量，回执不得丢边或返回错误视角", () => {
  const a = createMediaNode(
    {
      id: "00000000-0000-4000-8000-000000000003",
      project_id: projectID,
      kind: "image",
    },
    projectID,
    0,
  );
  const b = { ...a, id: "00000000-0000-4000-8000-000000000004", x: 600 };
  const base = { ...empty, nodes: [a, b] };
  const edge = {
    id: "00000000-0000-4000-8000-000000000005",
    edge_type: "annotation",
    source_node_id: a.id,
    target_node_id: b.id,
  };
  const options = { edges: [edge], viewport: { x: -50, y: 100, zoom: 0.5 } };
  const body = commandBatch(base, base.nodes, options);
  expect(body.commands?.map((c) => c.type)).toEqual(["Connect", "SetViewport"]);
  const receipt = {
    ...base,
    ...options,
    revision: 2,
    results: [{ ok: true }, { ok: true }],
  };
  expect(requireSaveResult(receipt, base, base.nodes, body).edges).toEqual([
    edge,
  ]);
  expect(() =>
    requireSaveResult({ ...receipt, edges: [] }, base, base.nodes, body),
  ).toThrow();
  expect(() =>
    requireSaveResult(
      { ...receipt, viewport: { x: 0, y: 0, zoom: 1 } },
      base,
      base.nodes,
      body,
    ),
  ).toThrow();
  const connected = { ...base, edges: [edge] };
  expect(
    commandBatch(connected, connected.nodes, { edges: [] }).commands,
  ).toEqual([{ type: "Disconnect", ids: [edge.id] }]);
});
it("保护业务关系、生成草稿和锁定节点，不把标注连接当生成引用", () => {
  const a = createResourceNode("text", { x: 0, y: 0 });
  const draft = {
    ...createDraftNode("image_generation", "1:1", 0),
    input_revision: 1,
  };
  const edge = {
    id: crypto.randomUUID(),
    edge_type: "annotation",
    source_node_id: a.id,
    target_node_id: draft.id,
  };
  expect(() =>
    commandBatch({ ...empty, nodes: [a, draft] }, [a, draft], {
      edges: [edge],
    }),
  ).toThrow();
  const protectedEdge = { ...edge, edge_type: "business", role: "reference" };
  const base = { ...empty, nodes: [a, draft], edges: [protectedEdge] };
  expect(() => commandBatch(base, base.nodes, { edges: [] })).toThrow();
  const locked = { ...a, last_operation_id: crypto.randomUUID() };
  expect(() =>
    commandBatch({ ...empty, nodes: [locked] }, [{ ...locked, x: 20 }]),
  ).toThrow();
});
it("已有composer不能被旧草稿格式覆盖", () => {
  const legacy = {
    ...createDraftNode("image_generation", "1:1", 0),
    input_revision: 1,
  };
  const rich = {
    ...legacy,
    config: {
      draft: {
        composer: {
          schemaVersion: 1 as const,
          document: {
            type: "doc" as const,
            content: [{ type: "paragraph" as const }],
          },
          references: [],
          modelVersion: null,
          mode: null,
          params: {},
          labelState: {
            highWater: { image: 0, video: 0, audio: 0, subject: 0 },
            allocations: {},
          },
        },
        prompt: "",
        negative_prompt: "",
        aspect_ratio: "",
        candidate_count: 0,
      },
    },
  };
  expect(() => commandBatch({ ...empty, nodes: [rich] }, [legacy])).toThrow();
});
it("4000条合法标注增删按单命令2000项拆分", () => {
  const a = createResourceNode("text", { x: 0, y: 0 }),
    b = createResourceNode("text", { x: 500, y: 0 });
  const edges = Array.from({ length: 4000 }, (_, i) => ({
    id: `00000000-0000-4000-8000-${String(i + 100).padStart(12, "0")}`,
    edge_type: "annotation",
    source_node_id: a.id,
    target_node_id: b.id,
  }));
  const base = { ...empty, nodes: [a, b] };
  const add = commandBatch(base, base.nodes, { edges });
  expect(add.commands?.map((c) => [c.type, c.edges?.length])).toEqual([
    ["Connect", 2000],
    ["Connect", 2000],
  ]);
  const remove = commandBatch({ ...base, edges }, base.nodes, { edges: [] });
  expect(remove.commands?.map((c) => [c.type, c.ids?.length])).toEqual([
    ["Disconnect", 2000],
    ["Disconnect", 2000],
  ]);
});
