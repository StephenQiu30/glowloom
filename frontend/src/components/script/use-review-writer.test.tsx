import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { useReviewWriter } from "./use-review-writer";
import { getWorkspace } from "./source-queries";
import { runReviewIntent } from "./review-queries";
vi.mock("./source-queries", async (original) => ({
  ...(await original<typeof import("./source-queries")>()),
  getWorkspace: vi.fn(),
}));
vi.mock("./review-queries", () => ({ runReviewIntent: vi.fn() }));
const id = (n: number) =>
  `${String(n).repeat(8)}-${String(n).repeat(4)}-4${String(n).repeat(3)}-8${String(n).repeat(3)}-${String(n).repeat(12)}`;
const scope = {
  origin: window.location.origin,
  actorId: id(1),
  orgId: id(2),
  projectId: id(3),
};
const command = {
  action: "resplit" as const,
  body: {
    version_id: id(4),
    expected_revision: 2,
    expected_split_revision: 1,
    candidate_set_id: id(5),
    ack_invalidate: false,
  },
};
beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  vi.mocked(getWorkspace).mockResolvedValue({
    current_actor_id: scope.actorId,
    current_org_id: scope.orgId,
    state: {
      project_id: scope.projectId,
      org_id: scope.orgId,
      revision: 2,
      draft_version_id: id(4),
      updated_at: "2026-10-02T00:00:00Z",
    },
  });
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
it("审核未知跨刷新只使用原键原正文；读新事实或授权拒绝不自动改原CAS", async () => {
  vi.mocked(runReviewIntent).mockRejectedValueOnce(
    new ApiError(0, "network_error"),
  );
  const hook = renderHook(() => useReviewWriter(scope, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  await act(() => hook.result.current.submit(command));
  const original = hook.result.current.intent!;
  hook.unmount();
  const next = renderHook(() => useReviewWriter(scope, vi.fn()));
  await waitFor(() =>
    expect(next.result.current.intent?.key).toBe(original.key),
  );
  expect(runReviewIntent).toHaveBeenCalledTimes(1);
  vi.mocked(runReviewIntent).mockRejectedValueOnce(
    new ApiError(403, "forbidden"),
  );
  await act(() => next.result.current.replay());
  expect(next.result.current.intent).toEqual(original);
  expect(runReviewIntent).toHaveBeenLastCalledWith(original);
});
it("quota阻止发送；已确定409保留原草稿，只在人工审阅后允许新意图", async () => {
  const hook = renderHook(() => useReviewWriter(scope, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  const set = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new DOMException("quota", "QuotaExceededError");
  });
  await act(() => hook.result.current.submit(command));
  expect(runReviewIntent).not.toHaveBeenCalled();
  expect(hook.result.current.intent?.body).toEqual(command.body);
  set.mockRestore();
  await act(() => hook.result.current.restoreStorage());
  vi.mocked(runReviewIntent).mockRejectedValueOnce(
    new ApiError(409, "revision_conflict"),
  );
  await act(() => hook.result.current.replay());
  expect(hook.result.current.rejected?.intent.body).toEqual(command.body);
  expect(hook.result.current.intent).toBeNull();
  await act(() => hook.result.current.submit(command));
  expect(runReviewIntent).toHaveBeenCalledTimes(1);
  act(() => hook.result.current.acknowledgeLatest());
  vi.mocked(runReviewIntent).mockRejectedValueOnce(
    new ApiError(422, "bad_boundaries"),
  );
  await act(() =>
    hook.result.current.submit({
      ...command,
      body: { ...command.body, expected_revision: 9 },
    }),
  );
  expect(runReviewIntent).toHaveBeenCalledTimes(2);
  expect(hook.result.current.intent).toBeNull();
});
