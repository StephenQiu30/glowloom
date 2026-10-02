import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import * as query from "./bible-queries";
import { useBibleWriter } from "./use-bible-writer";
import { loadBibleIntent } from "./bible-intent";

vi.mock("./bible-queries", () => ({
  freshBibleScope: vi.fn(),
  applyBibleIntent: vi.fn(),
  getBibleDetail: vi.fn(),
}));
const identity = {
  origin: window.location.origin,
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  projectId: "33333333-3333-4333-8333-333333333333",
};
const command = {
  action: "create" as const,
  kind: "character" as const,
  body: {
    expected_revision: 0,
    character: { name: "完整原角色😀", definition: { role: "原正文" } },
  },
};
const page = {
  entries: [],
  current_actor_id: identity.actorId,
  current_org_id: identity.orgId,
};
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  vi.mocked(query.freshBibleScope).mockResolvedValue(page);
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
it("非法200未知保完整正文/原键，刷新无自动写且scope403不读正文/不释放原意图", async () => {
  vi.mocked(query.applyBibleIntent).mockRejectedValue(
    new ApiError(502, "invalid_bible_response"),
  );
  const accepted = vi.fn();
  const first = renderHook(() => useBibleWriter(identity, accepted));
  await waitFor(() => expect(first.result.current.ready).toBe(true));
  await act(async () => first.result.current.submit(command));
  const original = first.result.current.intent!;
  expect(original.command).toEqual(command);
  expect(loadBibleIntent(sessionStorage, identity)).toEqual(original);
  first.unmount();
  const refreshed = renderHook(() => useBibleWriter(identity, accepted));
  await waitFor(() =>
    expect(refreshed.result.current.intent).toEqual(original),
  );
  expect(query.applyBibleIntent).toHaveBeenCalledOnce();
  vi.mocked(query.freshBibleScope).mockRejectedValue(
    new ApiError(403, "bible_scope_changed"),
  );
  await act(async () => refreshed.result.current.replay());
  expect(query.applyBibleIntent).toHaveBeenCalledOnce();
  expect(refreshed.result.current.intent).toEqual(original);
  expect(refreshed.result.current.rejected).toBeNull();
  const unload = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(unload);
  expect(unload.defaultPrevented).toBe(true);
});
it("存储写失败不发命令，恢复存储后仍人工原键核验", async () => {
  const spy = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("quota");
  });
  const hook = renderHook(() => useBibleWriter(identity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  await act(async () => hook.result.current.submit(command));
  const original = hook.result.current.intent;
  expect(query.applyBibleIntent).not.toHaveBeenCalled();
  expect(hook.result.current.storageError).toBeTruthy();
  spy.mockRestore();
  await act(async () => hook.result.current.restoreStorage());
  expect(query.applyBibleIntent).not.toHaveBeenCalled();
  expect(hook.result.current.intent).toEqual(original);
});
it("CAS409保草稿，读最新失败不可释放；明确新意图才更换key", async () => {
  vi.mocked(query.applyBibleIntent).mockRejectedValue(
    new ApiError(409, "revision_conflict"),
  );
  const hook = renderHook(() => useBibleWriter(identity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  await act(async () => hook.result.current.submit(command));
  const original = hook.result.current.intent!;
  expect(hook.result.current.rejected).toBeTruthy();
  expect(await hook.result.current.discardRejected()).toBe(false);
  vi.mocked(query.freshBibleScope).mockRejectedValueOnce(
    new ApiError(403, "forbidden"),
  );
  await act(async () => hook.result.current.readLatest());
  expect(hook.result.current.latest).toBeNull();
  expect(hook.result.current.intent).toEqual(original);
  await act(async () => hook.result.current.readLatest());
  await act(async () => {
    expect(await hook.result.current.discardRejected()).toBe(true);
  });
  await act(async () => hook.result.current.submit(command));
  expect(vi.mocked(query.applyBibleIntent).mock.calls[1][0].key).not.toBe(
    original.key,
  );
});
