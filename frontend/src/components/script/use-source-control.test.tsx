import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { useSourceControl } from "./use-source-control";
import { listSourceWrites, runSourceControl } from "./review-queries";
vi.mock("./review-queries", () => ({
  listSourceWrites: vi.fn(),
  runSourceControl: vi.fn(),
}));
const scope = {
  origin: window.location.origin,
  projectId: "11111111-1111-4111-8111-111111111111",
  actorId: "22222222-2222-4222-8222-222222222222",
  orgId: "33333333-3333-4333-8333-333333333333",
};
const command = {
  intentId: "44444444-4444-4444-8444-444444444444",
  action: "reconcile" as const,
  body: { expected_revision: 4 },
};
beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  vi.mocked(listSourceWrites).mockResolvedValue({
    current_actor_id: scope.actorId,
    current_org_id: scope.orgId,
    items: [],
  });
});
afterEach(cleanup);
it("未知202控制刷新仅原键原CAS人工核验，403重放保原意图而不称停止", async () => {
  vi.mocked(runSourceControl).mockRejectedValueOnce(
    new ApiError(0, "network_error"),
  );
  const first = renderHook(() => useSourceControl(scope, vi.fn()));
  await waitFor(() => expect(first.result.current.ready).toBe(true));
  await act(() => first.result.current.submit(command));
  const original = first.result.current.intent!;
  first.unmount();
  const second = renderHook(() => useSourceControl(scope, vi.fn()));
  await waitFor(() =>
    expect(second.result.current.intent?.key).toBe(original.key),
  );
  vi.mocked(runSourceControl).mockRejectedValueOnce(
    new ApiError(403, "forbidden"),
  );
  await act(() => second.result.current.replay());
  expect(second.result.current.intent).toEqual(original);
  expect(runSourceControl).toHaveBeenLastCalledWith(
    scope,
    {
      intentId: original.intentId,
      action: original.action,
      body: original.body,
    },
    original.key,
  );
  expect(second.result.current.locked).toBe(true);
});
it("scope变更不发送控制；409由人工读取最新后解开新意图", async () => {
  const hook = renderHook(() => useSourceControl(scope, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  vi.mocked(runSourceControl).mockRejectedValueOnce(
    new ApiError(409, "stale_revision"),
  );
  await act(() => hook.result.current.submit(command));
  expect(hook.result.current.conflicted).toBe(true);
  await act(() =>
    hook.result.current.submit({ ...command, body: { expected_revision: 9 } }),
  );
  expect(runSourceControl).toHaveBeenCalledTimes(1);
  act(() => hook.result.current.acknowledgeLatest());
  vi.mocked(listSourceWrites).mockResolvedValueOnce({
    current_actor_id: command.intentId,
    current_org_id: scope.orgId,
    items: [],
  });
  await act(() => hook.result.current.submit(command));
  expect(runSourceControl).toHaveBeenCalledTimes(1);
  expect(hook.result.current.intent).toBeTruthy();
});
it("存储配额不足零控制写入，恢复仅持久原键，另人工核验才发送", async () => {
  const hook = renderHook(() => useSourceControl(scope, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  const spy = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new DOMException("quota", "QuotaExceededError");
  });
  try {
    await act(() => hook.result.current.submit(command));
    expect(runSourceControl).not.toHaveBeenCalled();
    expect(hook.result.current.intent?.body).toEqual(command.body);
  } finally {
    spy.mockRestore();
  }
  const key = hook.result.current.intent?.key;
  await act(() => hook.result.current.restoreStorage());
  expect(hook.result.current.intent?.key).toBe(key);
  expect(runSourceControl).not.toHaveBeenCalled();
});
