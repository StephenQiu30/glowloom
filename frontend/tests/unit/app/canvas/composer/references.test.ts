import { expect, it } from "vitest";
import {
  emptyPromptDraft,
  validatePromptDraft,
} from "@/components/generation/prompt-document";
import {
  createDraftNode,
  createMediaNode,
  commandBatch,
} from "@/app/canvas/document";
import {
  adjacentGraph,
  materializeReferences,
  referenceEdges,
  referenceSources,
  connectedReferenceSources,
} from "@/app/canvas/composer/references";

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

it("参考候选只取当前节点实际相连的媒体，兼容连线两端并排除业务边", () => {
  const owner = createDraftNode("video_generation", "16:9", 0);
  const image = createMediaNode(asset, pid, 0);
  const empty = createDraftNode("image_generation", "1:1", 1);
  const unrelated = { ...image, id: crypto.randomUUID() };
  const edges = [
    {
      id: "out",
      edge_type: "annotation",
      source_node_id: owner.id,
      target_node_id: image.id,
    },
    {
      id: "in",
      edge_type: "annotation",
      source_node_id: empty.id,
      target_node_id: owner.id,
    },
    {
      id: "business",
      edge_type: "dependency",
      source_node_id: unrelated.id,
      target_node_id: owner.id,
    },
  ];
  expect(
    connectedReferenceSources(
      [owner, image, empty, unrelated],
      edges,
      owner.id,
    ),
  ).toEqual([
    {
      nodeID: image.id,
      edgeID: "out",
      title: image.title,
      kind: "image",
      assetID: asset.id,
    },
    {
      nodeID: empty.id,
      edgeID: "in",
      title: empty.title,
      kind: "image",
      assetID: undefined,
    },
  ]);
});

it("反向标注线引用已有素材时复用原线，保留同素材副本的来源", () => {
  const image = createMediaNode(asset, pid, 0);
  const duplicate = { ...image, id: crypto.randomUUID() };
  const { node, edge } = adjacentGraph(
    image,
    "video_generation",
    "right",
    "1:1",
    asset,
  );
  const reverse = {
    ...edge,
    source_node_id: node.id,
    target_node_id: image.id,
  };
  expect(referenceEdges([duplicate, image, node], [reverse])).toEqual([
    { ...reverse, edge_type: "prompt_reference" },
  ]);
  const otherBinding = structuredClone(node);
  otherBinding.config.draft!.composer!.references[0].id = crypto.randomUUID();
  expect(referenceEdges([duplicate, image, otherBinding], [reverse])).toEqual([
    { ...reverse, edge_type: "prompt_reference" },
  ]);
});

it("上传或库引用复用媒体节点与同一关系，正文编辑不重复建图", () => {
  const owner = createDraftNode("video_generation", "16:9", 0);
  const draft = emptyPromptDraft();
  draft.references = [
    {
      id: crypto.randomUUID(),
      order: 0,
      bindingGeneration: 1,
      mediaKind: "image",
      role: "reference_image",
      source: { kind: "media", sourceId: asset.id!, sourceVersion: "3" },
    },
  ];
  const first = materializeReferences([owner], [], owner.id, draft, pid);
  expect(first.nodes).toHaveLength(2);
  expect(first.edges).toHaveLength(1);
  const second = materializeReferences(
    first.nodes,
    first.edges,
    owner.id,
    draft,
    pid,
  );
  expect(second.nodes).toHaveLength(2);
  expect(second.edges).toEqual(first.edges);
  expect(second.nodes.find((n) => n.ref_id === asset.id)?.node_type).toBe(
    "image",
  );
});
