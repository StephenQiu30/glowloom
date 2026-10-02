import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { useFileImport } from "./use-file-import";
import { listFileImports, runFileImportIntent } from "./file-import-queries";
vi.mock("./file-import-queries", () => ({
  listFileImports: vi.fn(),
  runFileImportIntent: vi.fn(),
}));
const id = (n: number) =>
  `${String(n).repeat(8)}-${String(n).repeat(4)}-4${String(n).repeat(3)}-8${String(n).repeat(3)}-${String(n).repeat(12)}`;
const scope = {
  origin: window.location.origin,
  projectId: id(1),
  actorId: id(2),
  orgId: id(3),
};
const command = {
  action: "create" as const,
  body: {
    expected_revision: 5,
    base_version_id: id(5),
    rights_confirmed: true as const,
    asset_ids: [id(6), id(7)],
  },
};
beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  vi.mocked(listFileImports).mockResolvedValue({
    current_actor_id: scope.actorId,
    current_org_id: scope.orgId,
    items: [],
  });
});
afterEach(cleanup);
it("unknown跨刷新同原键/原序/原CAS人工核验，scope拒绝与403不丢原体", async () => {
  vi.mocked(runFileImportIntent).mockRejectedValue(
    new ApiError(502, "invalid_response"),
  );
  const first = renderHook(() => useFileImport(scope, vi.fn()));
  await waitFor(() => expect(first.result.current.ready).toBe(true));
  await act(() => first.result.current.submit(command));
  const original = first.result.current.intent!;
  first.unmount();
  const second = renderHook(() => useFileImport(scope, vi.fn()));
  await waitFor(() =>
    expect(second.result.current.intent?.key).toBe(original.key),
  );
  vi.mocked(listFileImports).mockResolvedValueOnce({
    current_actor_id: id(9),
    current_org_id: scope.orgId,
    items: [],
  });
  await act(() => second.result.current.replay());
  expect(runFileImportIntent).toHaveBeenCalledTimes(1);
  vi.mocked(runFileImportIntent).mockRejectedValueOnce(
    new ApiError(403, "forbidden"),
  );
  await act(() => second.result.current.replay());
  expect(runFileImportIntent).toHaveBeenLastCalledWith(original);
  expect(second.result.current.intent).toEqual(original);
});
it("storage失败零DML，409保草稿且人工读最新以前不能另新键", async () => {
  const hook = renderHook(() => useFileImport(scope, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  const spy = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw new DOMException("quota", "QuotaExceededError");
  });
  await act(() => hook.result.current.submit(command));
  expect(runFileImportIntent).not.toHaveBeenCalled();
  const key = hook.result.current.intent!.key;
  spy.mockRestore();
  await act(() => hook.result.current.restoreStorage());
  expect(hook.result.current.intent!.key).toBe(key);
  vi.mocked(runFileImportIntent).mockRejectedValueOnce(
    new ApiError(409, "stale_revision"),
  );
  await act(() => hook.result.current.replay());
  expect(hook.result.current.rejected?.intent.body).toEqual(command.body);
  await act(() =>
    hook.result.current.submit({
      ...command,
      body: { ...command.body, expected_revision: 8 },
    }),
  );
  expect(runFileImportIntent).toHaveBeenCalledTimes(1);
});
