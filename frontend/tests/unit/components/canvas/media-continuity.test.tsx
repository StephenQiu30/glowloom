import { cleanup, renderHook, act } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it } from "vitest";
import {
  fitCanvasMediaNode,
  useCanvasMediaSize,
} from "@/components/canvas/hooks/use-canvas-media-size";
import { replaceCanvasNodeAsset } from "@/components/canvas/lib/canvas-asset-replacement";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";

afterEach(cleanup);
function node(): CanvasNodeData {
  return {
    id: "media",
    type: "image",
    title: "素材",
    width: 320,
    height: 320,
    position: { x: 10, y: 20 },
    metadata: { content: "/image.png" },
    documentData: {
      id: "media",
      type: "image",
      nodeTypeVersion: 1,
      title: "素材",
      width: 320,
      height: 320,
      position: { x: 10, y: 20 },
      zIndex: 0,
      fileReferences: [
        {
          referenceKey: "primary",
          fileId: "file",
          version: "1",
          purpose: "CONTENT",
        },
      ],
    },
  };
}
it("已有服务节点解码后匹配素材比例，保持中心", () => {
  const result = fitCanvasMediaNode(
    node(),
    { source: "/image.png", width: 640, height: 360 },
    "https://app.test",
  );
  expect([result.width, result.height]).toEqual([320, 180]);
  expect(result.position).toEqual({ x: 10, y: 90 });
});
it.each(["image", "video"] as const)(
  "%s 替换使用回执尺寸并保持节点及引用身份",
  (type) => {
    const before = { ...node(), type };
    const asset = {
      kind: type,
      fileId: "new-file",
      fileVersion: "2",
      title: "新素材",
      dataUrl: "/new.png",
      url: "/new.mp4",
      width: 360,
      height: 640,
    };
    const result = replaceCanvasNodeAsset(before, asset);
    expect([result.width, result.height]).toEqual([180, 320]);
    expect(result.position).toEqual({ x: 80, y: 20 });
    expect(result.metadata?.freeResize).toBe(false);
    expect(result.documentData?.fileReferences?.[0]).toMatchObject({
      referenceKey: "primary",
      fileId: "new-file",
      version: "2",
    });
  },
);
it("错误或旧来源测量不改变节点", () => {
  const before = node();
  for (const measurement of [
    { source: "/old.png", width: 640, height: 360 },
    { source: "/image.png", width: 0, height: 100 },
  ])
    expect(fitCanvasMediaNode(before, measurement, "https://app.test")).toBe(
      before,
    );
});
it("迟到解码在手势结束后调整，重复解码不重复提交", () => {
  const hook = renderHook(
    ({ enabled }) => {
      const [nodes, setNodes] = useState([node()]);
      const measure = useCanvasMediaSize(nodes, enabled, setNodes);
      return { nodes, measure };
    },
    { initialProps: { enabled: false } },
  );
  act(() =>
    hook.result.current.measure("media", "/image.png", {
      width: 640,
      height: 360,
    }),
  );
  expect(hook.result.current.nodes[0].height).toBe(320);
  hook.rerender({ enabled: true });
  expect(hook.result.current.nodes[0].height).toBe(180);
  const fitted = hook.result.current.nodes;
  act(() =>
    hook.result.current.measure("media", "/image.png", {
      width: 640,
      height: 360,
    }),
  );
  expect(hook.result.current.nodes).toBe(fitted);
});

it("保存回执不能清空同版本文件的已加载预览", async () => {
  const { retainCanvasMediaPreviews } =
    await import("@/components/canvas/lib/canvas-media-continuity");
  const graph = {
    nodes: [node()],
    connections: [],
    viewport: { x: 0, y: 0, k: 1 },
    backgroundMode: "dots" as const,
    showImageInfo: false,
  };
  const receipt = structuredClone(graph);
  receipt.nodes[0].position.x += 50;
  receipt.nodes[0].metadata!.content = "";
  const result = {
    ...receipt,
    nodes: retainCanvasMediaPreviews(graph.nodes, receipt.nodes),
  };
  expect(result.nodes[0].metadata?.content).toBe("/image.png");
  expect(result.nodes[0].position.x).toBe(60);
});

