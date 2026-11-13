import { beforeEach, expect, it, vi } from "vitest";
import {
  applyCanvasOperationResult,
  canvasOperationTarget,
} from "@/components/canvas/lib/canvas-operation-result";
import type { CanvasYjsLiveSession } from "@/components/canvas/yjs-live-session";
import type { CanvasNodeData } from "@/components/canvas/types/canvas";
import type { Task } from "@/components/generation/contracts";
const preview = vi.hoisted(() => vi.fn());
vi.mock("@/api/media", () => ({ getMediaPreview: preview }));
const pid = "11111111-1111-4111-8111-111111111111",
  cid = "22222222-2222-4222-8222-222222222222",
  nid = "33333333-3333-4333-8333-333333333333",
  aid = "44444444-4444-4444-8444-444444444444";
const task: Task = {
  id: "55555555-5555-4555-8555-555555555555",
  project_id: pid,
  status: "completed",
  target_content_version: "5",
  source: { canvas_id: cid, node_id: nid },
  outputs: [
    { kind: "image", moderation_status: "passed", media_asset_id: aid },
  ],
};
function setup(existing = false) {
  const state = {
    writable: true,
    migration: false,
    document: {
      nodes: [
        {
          id: nid,
          type: "image",
          title: "图片",
          width: 320,
          height: 320,
          position: { x: 0, y: 0 },
          documentData: {
            fileReferences: existing
              ? [
                  {
                    purpose: "CONTENT",
                    fileId: "old",
                    version: "1",
                    referenceKey: "old",
                  },
                ]
              : [],
          },
          serviceData: { input_revision: 2, content_version: "5" },
        } as CanvasNodeData,
      ],
      connections: [],
      backgroundMode: "dots",
      showImageInfo: false,
      viewport: { x: 0, y: 0, k: 1 },
    },
  };
  const change = vi.fn((next) => {
    state.document = next;
  });
  const client = {
    getSnapshot: () => state,
    getDocumentEpoch: () => "epoch",
    canAccept: () => true,
    change,
    waitForSaved: async (check: () => void) => check(),
  } as unknown as CanvasYjsLiveSession;
  return { state, client, change };
}
beforeEach(() => {
  preview.mockReset().mockResolvedValue({
    asset: {
      id: aid,
      project_id: pid,
      kind: "image",
      revision: 1,
      width: 720,
      height: 540,
    },
    url: "https://example.com/result.png",
  });
});
it("已审核结果直接写回原节点，并采用结果宽高比", async () => {
  const { client, state, change } = setup();
  await applyCanvasOperationResult(client, pid, cid, nid, task);
  expect(change).toHaveBeenCalledOnce();
  expect(state.document.nodes[0].height).toBe(320);
  expect(state.document.nodes[0].width).toBeCloseTo(426.666);
  expect(state.document.nodes[0].id).toBe(nid);
  expect(
    state.document.nodes[0].width / state.document.nodes[0].height,
  ).toBeCloseTo(4 / 3);
  expect(state.document.nodes[0].documentData!.fileReferences![0].fileId).toBe(
    aid,
  );
  await applyCanvasOperationResult(client, pid, cid, nid, task);
  expect(change).toHaveBeenCalledOnce();
});
it("本次提交可替换已有结果，刷新恢复的任务不会覆盖现有素材", async () => {
  const { client, change } = setup(true);
  await expect(
    applyCanvasOperationResult(client, pid, cid, nid, {
      ...task,
      target_content_version: "4",
    }),
  ).resolves.toBe("superseded");
  expect(change).not.toHaveBeenCalled();
  await applyCanvasOperationResult(
    client,
    pid,
    cid,
    nid,
    task,
    canvasOperationTarget(client, nid),
  );
  expect(change).toHaveBeenCalledOnce();
});
it.each(["输入", "素材", "删除", "只读"])(
  "查询结果期间%s变化不会被覆盖",
  async (kind) => {
    const { client, state, change } = setup();
    let done!: (v: unknown) => void;
    preview.mockReturnValue(
      new Promise((r) => {
        done = r;
      }),
    );
    const pending = applyCanvasOperationResult(
      client,
      pid,
      cid,
      nid,
      task,
      canvasOperationTarget(client, nid),
    );
    if (kind === "输入")
      state.document.nodes[0].serviceData!.input_revision = 3;
    if (kind === "素材")
      state.document.nodes[0].documentData!.fileReferences = [
        {
          purpose: "CONTENT",
          fileId: "changed",
          version: "2",
          referenceKey: "new",
        },
      ];
    if (kind === "删除") state.document.nodes = [];
    if (kind === "只读") state.writable = false;
    done({
      asset: { id: aid, project_id: pid, kind: "image", revision: 1 },
      url: "https://example.com/result.png",
    });
    await expect(pending).rejects.toThrow();
    expect(change).not.toHaveBeenCalled();
  },
);
it("跨项目、跨画布及待审核结果禁止写回", async () => {
  const { client, change } = setup();
  await expect(
    applyCanvasOperationResult(client, pid, "other", nid, task),
  ).rejects.toThrow("来源不匹配");
  await expect(
    applyCanvasOperationResult(client, pid, cid, nid, {
      ...task,
      outputs: [
        { kind: "image", moderation_status: "pending", media_asset_id: aid },
      ],
    }),
  ).rejects.toThrow("尚不可用");
  expect(change).not.toHaveBeenCalled();
});

