import { expect, it } from "vitest";
import {
  emptyPromptDraft,
  validatePromptDraft,
} from "@/components/generation/prompt-document";
import { createDraftNode, createMediaNode, commandBatch } from "./editor-state";
import {
  adjacentGraph,
  referenceEdges,
  referenceSources,
} from "./canvas-references";

const pid = "00000000-0000-4000-8000-000000000001";
const asset: API.AssetSummary = {
  id: "00000000-0000-4000-8000-000000000002",
  project_id: pid,
  kind: "image",
  revision: 3,
  file_name: "参考图",
};

it("侧边创建与定向布局线形成同一命令批次，空节点没有虚假引用", () => {
  const source = createDraftNode("image_generation", "1:1", 0);
  const result = adjacentGraph(source, "video_generation", "right", "1:1");
  const base = {
    id: crypto.randomUUID(),
    project_id: pid,
    revision: 1,
    nodes: [source],
    edges: [],
  };
  const body = commandBatch(base, [source, result.node], {
    edges: [result.edge],
  });
  expect(body.commands?.map((c) => c.type)).toEqual(["AddNodes", "Connect"]);
  expect(result.edge).toMatchObject({
    source_node_id: source.id,
    target_node_id: result.node.id,
    edge_type: "annotation",
  });
  expect(result.node.config.draft?.composer?.references ?? []).toEqual([]);
  const left = adjacentGraph(source, "text", "left", "1:1");
  expect(left.edge.source_node_id).toBe(left.node.id);
  expect(left.edge.target_node_id).toBe(source.id);
});

it("已核验媒体自动绑定版本与编号，引用投影不写入业务边", () => {
  const source = createMediaNode(asset, pid, 0);
  const { node, edge } = adjacentGraph(
    source,
    "video_generation",
    "right",
    "1:1",
    asset,
  );
  const draft = node.config.draft!.composer!;
  validatePromptDraft(draft);
  expect(draft.references).toEqual([
    expect.objectContaining({
      id: edge.id,
      role: "reference_image",
      source: { kind: "media", sourceId: asset.id, sourceVersion: "3" },
    }),
  ]);
  const lines = referenceEdges([source, node], [edge]);
  expect(lines).toHaveLength(1);
  expect(lines[0]).toMatchObject({
    id: edge.id,
    edge_type: "prompt_reference",
  });
  const removed = {
    ...node,
    config: { draft: { ...node.config.draft!, composer: emptyPromptDraft() } },
  };
  expect(referenceEdges([source, removed], [edge])[0].edge_type).toBe(
    "annotation",
  );
  expect(referenceEdges([source, removed], [])).toEqual([]);
});

it("拒绝错误素材回执，画布候选保留空节点但不作为有效媒体", () => {
  const source = createMediaNode(asset, pid, 0);
  expect(() =>
    adjacentGraph(source, "video_generation", "right", "1:1", {
      ...asset,
      id: crypto.randomUUID(),
    }),
  ).toThrow();
  const empty = createDraftNode("image_generation", "1:1", 1);
  expect(referenceSources([source, empty])).toEqual([
    expect.objectContaining({
      nodeID: source.id,
      assetID: asset.id,
      kind: "image",
    }),
    expect.objectContaining({
      nodeID: empty.id,
      assetID: undefined,
      kind: "image",
    }),
  ]);
});

it("侧边重复创建避让已有节点，分组背景不阻挡放置", () => {
  const source = createDraftNode("video_generation", "1:1", 0);
  const first = adjacentGraph(source, "image_generation", "left", "1:1");
  const second = adjacentGraph(
    source,
    "image_generation",
    "left",
    "1:1",
    undefined,
    [source, first.node],
  );
  expect(second.node.x).toBe(first.node.x);
  expect(second.node.y).toBeGreaterThanOrEqual(
    first.node.y + first.node.height! + 24,
  );
});

it("同一素材有多个画布副本时，自动引用保持原连接来源", () => {
  const first = createMediaNode(asset, pid, 0);
  const source = { ...first, id: crypto.randomUUID(), x: 800 };
  const { node, edge } = adjacentGraph(
    source,
    "video_generation",
    "right",
    "1:1",
    asset,
  );
  const lines = referenceEdges([first, source, node], [edge]);
  expect(lines).toHaveLength(1);
  expect(lines[0]).toMatchObject({
    source_node_id: source.id,
    target_node_id: node.id,
    edge_type: "prompt_reference",
  });
});
