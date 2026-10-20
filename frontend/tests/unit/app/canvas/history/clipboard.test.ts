import { expect, it } from "vitest";
import {
  encodeCanvasClipboard,
  pasteCanvasClipboard,
  remapCopiedNodeReferences,
} from "@/app/canvas/history/clipboard";
import {
  createDraftNode,
  createMediaNode,
  createResourceNode,
  duplicateNode,
  type Node,
} from "@/app/canvas/document";
import {
  createDirectorConfig,
  createDirectorObject,
} from "@/app/canvas/director/model";
const project = "00000000-0000-4000-8000-000000000001";
it("剪贴板保留分组和内部线但重新分配节点与边身份", () => {
  const group = createResourceNode("group", { x: 10, y: 20 }),
    child = createResourceNode("text", { x: 50, y: 60 });
  child.parent_id = group.id;
  const edges = [
    {
      id: crypto.randomUUID(),
      edge_type: "annotation",
      source_node_id: group.id,
      target_node_id: child.id,
    },
  ];
  const text = encodeCanvasClipboard(
    project,
    [group, child],
    edges,
    new Set([group.id]),
  );
  const pasted = pasteCanvasClipboard(text, project, { x: 200, y: 300 });
  expect(pasted.nodes).toHaveLength(2);
  expect(pasted.nodes[0].id).not.toBe(group.id);
  expect(pasted.nodes[1].parent_id).toBe(pasted.nodes[0].id);
  expect(pasted.nodes[0]).toMatchObject({ x: 200, y: 300 });
  expect(pasted.edges[0]).toMatchObject({
    source_node_id: pasted.nodes[0].id,
    target_node_id: pasted.nodes[1].id,
  });
  expect(pasted.edges[0].id).not.toBe(edges[0].id);
});
it("拒绝跨项目、只读投影和未验证剪贴板载荷", () => {
  const n = createResourceNode("text", { x: 0, y: 0 });
  const encoded = encodeCanvasClipboard(project, [n], [], new Set([n.id]));
  expect(() => pasteCanvasClipboard(encoded, crypto.randomUUID())).toThrow();
  expect(() =>
    pasteCanvasClipboard(
      '{"type":"glowloom.canvas","version":1,"nodes":[{}]}',
      project,
    ),
  ).toThrow();
  expect(() =>
    encodeCanvasClipboard(
      project,
      [{ ...n, last_operation_id: crypto.randomUUID() }],
      [],
      new Set([n.id]),
    ),
  ).toThrow();
});
it("生成草稿副本丢弃原输入版本，空选择不覆盖系统剪贴板", () => {
  const n = {
    ...createDraftNode("image_generation", "1:1", 0),
    input_revision: 4,
  };
  const text = encodeCanvasClipboard(project, [n], [], new Set([n.id]));
  expect(
    pasteCanvasClipboard(text, project).nodes[0].input_revision,
  ).toBeUndefined();
  expect(() => encodeCanvasClipboard(project, [n], [], new Set())).toThrow();
});