it("远端同文件更新保留预览但不覆盖几何、标题或配置", async () => {
  const { retainCanvasMediaPreviews } =
    await import("@/components/canvas/lib/canvas-media-continuity");
  const old = node();
  old.metadata!.naturalWidth = 640;
  const incoming = {
    ...node(),
    title: "远端标题",
    position: { x: 900, y: 800 },
    width: 160,
    height: 90,
    metadata: { content: "", prompt: "远端输入" },
  };
  const result = retainCanvasMediaPreviews([old], [incoming])[0];
  expect(result).toMatchObject({
    title: "远端标题",
    position: { x: 900, y: 800 },
    width: 160,
    height: 90,
    metadata: { content: "/image.png", prompt: "远端输入", naturalWidth: 640 },
  });
  expect(incoming.metadata.content).toBe("");
});
it.each([
  "fileId",
  "version",
  "referenceKey",
  "publicAssetId",
  "publicRevision",
])("%s 变化不继承旧预览", async (field) => {
  const { retainCanvasMediaPreviews } =
    await import("@/components/canvas/lib/canvas-media-continuity");
  const incoming = node();
  incoming.metadata!.content = "";
  Object.assign(incoming.documentData!.fileReferences![0], {
    [field]: "changed",
  });
  expect(
    retainCanvasMediaPreviews([node()], [incoming])[0].metadata?.content,
  ).toBe("");
});
it("移除、删除、新节点及跨类型不会复活旧媒体，已给出新地址不覆盖", async () => {
  const { retainCanvasMediaPreviews } =
    await import("@/components/canvas/lib/canvas-media-continuity");
  const removed = node();
  removed.metadata!.content = "";
  removed.documentData!.fileReferences = [];
  const fresh = { ...node(), id: "new", metadata: { content: "" } };
  const text = { ...node(), type: "text", metadata: { content: "" } };
  const renewed = { ...node(), metadata: { content: "/renewed.png" } };
  for (const incoming of [removed, fresh, text, renewed])
    expect(retainCanvasMediaPreviews([node()], [incoming])[0]).toBe(incoming);
  expect(retainCanvasMediaPreviews([node()], [])).toEqual([]);
});
it("临时预览和自然尺寸不形成业务写入", async () => {
  const { serviceGraph, serviceDocumentFromEditor } =
    await import("@/components/canvas/service-graph");
  const dto: API.DocumentDTO = {
    id: "11111111-1111-4111-8111-111111111111",
    project_id: "22222222-2222-4222-8222-222222222222",
    revision: "1",
    schema_version: 1,
    operation_version: 1,
    permissions: { read: true, write: true },
    nodes: [
      {
        id: "33333333-3333-4333-8333-333333333333",
        node_type: "image",
        node_action: "resource",
        title: "图片",
        width: 320,
        height: 180,
        x: 10,
        y: 20,
        config: {},
      },
    ],
    edges: [],
  };
  const after = serviceGraph(dto, dto.id!, dto.project_id!);
  after.nodes[0].metadata!.content = "https://media.test/signed-preview";
  after.nodes[0].metadata!.naturalWidth = 640;
  after.nodes[0].metadata!.naturalHeight = 360;
  expect(serviceDocumentFromEditor(dto, after)).toEqual(dto);
});

it("用户显式自由比例不被解码重置，替换素材则重新匹配比例", () => {
  const original = {
    ...node(),
    metadata: { content: "/image.png", freeResize: true },
  };
  const decoded = fitCanvasMediaNode(
    original,
    { source: "/image.png", width: 640, height: 360 },
    "https://app.test",
  );
  expect([decoded.width, decoded.height]).toEqual([320, 320]);
  expect(decoded.metadata?.freeResize).toBe(true);
  const replaced = replaceCanvasNodeAsset(decoded, {
    kind: "image",
    dataUrl: "/new.png",
    title: "新图",
    fileId: "new",
    fileVersion: "1",
    width: 360,
    height: 640,
  });
  expect([replaced.width, replaced.height]).toEqual([180, 320]);
  expect(replaced.metadata?.freeResize).toBe(false);
});

it("已加载预览与纯服务端草稿之间替换素材，不把临时URL和解码尺寸当成并发业务冲突", async () => {
  const { retainCanvasMediaPreviews } =
    await import("@/components/canvas/lib/canvas-media-continuity");
  const graph = {
    nodes: [node()],
    connections: [],
    viewport: { x: 0, y: 0, k: 1 },
    backgroundMode: "dots" as const,
    showImageInfo: false,
  };
  graph.nodes[0].metadata!.naturalWidth = 640;
  const authority = structuredClone(graph);
  authority.nodes[0].metadata = { content: "" };
  const replacement = replaceCanvasNodeAsset(graph.nodes[0], {
    kind: "image",
    dataUrl: "/new.png",
    title: "新图",
    fileId: "new",
    fileVersion: "1",
    width: 360,
    height: 640,
  });
  const result = {
    nodes: retainCanvasMediaPreviews(authority.nodes, [replacement]),
  };
  expect(result.nodes[0].metadata?.content).toBe("/new.png");
  expect(result.nodes[0].documentData?.fileReferences?.[0].fileId).toBe("new");
  expect(result.nodes[0].width).toBe(180);
});

it("Yjs已确认另一素材身份时，不把旧预览覆盖回去", async () => {
  const { retainCanvasMediaPreviews } =
    await import("@/components/canvas/lib/canvas-media-continuity");
  const graph = {
    nodes: [node()],
    connections: [],
    viewport: { x: 0, y: 0, k: 1 },
    backgroundMode: "dots" as const,
    showImageInfo: false,
  };
  const replace = (fileId: string) => ({
    ...graph,
    nodes: [
      replaceCanvasNodeAsset(node(), {
        kind: "image",
        fileId,
        fileVersion: "1",
        dataUrl: `/${fileId}.png`,
        title: fileId,
        width: 640,
        height: 360,
      }),
    ],
  });
  const result = retainCanvasMediaPreviews(
    replace("mine").nodes,
    replace("theirs").nodes,
  );
  expect(result[0].documentData!.fileReferences![0].fileId).toBe("theirs");
  expect(result[0].metadata!.content).toBe("/theirs.png");
});
