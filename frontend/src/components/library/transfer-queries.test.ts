import { beforeEach, expect, it, vi } from "vitest";
import * as api from "@/api/media";
import { freshLibrary } from "@/components/media/library-queries";
import {
  listTransfers,
  getTransfer,
  runTransferIntent,
} from "./transfer-queries";
import { transferControl } from "./transfer-model";
import {
  transferIDs,
  transferIdentity,
  transferInput,
  transferJob,
  transferLibrary,
} from "./transfer-test-fixtures";
vi.mock("@/api/media", () => ({
  listMediaTransfers: vi.fn(),
  getMediaTransfer: vi.fn(),
  createMediaTransfer: vi.fn(),
  cancelMediaTransfer: vi.fn(),
  retryMediaTransfer: vi.fn(),
  reconcileMediaTransfer: vi.fn(),
}));
vi.mock("@/components/media/library-queries", () => ({
  freshLibrary: vi.fn(),
}));
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(freshLibrary).mockResolvedValue(transferLibrary);
});
it("仅生成SDK读取精确scope分页/abort，拒私有字段及当前主体变化", async () => {
  const signal = new AbortController().signal;
  vi.mocked(api.listMediaTransfers).mockResolvedValue({
    current_actor_id: transferIDs.actor,
    current_org_id: transferIDs.org,
    page: 2,
    page_size: 20,
    items: [transferJob],
  });
  await listTransfers(transferIdentity, 2, signal);
  expect(api.listMediaTransfers).toHaveBeenCalledWith(
    { scope: "personal", page: 2, page_size: 20 },
    { signal },
  );
  vi.mocked(api.getMediaTransfer).mockResolvedValue({
    ...transferJob,
    current_org_id: transferIDs.project,
  });
  await expect(getTransfer(transferIdentity, transferJob.id)).rejects.toThrow();
});
it("创建只送原完整输入，当前身份撤销零POST，202不作为当前缓存", async () => {
  const original = {
    ...transferIdentity,
    version: 1 as const,
    key: transferIDs.project,
    action: "create" as const,
    body: transferInput,
  };
  vi.mocked(api.createMediaTransfer).mockResolvedValue(transferJob);
  expect((await runTransferIntent(original)).id).toBe(transferJob.id);
  expect(api.createMediaTransfer).toHaveBeenCalledWith(transferInput, {
    headers: { "Idempotency-Key": original.key, Origin: original.origin },
  });
  vi.mocked(freshLibrary).mockRejectedValue(new Error("revoked"));
  await expect(runTransferIntent(original)).rejects.toThrow("revoked");
  expect(api.createMediaTransfer).toHaveBeenCalledTimes(1);
});
it.each(["cancel", "retry", "reconcile"] as const)(
  "%s同job/原attempt/原revision，SDK不注入第三种scope",
  async (action) => {
    const original = {
      ...transferIdentity,
      version: 1 as const,
      key: transferIDs.project,
      ...transferControl(transferJob, action),
    };
    const result = {
      ...transferJob,
      revision: 2,
      ...(action === "cancel"
        ? {
            status: "cancelled" as const,
            stage: "completed" as const,
            cancellation_requested: true,
          }
        : { attempt: 2 }),
    };
    const endpoint =
      action === "cancel"
        ? api.cancelMediaTransfer
        : action === "retry"
          ? api.retryMediaTransfer
          : api.reconcileMediaTransfer;
    vi.mocked(endpoint).mockResolvedValue(result);
    await runTransferIntent(original);
    expect(endpoint).toHaveBeenCalledWith(
      { job_id: transferJob.id },
      { revision: 1 },
      { headers: { "Idempotency-Key": original.key, Origin: original.origin } },
    );
  },
);
