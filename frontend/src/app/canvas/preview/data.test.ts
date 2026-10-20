import { beforeEach, expect, it, vi } from "vitest";
import {
  CanvasPreviewData,
  applyPreviewCommands,
  initialPreviewState,
  previewUploadFingerprint,
} from "./data";
import {
  commandBatch,
  requireSaveResult,
  createResourceNode,
  createMediaNode,
  isEditableNode,
} from "../document";
beforeEach(() => localStorage.clear());
it("Mock保存通过正式命令与回执验证，刷新保留编辑与原幂等回执", async () => {
  const state = initialPreviewState(0),
    data = new CanvasPreviewData(state);
  const nodes = [createResourceNode("text", { x: 80, y: 160 })];
  nodes[0].config.text = "中文 Mock 保存";
  const body = commandBatch(state.canvas, nodes);
  const url = `/api/canvases/${state.canvas.id}/commands`;
  const options = {
    method: "POST",
    data: body,
    headers: { "Idempotency-Key": crypto.randomUUID() },
  };
  const result = await data.request(url, options);
  expect(
    requireSaveResult(
      result,
      { ...state.canvas, revision: 1, nodes: [] },
      nodes,
      body,
    ).nodes[0].config.text,
  ).toBe("中文 Mock 保存");
  const reopened = CanvasPreviewData.load();
  expect(reopened.state.canvas.nodes[0].config.text).toBe("中文 Mock 保存");
  expect(await reopened.request(url, options)).toEqual(result);
  await expect(
    reopened.request(url, { ...options, data: { ...body, commands: [] } }),
  ).rejects.toThrow("idempotency_conflict");
});
it("Mock上传回执刷新后同键同体重放，异体拒绝", async () => {
  const state = initialPreviewState();
  const file = new File(["fixture"], "test.png", { type: "image/png" });
  Object.defineProperty(file, "arrayBuffer", {
    value: async () => new TextEncoder().encode("fixture").buffer,
  });
  state.uploads = [
    {
      key: "upload-key",
      fingerprint: await previewUploadFingerprint(file),
      asset: state.assets[0].asset,
    },
  ];
  const data = new CanvasPreviewData(state);
  data.persist();
  const form = new FormData();
  form.set("file", file);
  const options = {
    method: "POST",
    data: form,
    headers: { "Idempotency-Key": "upload-key" },
  };
  expect(
    await CanvasPreviewData.load().request(
      `/api/projects/${state.canvas.project_id}/media/uploads`,
      options,
    ),
  ).toEqual({ asset: state.assets[0].asset });
  const changed = new File(["changed"], "test.png", { type: "image/png" });
  Object.defineProperty(changed, "arrayBuffer", {
    value: async () => new TextEncoder().encode("changed").buffer,
  });
  form.set("file", changed);
  await expect(
    data.request(
      `/api/projects/${state.canvas.project_id}/media/uploads`,
      options,
    ),
  ).rejects.toThrow("idempotency_conflict");
});
it("Mock移动、分组与相机回执沿用正式校验；过期修订明确冲突", () => {
  const state = initialPreviewState(),
    base = structuredClone(state.canvas);
  const group = {
    ...createResourceNode("group", { x: 0, y: 0 }),
    width: 1000,
    height: 800,
  };
  const nodes = [
    ...base.nodes.map((node, i) => ({
      ...node,
      x: node.x + 20,
      ...(i < 2 ? { parent_id: group.id } : {}),
    })),
    group,
  ];
  const body = commandBatch(base, nodes, {
    viewport: { x: 50, y: 80, zoom: 0.5 },
  });
  const result = applyPreviewCommands(base, body);
  expect(requireSaveResult(result, base, nodes, body).viewport?.zoom).toBe(0.5);
  expect(() =>
    applyPreviewCommands(base, { ...body, expected_revision: 99 }),
  ).toThrow("revision_conflict");
});
it("未提供的Mock接口明确拒绝，不回落到真实业务接口", async () => {
  const data = new CanvasPreviewData(initialPreviewState());
  await expect(
    data.request("/api/platform/users", { method: "POST" }),
  ).rejects.toThrow("Mock预览尚未接入此操作");
});
it("Mock新增媒体的回执保持可编辑，等同JSON接口清除undefined字段", () => {
  const state = initialPreviewState(0);
  const node = createMediaNode(
    state.assets[0].asset,
    state.canvas.project_id,
    0,
  );
  const body = commandBatch(state.canvas, [node]);
  const result = applyPreviewCommands(state.canvas, body);
  expect(isEditableNode(result.nodes[0])).toBe(true);
});
it("Mock持久化失败不能被同键重试误报为已保存", async () => {
  const state = initialPreviewState(0),
    data = new CanvasPreviewData(state);
  data.persist();
  const before = localStorage.getItem("glowloom.canvas-preview.v1");
  const body = commandBatch(state.canvas, [
    createResourceNode("text", { x: 0, y: 0 }),
  ]);
  const options = {
    method: "POST",
    data: body,
    headers: { "Idempotency-Key": "retry" },
  };
  const url = `/api/canvases/${state.canvas.id}/commands`;
  const write = vi
    .spyOn(Storage.prototype, "setItem")
    .mockImplementationOnce(() => {
      throw new DOMException("full", "QuotaExceededError");
    });
  await expect(data.request(url, options)).rejects.toThrow("full");
  expect(localStorage.getItem("glowloom.canvas-preview.v1")).toBe(before);
  expect(data.state.canvas.revision).toBe(1);
  await data.request(url, options);
  expect(CanvasPreviewData.load().state.canvas.nodes).toHaveLength(1);
  write.mockRestore();
});