function directorWithSource(media: Node): Node {
  const config = createDirectorConfig();
  config.objects = [
    {
      ...createDirectorObject("plane"),
      kind: "billboard",
      source_node_id: media.id,
      asset_id: media.ref_id,
    },
  ];
  return {
    ...createResourceNode("text", { x: 0, y: 0 }),
    node_type: "director",
    node_action: "tool",
    config: { director: config },
  };
}
function imageNode(): Node {
  return createMediaNode(
    {
      id: crypto.randomUUID(),
      project_id: project,
      kind: "image",
      file_name: "来源图片",
    },
    project,
    0,
  );
}
it("同组媒体与导演台粘贴到同项目另一画布时只引用副本节点", () => {
  const group = createResourceNode("group", { x: 10, y: 10 }),
    media = imageNode(),
    director = directorWithSource(media);
  media.parent_id = group.id;
  director.parent_id = group.id;
  const original = structuredClone([group, media, director]);
  const encoded = encodeCanvasClipboard(
    project,
    [group, media, director],
    [],
    new Set([group.id]),
  );
  const pasted = pasteCanvasClipboard(encoded, project);
  const copiedMedia = pasted.nodes.find((n) => n.node_type === "image")!,
    copiedDirector = pasted.nodes.find((n) => n.node_type === "director")!;
  expect(copiedDirector.config.director!.objects![0]).toMatchObject({
    source_node_id: copiedMedia.id,
    asset_id: media.ref_id,
  });
  expect(copiedMedia.id).not.toBe(media.id);
  expect(
    pasted.nodes.some(
      (n) =>
        n.id === copiedDirector.config.director!.objects![0].source_node_id,
    ),
  ).toBe(true);
  expect([group, media, director]).toEqual(original);
});
it("单独复制导演台先从未复制的来源冻结真实asset_id，可跨画布粘贴", () => {
  const media = imageNode(),
    director = directorWithSource(media);
  delete director.config.director!.objects![0].asset_id;
  const encoded = encodeCanvasClipboard(
    project,
    [media, director],
    [],
    new Set([director.id]),
  );
  const decoded = JSON.parse(encoded).nodes[0] as Node;
  expect(decoded.config.director!.objects![0].source_node_id).toBeUndefined();
  expect(decoded.config.director!.objects![0].asset_id).toBe(media.ref_id);
  const pasted = pasteCanvasClipboard(encoded, project);
  expect(pasted.nodes).toHaveLength(1);
  expect(pasted.nodes[0].config.director!.objects![0]).toMatchObject({
    asset_id: media.ref_id,
  });
  expect(
    pasted.nodes[0].config.director!.objects![0].source_node_id,
  ).toBeUndefined();
  expect(director.config.director!.objects![0].source_node_id).toBe(media.id);
});
it("共享重映射函数支持副本路径且不改动输入对象", () => {
  const media = imageNode(),
    director = directorWithSource(media),
    originals = [media, director],
    copies = originals.map(duplicateNode),
    before = structuredClone(copies);
  const remapped = remapCopiedNodeReferences(originals, copies);
  expect(remapped[1].config.director!.objects![0].source_node_id).toBe(
    copies[0].id,
  );
  expect(copies).toEqual(before);
  const solo = remapCopiedNodeReferences(
    [director],
    [duplicateNode(director)],
    [media, director],
  );
  expect(solo[0].config.director!.objects![0]).toMatchObject({
    asset_id: media.ref_id,
  });
  expect(solo[0].config.director!.objects![0].source_node_id).toBeUndefined();
});
it("旧时间线内部引用重映射、外部引用冻结，不启用任何时间线编辑入口", () => {
  const media = imageNode(),
    timeline: Node = {
      ...createResourceNode("text", { x: 0, y: 0 }),
      node_type: "timeline",
      node_action: "tool",
      config: {
        timeline: {
          version: 1,
          clips: [
            { id: crypto.randomUUID(), kind: "image", node_id: media.id },
          ],
        },
      },
    };
  const copies = remapCopiedNodeReferences(
    [media, timeline],
    [duplicateNode(media), duplicateNode(timeline)],
  );
  expect(copies[1].config.timeline!.clips![0]).toMatchObject({
    node_id: copies[0].id,
    asset_id: media.ref_id,
  });
  const solo = remapCopiedNodeReferences(
    [timeline],
    [duplicateNode(timeline)],
    [media, timeline],
  );
  expect(solo[0].config.timeline!.clips![0].node_id).toBeUndefined();
  expect(solo[0].config.timeline!.clips![0].asset_id).toBe(media.ref_id);
});
it("拒绝无法解析的来源、类型错误和相互冲突的素材身份", () => {
  const media = imageNode(),
    director = directorWithSource(media);
  expect(() =>
    encodeCanvasClipboard(project, [director], [], new Set([director.id])),
  ).toThrow(/来源/);
  const forged = JSON.stringify({
    type: "glowloom.canvas",
    version: 1,
    project_id: project,
    nodes: [director],
    edges: [],
  });
  expect(() => pasteCanvasClipboard(forged, project)).toThrow(/来源/);
  director.config.director!.objects![0].asset_id = crypto.randomUUID();
  expect(() =>
    remapCopiedNodeReferences(
      [media, director],
      [duplicateNode(media), duplicateNode(director)],
    ),
  ).toThrow(/素材/);
  director.config.director!.objects![0].asset_id = media.ref_id;
  director.config.director!.objects![0].kind = "model";
  expect(() =>
    remapCopiedNodeReferences(
      [media, director],
      [duplicateNode(media), duplicateNode(director)],
    ),
  ).toThrow(/来源/);
});
