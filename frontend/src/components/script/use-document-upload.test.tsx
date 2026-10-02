import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useDocumentUpload } from "./use-document-upload";
import { uploadDocument } from "./document-media";
import { getWorkspace } from "./source-queries";
import {
  documentFileFingerprint,
  requireOriginalDocument,
} from "./document-upload-intent";
vi.mock("./document-media", async (original) => ({
  ...(await original<typeof import("./document-media")>()),
  uploadDocument: vi.fn(),
}));
vi.mock("./source-queries", async (original) => ({
  ...(await original<typeof import("./source-queries")>()),
  getWorkspace: vi.fn(),
}));
vi.mock("./document-upload-intent", async (original) => ({
  ...(await original<typeof import("./document-upload-intent")>()),
  documentFileFingerprint: vi.fn(),
  requireOriginalDocument: vi.fn(),
}));
const scope = {
  origin: "http://127.0.0.1:3000",
  projectId: "11111111-1111-4111-8111-111111111111",
  actorId: "22222222-2222-4222-8222-222222222222",
  orgId: "33333333-3333-4333-8333-333333333333",
};
const head = {
  current_actor_id: scope.actorId,
  current_org_id: scope.orgId,
  state: {
    project_id: scope.projectId,
    org_id: scope.orgId,
    revision: 0,
    updated_at: "2026-10-02T00:00:00Z",
  },
};
const file = new File(["原文"], "原件.txt", { type: "text/plain" });
beforeEach(() => {
  vi.clearAllMocks();
  sessionStorage.clear();
  vi.mocked(getWorkspace).mockResolvedValue(head);
  vi.mocked(documentFileFingerprint).mockResolvedValue({
    fileName: file.name,
    byteSize: file.size,
    sha256: "a".repeat(64),
  });
  vi.mocked(requireOriginalDocument).mockResolvedValue(undefined);
});
afterEach(() => vi.restoreAllMocks());
it("未知上传跨挂载恢复同键，缺确切原文件不可重放且原scope改变零DML", async () => {
  vi.mocked(uploadDocument).mockRejectedValue(new Error("lost response"));
  const first = renderHook(() => useDocumentUpload(scope, vi.fn()));
  await waitFor(() => expect(first.result.current.ready).toBe(true));
  await act(() => first.result.current.submit(file));
  const original = first.result.current.intent!;
  expect(original).toBeTruthy();
  first.unmount();
  const second = renderHook(() => useDocumentUpload(scope, vi.fn()));
  await waitFor(() =>
    expect(second.result.current.intent?.key).toBe(original.key),
  );
  await act(() => second.result.current.replay(null));
  expect(uploadDocument).toHaveBeenCalledTimes(1);
  await act(() => second.result.current.replay(file));
  expect(uploadDocument).toHaveBeenLastCalledWith(
    scope.projectId,
    file,
    original.key,
    scope.origin,
    expect.any(AbortSignal),
    expect.any(Function),
  );
  vi.mocked(getWorkspace).mockResolvedValue({
    ...head,
    current_actor_id: original.key,
  });
  await act(() => second.result.current.replay(file));
  expect(uploadDocument).toHaveBeenCalledTimes(2);
  expect(second.result.current.intent?.key).toBe(original.key);
});
it("持久化失败不发上传，人工恢复存储仍保留原key且不用新键", async () => {
  const storage = vi
    .spyOn(Storage.prototype, "setItem")
    .mockImplementation(() => {
      throw new Error("quota");
    });
  const hook = renderHook(() => useDocumentUpload(scope, vi.fn()));
  await waitFor(() => expect(hook.result.current.ready).toBe(true));
  await act(() => hook.result.current.submit(file));
  const key = hook.result.current.intent!.key;
  expect(uploadDocument).not.toHaveBeenCalled();
  expect(hook.result.current.storageError).toBeTruthy();
  storage.mockRestore();
  await act(() => hook.result.current.restoreStorage());
  expect(hook.result.current.intent!.key).toBe(key);
  expect(uploadDocument).not.toHaveBeenCalled();
});
