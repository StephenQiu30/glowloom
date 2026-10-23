import { afterEach, expect, it, vi } from "vitest";
import {
  publishDerivedNodes,
  type DerivedReceipt,
} from "@/components/canvas/media-tools/publish-derived";
import { uploadMediaAsset } from "@/api/media";
import type { CanvasNodeData } from "@/components/canvas/types";
vi.mock("@/api/media", () => ({ uploadMediaAsset: vi.fn() }));
afterEach(() => vi.resetAllMocks());
const pid = "766d1bcc-b2ef-4ab5-b449-dd02d72057e2";
const assetA = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
  assetB = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb";
const nodes: CanvasNodeData[] = ["a", "b"].map((id) => ({
  id,
  type: "image",
  title: id,
  position: { x: 0, y: 0 },
  width: 10,
  height: 10,
  metadata: { content: "data:image/png;base64,aGVsbG8=" },
}));
it("部分上传失败及图提交重试复用原请求身份和成功素材，不重复上传", async () => {
  const receipts = new Map<string, DerivedReceipt>();
  const signal = new AbortController().signal;
  const api = vi.mocked(uploadMediaAsset);
  api
    .mockResolvedValueOnce({
      asset: { id: assetA, project_id: pid, kind: "image" },
    })
    .mockRejectedValueOnce(new Error("network"));
  await expect(
    publishDerivedNodes(nodes, pid, receipts, signal),
  ).rejects.toThrow("network");
  const retryKey = api.mock.calls[1][3]?.headers?.["Idempotency-Key"];
  api.mockResolvedValueOnce({
    asset: { id: assetB, project_id: pid, kind: "image" },
  });
  const result = await publishDerivedNodes(nodes, pid, receipts, signal);
  expect(api).toHaveBeenCalledTimes(3);
  expect(api.mock.calls[2][3]?.headers?.["Idempotency-Key"]).toBe(retryKey);
  expect(result.map((n) => n.metadata?.assetId)).toEqual([assetA, assetB]);
  expect(result.every((n) => n.metadata?.content === "")).toBe(true);
  expect(await publishDerivedNodes(nodes, pid, receipts, signal)).toEqual(
    result,
  );
  expect(api).toHaveBeenCalledTimes(3);
});
it("跨项目回执拒绝，取消后的迟到成功仅保留回执不进入图", async () => {
  const receipts = new Map<string, DerivedReceipt>();
  vi.mocked(uploadMediaAsset).mockResolvedValueOnce({
    asset: { id: assetA, project_id: "other", kind: "image" },
  });
  await expect(
    publishDerivedNodes(nodes, pid, receipts, new AbortController().signal),
  ).rejects.toThrow("回执无效");
  expect(receipts.get("a")?.assetID).toBeUndefined();
  const abort = new AbortController();
  vi.mocked(uploadMediaAsset).mockImplementationOnce(async () => {
    abort.abort();
    return { asset: { id: assetA, project_id: pid, kind: "image" } };
  });
  await expect(
    publishDerivedNodes(nodes, pid, receipts, abort.signal),
  ).rejects.toThrow();
  expect(receipts.get("a")?.assetID).toBe(assetA);
  expect(receipts.has("b")).toBe(false);
});
