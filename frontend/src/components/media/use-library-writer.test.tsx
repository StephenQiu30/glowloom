import { act, renderHook, waitFor, cleanup } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { useLibraryWriter } from "./use-library-writer";
import * as query from "./library-queries";
import { saveLibraryIntent } from "./library-intent";
vi.mock("./library-queries", () => ({
  freshLibrary: vi.fn(),
  applyLibraryIntent: vi.fn(),
  getLibraryDetail: vi.fn(),
}));
const identity = {
  origin: window.location.origin,
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  libraryId: "33333333-3333-5333-8333-333333333333",
  scope: { kind: "personal" as const },
};
const body = {
  scope: identity.scope,
  action: "delete_folder" as const,
  expected_revision: 4,
  folder_id: identity.actorId,
  expected_folder_revision: 1,
};
const page = {
  current_actor_id: identity.actorId,
  current_org_id: identity.orgId,
  library_id: identity.libraryId,
  scope: identity.scope,
  revision: 9,
  page: 1,
  page_size: 1,
  total: 0,
  items: [],
  folders: [],
  category_counts: {},
  folder_counts: {},
};
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  vi.mocked(query.freshLibrary).mockResolvedValue(page);
});
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
it("未知提交刷新后人工同键同体核验，scope变化与旧主体403不释放新写", async () => {
  vi.mocked(query.applyLibraryIntent).mockRejectedValue(
    new ApiError(502, "invalid_response"),
  );
  const accepted = vi.fn();
  const first = renderHook(() => useLibraryWriter(identity, accepted));
  await waitFor(() => expect(first.result.current.ready).toBe(true));
  await act(async () => first.result.current.submit(body));
  const original = first.result.current.intent!;
  expect(original.body).toEqual(body);
  expect(first.result.current.locked).toBe(true);
  first.unmount();
  const reloaded = renderHook(() => useLibraryWriter(identity, accepted));
  await waitFor(() =>
    expect(reloaded.result.current.intent?.key).toBe(original.key),
  );
  expect(query.applyLibraryIntent).toHaveBeenCalledOnce();
  vi.mocked(query.freshLibrary).mockRejectedValue(
    new ApiError(409, "scope_changed"),
  );
  await act(async () => reloaded.result.current.replay());
  expect(query.applyLibraryIntent).toHaveBeenCalledOnce();
  vi.mocked(query.freshLibrary).mockResolvedValue(page);
  vi.mocked(query.applyLibraryIntent).mockRejectedValue(
    new ApiError(403, "forbidden"),
  );
  await act(async () => reloaded.result.current.replay());
  expect(query.applyLibraryIntent).toHaveBeenLastCalledWith(original);
  expect(reloaded.result.current.intent).toEqual(original);
  expect(reloaded.result.current.rejected).toBeNull();
  expect(accepted).not.toHaveBeenCalled();
  const unload = new Event("beforeunload", { cancelable: true });
  window.dispatchEvent(unload);
  expect(unload.defaultPrevented).toBe(true);
});
it("确定CAS409保留原草稿，读最新后明确废弃旧意图；新提交才产生新键", async () => {
  vi.mocked(query.applyLibraryIntent).mockRejectedValue(
    new ApiError(409, "revision_conflict"),
  );
  const hook = renderHook(() => useLibraryWriter(identity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  await act(async () => hook.result.current.submit(body));
  const original = hook.result.current.intent!;
  expect(hook.result.current.rejected).toBeTruthy();
  await act(async () => hook.result.current.readLatest());
  expect(hook.result.current.intent).toEqual(original);
  expect(query.applyLibraryIntent).toHaveBeenCalledOnce();
  await act(async () => hook.result.current.discardRejected());
  expect(hook.result.current.intent).toBeNull();
  vi.mocked(query.applyLibraryIntent).mockResolvedValue({
    ...page,
    revision: 10,
    items: [],
  });
  await act(async () =>
    hook.result.current.submit({ ...body, expected_revision: 9 }),
  );
  expect(vi.mocked(query.applyLibraryIntent).mock.calls[1][0].key).not.toBe(
    original.key,
  );
});
it("存储quota零DML，恢复后仍需人工原键核验；启动不自动重放", async () => {
  const set = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new Error("quota");
  });
  const hook = renderHook(() => useLibraryWriter(identity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  await act(async () => hook.result.current.submit(body));
  const original = hook.result.current.intent!;
  expect(query.applyLibraryIntent).not.toHaveBeenCalled();
  expect(hook.result.current.storageError).toBeTruthy();
  set.mockRestore();
  await act(async () => hook.result.current.restoreStorage());
  expect(query.applyLibraryIntent).not.toHaveBeenCalled();
  expect(hook.result.current.intent).toEqual(original);
  vi.mocked(query.applyLibraryIntent).mockResolvedValue({
    ...page,
    revision: 5,
    items: [],
  });
  await act(async () => hook.result.current.replay());
  expect(query.applyLibraryIntent).toHaveBeenCalledWith(original);
});
it("旧永久成功回执只交失效回调，不把当前版本换成旧回执", async () => {
  const original = {
    version: 1 as const,
    key: identity.orgId,
    ...identity,
    body,
  };
  saveLibraryIntent(sessionStorage, original);
  const receipt = { ...page, revision: 5, items: [] };
  vi.mocked(query.applyLibraryIntent).mockResolvedValue(receipt);
  const accepted = vi.fn();
  const hook = renderHook(() => useLibraryWriter(identity, accepted));
  await waitFor(() => expect(hook.result.current.intent).toEqual(original));
  await act(async () => hook.result.current.replay());
  expect(accepted).toHaveBeenCalledWith(receipt);
  expect(hook.result.current.latest).toBeNull();
  expect(hook.result.current.intent).toBeNull();
});
it("409读取成员详情必须等fresh身份成功，403不会沿旧scope读取正文", async () => {
  vi.mocked(query.applyLibraryIntent).mockRejectedValue(
    new ApiError(409, "revision_conflict"),
  );
  const hook = renderHook(() => useLibraryWriter(identity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  await act(async () =>
    hook.result.current.submit({
      scope: identity.scope,
      expected_revision: 4,
      action: "recycle_items",
      items: [{ id: identity.actorId, revision: 1 }],
    }),
  );
  let reject!: (cause: Error) => void;
  vi.mocked(query.freshLibrary).mockImplementation(
    () =>
      new Promise((_, fail) => {
        reject = fail;
      }),
  );
  let pending!: Promise<void>;
  act(() => {
    pending = hook.result.current.readLatest();
  });
  expect(query.getLibraryDetail).not.toHaveBeenCalled();
  await act(async () => {
    reject(new ApiError(403, "forbidden"));
    await pending;
  });
  expect(query.getLibraryDetail).not.toHaveBeenCalled();
  expect(hook.result.current.latest).toBeNull();
  expect(hook.result.current.intent).toBeTruthy();
});
