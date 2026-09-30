import axios, { AxiosError } from "axios";
import { afterEach, expect, it, vi } from "vitest";
import { createNode } from "./document";
import { CanvasNodeType } from "./model";
import {
  decodeDocument,
  saveCanvasCommands,
  renameCanvas,
  deleteCanvas,
  getMediaPreview,
} from "./queries";
const id = (n: number) =>
  `00000000-0000-4000-8000-${String(n).padStart(12, "0")}`;
const payload = () => ({
  id: id(1),
  project_id: id(2),
  name: "画布",
  scope: {},
  revision: 2,
  viewport: { x: 0, y: 0, zoom: 1 },
  nodes: [],
  edges: [],
});
afterEach(() => {
  vi.restoreAllMocks();
});
it("历史省略尺寸有显式默认值，父group与媒体仅映射正式身份", () => {
  const decoded = decodeDocument({
    ...payload(),
    nodes: [
      {
        id: id(3),
        node_type: "group",
        node_action: "resource",
        title: "组",
        config: {},
        x: 0,
        y: 0,
        z_index: 0,
      },
      {
        id: id(4),
        node_type: "video",
        node_action: "resource",
        title: "视频",
        config: { url: "https://untrusted.test/v.mp4" },
        x: 0,
        y: 0,
        z_index: 0,
        parent_id: id(3),
        ref_type: "media_asset",
        ref_id: id(5),
      },
    ],
  });
  expect(decoded.nodes[0]).toMatchObject({
    width: 640,
    height: 420,
    metadata: {
      frame: { expandedWidth: 640, expandedHeight: 420, collapsed: false },
    },
  });
  expect(decoded.nodes[1]).toMatchObject({
    width: 320,
    height: 220,
    parentId: id(3),
    assetId: id(5),
    metadata: {},
  });
  expect(decoded.nodes[1]).not.toHaveProperty("media");
  expect(() =>
    decodeDocument({ ...payload(), id: "sample-project" }),
  ).toThrow();
});
it("生成客户端保存链正确映射viewport/edge/null父关系并携带稳定幂等键", async () => {
  const send = vi
    .spyOn(axios, "request")
    .mockResolvedValue({ data: payload() });
  const node = createNode(CanvasNodeType.Image, { x: -25, y: 40 }, id(4), {
    assetId: id(5),
    media: {
      name: "Untrusted.png",
      mimeType: "image/png",
      url: "https://untrusted.test/a.png",
      expiresAt: "2099-01-01T00:00:00Z",
    },
  });
  await saveCanvasCommands(
    id(1),
    1,
    [
      { type: "AddNodes", nodes: [node] },
      {
        type: "Connect",
        edges: [{ id: id(6), fromNodeId: id(4), toNodeId: id(7) }],
      },
      { type: "SetNodeParents", parents: [{ id: id(4), parent_id: null }] },
      { type: "SetViewport", viewport: { x: 2, y: 3, k: 0.05 } },
    ],
    "same-key",
  );
  const request = send.mock.calls[0][0];
  expect(request).toMatchObject({
    url: `/api/canvases/${id(1)}/commands`,
    method: "POST",
    withCredentials: false,
    headers: {
      "Idempotency-Key": "same-key",
    },
    data: {
      expected_revision: 1,
      commands: [
        {
          type: "AddNodes",
          nodes: [
            {
              id: id(4),
              node_type: "image",
              node_action: "resource",
              ref_type: "media_asset",
              ref_id: id(5),
              config: {},
              x: -25,
              y: 40,
            },
          ],
        },
        {
          type: "Connect",
          edges: [
            {
              id: id(6),
              edge_type: "annotation",
              source_node_id: id(4),
              target_node_id: id(7),
            },
          ],
        },
        { type: "SetNodeParents", parents: [{ id: id(4), parent_id: null }] },
        { type: "SetViewport", viewport: { x: 2, y: 3, zoom: 0.05 } },
      ],
    },
  });
  expect(JSON.stringify(request.data)).not.toContain("untrusted");
});
it("文档重命名与删除revision沿公共生成客户端传递", async () => {
  const send = vi
    .spyOn(axios, "request")
    .mockResolvedValueOnce({ data: payload() })
    .mockResolvedValueOnce({ data: { id: id(1), revision: 3, deleted: true } });
  await renameCanvas(id(1), 1, "新名称", "rename-key");
  await deleteCanvas(id(1), 2, "delete-key");
  expect(send.mock.calls[0][0]).toMatchObject({
    method: "PATCH",
    data: { expected_revision: 1, name: "新名称" },
  });
  expect(send.mock.calls[1][0]).toMatchObject({
    method: "DELETE",
    data: { expected_revision: 2 },
  });
});
it("网络与修订冲突按统一problem解析，不伪造成功文档", async () => {
  vi.spyOn(axios, "request").mockRejectedValue(
    new AxiosError("private", undefined, undefined, undefined, {
      status: 409,
      statusText: "Conflict",
      headers: {},
      config: {} as never,
      data: {
        code: "revision_conflict",
        request_id: "synthetic-request",
        meta: { current_revision: 8 },
      },
    }),
  );
  await expect(
    saveCanvasCommands(
      id(1),
      1,
      [{ type: "SetViewport", viewport: { x: 0, y: 0, k: 1 } }],
      "key",
    ),
  ).rejects.toMatchObject({
    status: 409,
    code: "revision_conflict",
    requestId: "synthetic-request",
    meta: { current_revision: 8 },
  });
});
it("授权media preview走独立查询，短签URL不成为node config事实", async () => {
  const data = {
    asset: {
      id: id(5),
      project_id: id(2),
      kind: "image",
      file_name: "asset.png",
      mime_type: "image/png",
      byte_size: 128,
      revision: 1,
    },
    url: "https://objects.example.test/asset?signature=synthetic",
    expires_at: "2026-09-30T08:00:00Z",
  };
  const send = vi.spyOn(axios, "request").mockResolvedValue({ data });
  expect(await getMediaPreview(id(2), id(5))).toEqual(data);
  expect(send.mock.calls[0][0]).toMatchObject({
    url: `/api/projects/${id(2)}/media/${id(5)}/preview`,
    method: "GET",
  });
});
