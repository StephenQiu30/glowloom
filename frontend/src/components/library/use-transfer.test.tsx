import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { freshLibrary } from "@/components/media/library-queries";
import { useTransfer } from "./use-transfer";
import { listTransfers, runTransferIntent } from "./transfer-queries";
import {
  transferIDs,
  transferIdentity,
  transferInput,
  transferJob,
  transferLibrary,
} from "./transfer-test-fixtures";
vi.mock("./transfer-queries", () => ({
  listTransfers: vi.fn(),
  runTransferIntent: vi.fn(),
}));
vi.mock("@/components/media/library-queries", () => ({
  freshLibrary: vi.fn(),
}));
beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  vi.mocked(freshLibrary).mockResolvedValue(transferLibrary);
  vi.mocked(listTransfers).mockResolvedValue({
    current_actor_id: transferIDs.actor,
    current_org_id: transferIDs.org,
    page: 1,
    page_size: 20,
    items: [],
  });
});
afterEach(cleanup);
it("unknown跨刷新保留同键正文、不自动写；撤权重放不换原键", async () => {
  vi.mocked(runTransferIntent).mockRejectedValue(new ApiError(502, "unknown"));
  const first = renderHook(() => useTransfer(transferIdentity, vi.fn()));
  await waitFor(() => expect(first.result.current.ready).toBe(true));
  await act(() =>
    first.result.current.submit({ action: "create", body: transferInput }),
  );
  const original = first.result.current.intent!;
  first.unmount();
  const second = renderHook(() => useTransfer(transferIdentity, vi.fn()));
  await waitFor(() =>
    expect(second.result.current.intent?.key).toBe(original.key),
  );
  expect(runTransferIntent).toHaveBeenCalledTimes(1);
  vi.mocked(runTransferIntent).mockRejectedValueOnce(
    new ApiError(403, "forbidden"),
  );
  await act(() => second.result.current.replay());
  expect(runTransferIntent).toHaveBeenLastCalledWith(original);
  expect(second.result.current.intent).toEqual(original);
  expect(second.result.current.rejected).toBe(false);
});
it("storage失败零DML，409保选择；读最新前不能释放或造新键", async () => {
  const hook = renderHook(() => useTransfer(transferIdentity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  const spy = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("quota");
  });
  await act(() =>
    hook.result.current.submit({ action: "create", body: transferInput }),
  );
  expect(runTransferIntent).not.toHaveBeenCalled();
  const key = hook.result.current.intent!.key;
  spy.mockRestore();
  await act(() => hook.result.current.restoreStorage());
  vi.mocked(runTransferIntent).mockRejectedValueOnce(
    new ApiError(409, "media_transfer_conflict"),
  );
  await act(() => hook.result.current.replay());
  expect(hook.result.current.rejected).toBe(true);
  await act(() => hook.result.current.releaseRejected());
  expect(hook.result.current.intent?.key).toBe(key);
  await act(() => hook.result.current.readLatest());
  await act(() => hook.result.current.releaseRejected());
  expect(hook.result.current.intent).toBeNull();
});
it("已受理只回传原job并清journal，不自动重发、回调读失败不撤销已受理", async () => {
  const accepted = vi.fn().mockRejectedValue(new Error("readfailed"));
  vi.mocked(runTransferIntent).mockResolvedValue(transferJob);
  const hook = renderHook(() => useTransfer(transferIdentity, accepted));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  await act(() =>
    hook.result.current.submit({ action: "create", body: transferInput }),
  );
  expect(accepted).toHaveBeenCalledWith(transferJob);
  expect(hook.result.current.intent).toBeNull();
  expect(hook.result.current.error).toMatch("已受理");
  expect(runTransferIntent).toHaveBeenCalledTimes(1);
});