it("文字生成在原节点显示正文，旧文本列表不覆盖新结果", async () => {
  const { client, state, change } = setup();
  state.document.nodes[0].type = "text";
  state.document.nodes[0].metadata = {
    content: "旧正文",
    texts: [{ id: "old", status: "success", content: "旧正文" }],
  };
  await applyCanvasOperationResult(
    client,
    pid,
    cid,
    nid,
    {
      ...task,
      outputs: [
        { kind: "json", moderation_status: "passed", text: "新的正文" },
      ],
    },
    canvasOperationTarget(client, nid),
  );
  expect(change).toHaveBeenCalledOnce();
  expect(state.document.nodes[0].metadata).toMatchObject({
    content: "新的正文",
    texts: undefined,
  });
  expect(preview).not.toHaveBeenCalled();
});

it("本次结果写入的服务端修订确认不会被误报为并发覆盖", async () => {
  const { client, state, change } = setup();
  client.waitForSaved = async (check) => {
    state.document.nodes[0].serviceData!.input_revision! += 1;
    check?.();
    return { id: cid, project_id: pid, revision: "2", nodes: [], edges: [] };
  };
  await expect(
    applyCanvasOperationResult(
      client,
      pid,
      cid,
      nid,
      task,
      canvasOperationTarget(client, nid),
    ),
  ).resolves.toBeUndefined();
  expect(change).toHaveBeenCalledOnce();
});

it("批量图片按序一次写入，任一素材无权访问时不写入部分结果", async () => {
  const second = "77777777-7777-4777-8777-777777777777";
  const batch = {
    ...task,
    outputs: [
      { ...task.outputs![0], id: second, media_asset_id: second, sequence: 2 },
      { ...task.outputs![0], id: aid, sequence: 1 },
    ],
  };
  const { client, state, change } = setup();
  await expect(
    applyCanvasOperationResult(client, pid, cid, nid, batch),
  ).rejects.toThrow("生成素材不可用");
  expect(change).not.toHaveBeenCalled();
  preview.mockImplementation(async ({ asset_id }) => ({
    asset: {
      id: asset_id,
      project_id: pid,
      kind: "image",
      revision: 2,
      width: 720,
      height: 540,
    },
    url: "/media/" + asset_id,
  }));
  await applyCanvasOperationResult(client, pid, cid, nid, batch);
  expect(
    state.document.nodes[0].documentData?.fileReferences?.map((r) => r.fileId),
  ).toEqual([aid, second]);
  expect(
    state.document.nodes[0].metadata?.images?.map((i) => i.content),
  ).toEqual(["/media/" + aid, "/media/" + second]);
  expect(change).toHaveBeenCalledOnce();
});
