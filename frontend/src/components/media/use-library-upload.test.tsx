import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useLibraryUpload } from "./use-library-upload";
import * as reads from "./library-queries";
import * as writes from "./library-upload-query";
import { loadLibraryUploads } from "./library-upload-intent";
import { ApiError } from "@/lib/request";
vi.mock("./library-queries", () => ({ freshLibrary: vi.fn() }));
vi.mock("./library-upload-query", () => ({ uploadLibraryOriginal: vi.fn() }));
const identity = {
  origin: window.location.origin,
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  libraryId: "33333333-3333-5333-8333-333333333333",
  scope: { kind: "personal" as const },
};
const asset = {
  id: "44444444-4444-4444-8444-444444444444",
  kind: "document" as const,
  file_name: "a.txt",
  mime_type: "text/plain",
  byte_size: 4,
  width: null,
  height: null,
  duration_ms: null,
  revision: 1,
};
beforeEach(() => {
  vi.resetAllMocks();
  sessionStorage.clear();
  vi.mocked(reads.freshLibrary).mockResolvedValue(
    {} as Awaited<ReturnType<typeof reads.freshLibrary>>,
  );
});
afterEach(cleanup);
it("unknown刷新只恢复原key/fingerprint，重新选择确切原件后人工核验，scope换人零POST", async () => {
  const file = new File(["real"], "a.txt", { type: "text/plain" });
  vi.mocked(writes.uploadLibraryOriginal).mockRejectedValue(
    new ApiError(502, "invalid_response"),
  );
  const first = renderHook(() => useLibraryUpload(identity, vi.fn()));
  await waitFor(() => expect(first.result.current.ready).toBe(true));
  await act(async () => first.result.current.start([file]));
  const original = loadLibraryUploads(sessionStorage, identity)[0];
  expect(original).toBeTruthy();
  expect(first.result.current.locked).toBe(true);
  first.unmount();
  const callback = vi.fn();
  const restored = renderHook(() => useLibraryUpload(identity, callback));
  await waitFor(() => expect(restored.result.current.ready).toBe(true));
  expect(writes.uploadLibraryOriginal).toHaveBeenCalledTimes(1);
  await act(async () =>
    restored.result.current.replay(
      original.key,
      new File(["edit"], "a.txt", { type: "text/plain" }),
    ),
  );
  expect(writes.uploadLibraryOriginal).toHaveBeenCalledTimes(1);
  vi.mocked(reads.freshLibrary).mockRejectedValueOnce(
    new ApiError(409, "scope_changed"),
  );
  await act(async () => restored.result.current.replay(original.key, file));
  expect(writes.uploadLibraryOriginal).toHaveBeenCalledTimes(1);
  vi.mocked(writes.uploadLibraryOriginal).mockResolvedValue(asset);
  await act(async () => restored.result.current.replay(original.key, file));
  expect(writes.uploadLibraryOriginal).toHaveBeenLastCalledWith(
    identity,
    file,
    original,
    expect.any(AbortSignal),
    expect.any(Function),
  );
  expect(callback).toHaveBeenCalledOnce();
  expect(loadLibraryUploads(sessionStorage, identity)).toEqual([]);
});
it("存储quota发生在上传之前，恢复存储不发送文件；known409须人工释放，取消只中止网络", async () => {
  const hook = renderHook(() => useLibraryUpload(identity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  const set = vi.spyOn(Storage.prototype, "setItem").mockImplementation(() => {
    throw Error("quota");
  });
  const file = new File(["real"], "a.txt");
  await act(async () => hook.result.current.start([file]));
  expect(writes.uploadLibraryOriginal).not.toHaveBeenCalled();
  expect(hook.result.current.storageError).toBeTruthy();
  set.mockRestore();
  await act(async () => hook.result.current.restoreStorage());
  expect(writes.uploadLibraryOriginal).not.toHaveBeenCalled();
  const original = loadLibraryUploads(sessionStorage, identity)[0];
  vi.mocked(writes.uploadLibraryOriginal).mockRejectedValue(
    new ApiError(409, "revision_conflict"),
  );
  await act(async () => hook.result.current.replay(original.key, file));
  expect(hook.result.current.entries[0].status).toBe("rejected");
  await act(async () => hook.result.current.discardRejected(original.key));
  expect(loadLibraryUploads(sessionStorage, identity)).toEqual([]);
});
it("真实批次最多4个并发，停止只abort网络并保全部未确认原键，不假报已删除", async () => {
  const owners: AbortSignal[] = [];
  vi.mocked(writes.uploadLibraryOriginal).mockImplementation(
    async (_identity, _file, _intent, signal) => {
      owners.push(signal);
      return new Promise((_, reject) => {
        signal.addEventListener("abort", () => reject(new Error("aborted")), {
          once: true,
        });
      });
    },
  );
  const hook = renderHook(() => useLibraryUpload(identity, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  let running!: Promise<void>;
  act(() => {
    running = hook.result.current.start(
      Array.from(
        { length: 6 },
        (_, index) => new File(["real"], `part${index}.txt`),
      ),
    );
  });
  await waitFor(() =>
    expect(writes.uploadLibraryOriginal).toHaveBeenCalledTimes(4),
  );
  act(() => hook.result.current.stopSending());
  await act(async () => running);
  expect(owners.every((signal) => signal.aborted)).toBe(true);
  expect(writes.uploadLibraryOriginal).toHaveBeenCalledTimes(4);
  expect(loadLibraryUploads(sessionStorage, identity)).toHaveLength(6);
  expect(
    hook.result.current.entries.every((entry) => entry.status === "unknown"),
  ).toBe(true);
  expect(hook.result.current.locked).toBe(true);
});
