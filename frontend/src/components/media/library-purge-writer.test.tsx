import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import * as queries from "./library-purge-query";
import * as library from "./library-queries";
import { usePurgeWriter } from "./library-purge-writer";
import { savePurgeIntent, loadPurgeIntent } from "./library-purge-intent";
import { createPurgePlan, savePurgePlan } from "./library-purge-plan";
import { loadPurgePlan } from "./library-purge-plan";
import {
  purgeTestIdentity as identity,
  purgeTestInput as body,
  purgeTestJob as job,
  purgeTestReview as review,
} from "./library-purge-fixtures";
import {
  transferLibrary,
  transferIDs,
} from "@/components/library/transfer-test-fixtures";
vi.mock("./library-queries", () => ({ freshLibrary: vi.fn() }));
vi.mock("./library-purge-query", () => ({
  runLibraryPurge: vi.fn(),
  getLibraryPurge: vi.fn(),
  reviewLibraryPurge: vi.fn(),
}));
beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  vi.mocked(library.freshLibrary).mockResolvedValue({
    ...transferLibrary,
    revision: 4,
  });
  vi.mocked(queries.runLibraryPurge).mockResolvedValue(job);
  vi.mocked(queries.reviewLibraryPurge).mockResolvedValue({
    ...review,
    mode: "all",
  });
});
afterEach(cleanup);
it("整体恢复JSON超过1MiB时保留原计划和原键并停止，不能分别按1MiB受理", async () => {
  const items = Array.from({ length: 200 }, (_, i) => ({
    id: `10000000-0000-4000-8000-${String(i + 1).padStart(12, "0")}`,
    revision: 2,
    title: `条目${i}`,
  }));
  const plan = createPurgePlan(identity, { ...review, mode: "all", items });
  plan.batches[0].input = {
    ...body,
    items: items.map(({ id, revision }) => ({ id, revision })),
  };
  plan.batches[0].job = {
    ...job,
    items: items.map((item, index) => ({
      ...job.items[0],
      index,
      item_id: item.id,
    })),
  };
  const padding = 1024 * 1024 - JSON.stringify(plan).length - 1000;
  plan.batches[0].job.created_at = `2026-10-02T00:00:00.${"0".repeat(padding)}Z`;
  savePurgePlan(sessionStorage, plan, null);
  const original = {
    ...identity,
    version: 1 as const,
    action: "create" as const,
    key: plan.batches[0].key,
    body: plan.batches[0].input,
  };
  savePurgeIntent(sessionStorage, original);
  const stored = { ...sessionStorage };
  const hook = renderHook(() => usePurgeWriter(identity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  expect(hook.result.current.storageError).toMatch(/1MiB/);
  expect(queries.runLibraryPurge).not.toHaveBeenCalled();
  expect({ ...sessionStorage }).toEqual(stored);
});
it("一次确认201项在前台串行完成200+1，首批实际终态核验前不受理第二批", async () => {
  const items = Array.from({ length: 201 }, (_, i) => ({
    id: `10000000-0000-4000-8000-${String(i + 1).padStart(12, "0")}`,
    revision: 2,
    title: `条目${i}`,
  }));
  const firstJob = {
    ...job,
    status: "succeeded" as const,
    stage: "completed" as const,
    items: items.slice(0, 200).map((item, index) => ({
      index,
      item_id: item.id,
      asset_id: null,
      status: "succeeded" as const,
      failure_code: null,
    })),
  };
  const secondJob = {
    ...firstJob,
    id: transferIDs.target,
    items: [{ ...firstJob.items[0], index: 0, item_id: items[200].id }],
  };
  vi.mocked(queries.reviewLibraryPurge)
    .mockResolvedValueOnce({ ...review, mode: "all", items })
    .mockResolvedValueOnce({
      ...review,
      mode: "all",
      revision: 205,
      items: items.slice(200),
    })
    .mockResolvedValue({ ...review, mode: "all", revision: 207, items: [] });
  vi.mocked(queries.runLibraryPurge)
    .mockResolvedValueOnce(firstJob)
    .mockResolvedValueOnce(secondJob);
  vi.mocked(queries.getLibraryPurge).mockImplementation(async (_, id) =>
    id === firstJob.id ? firstJob : secondJob,
  );
  const hook = renderHook(() => usePurgeWriter(identity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  await act(() =>
    hook.result.current.startPlan({ ...review, mode: "all", items }),
  );
  expect(queries.runLibraryPurge).toHaveBeenCalledTimes(2);
  expect(hook.result.current.planComplete).toBe(true);
  expect(
    vi.mocked(queries.runLibraryPurge).mock.calls[1][0].body,
  ).toMatchObject({
    items: [{ id: items[200].id, revision: 2 }],
    expected_revision: 205,
  });
});
it("未知写不自动重发，原key/body/scope刷新后原样恢复，明确replay才再次送出", async () => {
  vi.mocked(queries.runLibraryPurge).mockRejectedValueOnce(
    new ApiError(0, "network"),
  );
  const hook = renderHook(() => usePurgeWriter(identity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  await act(() => hook.result.current.submit({ action: "create", body }));
  const original = loadPurgeIntent(sessionStorage, identity)!;
  expect(original.body).toEqual(body);
  expect(queries.runLibraryPurge).toHaveBeenCalledTimes(1);
  hook.unmount();
  const onAccepted = vi.fn(),
    restored = renderHook(() => usePurgeWriter(identity, onAccepted));
  await waitFor(() => expect(restored.result.current.intent).toEqual(original));
  expect(queries.runLibraryPurge).toHaveBeenCalledTimes(1);
  await act(() => restored.result.current.replay());
  expect(queries.runLibraryPurge).toHaveBeenLastCalledWith(original, undefined);
  expect(loadPurgeIntent(sessionStorage, identity)).toBeNull();
  expect(onAccepted).toHaveBeenCalledWith(job);
});
it("403恢复拒绝仍保留原意图，409明确拒绝不能直接重提或自动替换CAS", async () => {
  savePurgeIntent(sessionStorage, {
    ...identity,
    version: 1,
    key: transferIDs.target,
    action: "create",
    body,
  });
  const hook = renderHook(() => usePurgeWriter(identity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  vi.mocked(queries.runLibraryPurge).mockRejectedValueOnce(
    new ApiError(403, "denied"),
  );
  await act(() => hook.result.current.replay());
  expect(hook.result.current.rejected).toBe(false);
  expect(hook.result.current.intent?.key).toBe(transferIDs.target);
  vi.mocked(queries.runLibraryPurge).mockRejectedValueOnce(
    new ApiError(409, "media_purge_conflict"),
  );
  await act(() => hook.result.current.replay());
  expect(hook.result.current.rejected).toBe(true);
  await act(() =>
    hook.result.current.submit({
      action: "create",
      body: { ...body, expected_revision: 5 },
    }),
  );
  expect(queries.runLibraryPurge).toHaveBeenCalledTimes(2);
});
it("损坏或无法durable存储时不发送，新身份读取不恢复他人的原键", async () => {
  const spy = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("unavailable");
  });
  const hook = renderHook(() => usePurgeWriter(identity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  await act(() => hook.result.current.submit({ action: "create", body }));
  expect(queries.runLibraryPurge).not.toHaveBeenCalled();
  expect(hook.result.current.storageError).toBeTruthy();
  spy.mockRestore();
});
it("完整计划已冻结首批而页面中断时恢复精确子key和正文，不自动继续", async () => {
  const plan = createPurgePlan(identity, { ...review, mode: "all" });
  plan.batches[0].input = body;
  savePurgePlan(sessionStorage, plan, null);
  const hook = renderHook(() => usePurgeWriter(identity, vi.fn()));
  await waitFor(() =>
    expect(hook.result.current.intent?.key).toBe(plan.batches[0].key),
  );
  expect(hook.result.current.intent?.body).toEqual(body);
  expect(queries.runLibraryPurge).not.toHaveBeenCalled();
});
it("201项完整计划只受理首200，上一批未终态时明确继续仍停止，不会发余1项", async () => {
  const items = Array.from({ length: 201 }, (_, i) => ({
    id: `10000000-0000-4000-8000-${String(i + 1).padStart(12, "0")}`,
    revision: 2,
    title: `条目${i}`,
  }));
  vi.mocked(queries.reviewLibraryPurge).mockResolvedValue({
    ...review,
    mode: "all",
    items,
  });
  const firstJob = {
    ...job,
    items: items.slice(0, 200).map((item, index) => ({
      index,
      item_id: item.id,
      asset_id: null,
      status: "queued" as const,
      failure_code: null,
    })),
  };
  vi.mocked(queries.runLibraryPurge).mockResolvedValue(firstJob);
  vi.mocked(queries.getLibraryPurge).mockResolvedValue({
    ...firstJob,
    status: "needs_reconciliation",
    needs_reconciliation: true,
  });
  const hook = renderHook(() => usePurgeWriter(identity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  await act(() =>
    hook.result.current.startPlan({ ...review, mode: "all", items }),
  );
  expect(queries.runLibraryPurge).toHaveBeenCalledTimes(1);
  expect(hook.result.current.plan?.batches).toHaveLength(2);
  await act(() => hook.result.current.continuePlan());
  expect(queries.runLibraryPurge).toHaveBeenCalledTimes(1);
  expect(hook.result.current.error).toContain("未知");
});
it("页面变后台立即停止后续子批并保留稳定key，刷新不自动继续，明确继续才完成余1项", async () => {
  const items = Array.from({ length: 201 }, (_, i) => ({
    id: `10000000-0000-4000-8000-${String(i + 1).padStart(12, "0")}`,
    revision: 2,
    title: `条目${i}`,
  }));
  const first = {
      ...job,
      items: items.slice(0, 200).map((item, index) => ({
        index,
        item_id: item.id,
        asset_id: null,
        status: "queued" as const,
        failure_code: null,
      })),
    },
    terminal = {
      ...first,
      status: "succeeded" as const,
      stage: "completed" as const,
      items: first.items.map((item) => ({
        ...item,
        status: "succeeded" as const,
      })),
    },
    second = {
      ...terminal,
      id: transferIDs.target,
      items: [{ ...terminal.items[0], index: 0, item_id: items[200].id }],
    };
  vi.mocked(queries.reviewLibraryPurge).mockResolvedValueOnce({
    ...review,
    mode: "all",
    items,
  });
  vi.mocked(queries.runLibraryPurge)
    .mockResolvedValueOnce(first)
    .mockResolvedValue(second);
  let resolve: (value: typeof terminal) => void = () => {};
  vi.mocked(queries.getLibraryPurge).mockImplementationOnce(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const visibility = vi
    .spyOn(document, "visibilityState", "get")
    .mockReturnValue("visible");
  const hook = renderHook(() => usePurgeWriter(identity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  let work: Promise<void> = Promise.resolve();
  act(() => {
    work = hook.result.current.startPlan({ ...review, mode: "all", items });
  });
  await waitFor(() => expect(queries.getLibraryPurge).toHaveBeenCalled());
  await act(async () => {
    visibility.mockReturnValue("hidden");
    document.dispatchEvent(new Event("visibilitychange"));
    resolve(terminal);
    await work;
  });
  expect(queries.runLibraryPurge).toHaveBeenCalledTimes(1);
  expect(hook.result.current.running).toBe(false);
  const plan = loadPurgePlan(sessionStorage, identity)!;
  expect(plan.batches[1].input).toBeNull();
  const key = plan.batches[1].key;
  hook.unmount();
  visibility.mockReturnValue("visible");
  vi.mocked(queries.getLibraryPurge).mockImplementation(async (_, id) =>
    id === first.id ? terminal : second,
  );
  vi.mocked(queries.reviewLibraryPurge)
    .mockResolvedValueOnce({
      ...review,
      mode: "all",
      revision: 205,
      items: items.slice(200),
    })
    .mockResolvedValue({ ...review, mode: "all", revision: 207, items: [] });
  const restored = renderHook(() => usePurgeWriter(identity, vi.fn()));
  await waitFor(() => expect(restored.result.current.plan?.id).toBe(plan.id));
  expect(queries.runLibraryPurge).toHaveBeenCalledTimes(1);
  await act(() => restored.result.current.continuePlan());
  expect(queries.runLibraryPurge).toHaveBeenCalledTimes(2);
  expect(vi.mocked(queries.runLibraryPurge).mock.calls[1][0].key).toBe(key);
  expect(restored.result.current.planComplete).toBe(true);
  visibility.mockRestore();
});
