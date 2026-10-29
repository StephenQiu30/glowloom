import { StrictMode } from "react";
import { webcrypto } from "node:crypto";
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { CanvasYjsLiveSession } from "@/components/canvas/yjs-live-session";
import { emptyDirectorConfig } from "@/components/canvas/director/director-model";
import { useDirectorRecovery } from "@/components/canvas/director/director-recovery";
const api = vi.hoisted(() => ({
  read: vi.fn(),
  save: vi.fn(),
  remove: vi.fn(),
}));
vi.mock("@/api/canvases", () => ({
  readDirectorRecovery: api.read,
  saveDirectorRecovery: api.save,
  deleteDirectorRecovery: api.remove,
}));
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}
function client() {
  return {
    retainedDirector: vi.fn(() => null),
    retainDirector: vi.fn(),
    discardRetainedDirector: vi.fn(),
  } as unknown as CanvasYjsLiveSession;
}
function draft(revision: number) {
  return { revision, director: emptyDirectorConfig() };
}
beforeEach(() => {
  vi.stubGlobal("crypto", webcrypto);
  vi.clearAllMocks();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("ignores the earlier StrictMode read after the current read established its CAS revision", async () => {
  const old = deferred<{ draft: ReturnType<typeof draft> }>(),
    current = deferred<{ draft: ReturnType<typeof draft> }>();
  api.read
    .mockReturnValueOnce(old.promise)
    .mockReturnValueOnce(current.promise);
  api.save.mockResolvedValue({ revision: 8 });
  const session = client(),
    hook = renderHook(() => useDirectorRecovery(session, "canvas", "node"), {
      wrapper: StrictMode,
    });
  expect(api.read).toHaveBeenCalledTimes(2);
  const newest = draft(7);
  await act(async () => {
    current.resolve({ draft: newest });
    await current.promise;
  });
  await act(async () => {
    old.resolve({ draft: draft(2) });
    await old.promise;
  });
  await act(async () => {
    hook.result.current.retain(newest.director, newest.director, "epoch");
    await new Promise((resolve) => setTimeout(resolve, 20));
  });
  expect(api.save.mock.calls[0]?.[1].expected_revision).toBe(7);
});
it("does not carry a retained scene from one canvas/node into the next", async () => {
  const a = draft(4),
    b = draft(1);
  api.read
    .mockResolvedValueOnce({ draft: a })
    .mockResolvedValueOnce({ draft: b });
  const session = client(),
    hook = renderHook(
      ({ canvas, node }) => useDirectorRecovery(session, canvas, node),
      { initialProps: { canvas: "a", node: "a-node" } },
    );
  await act(async () => {
    await Promise.resolve();
  });
  expect(hook.result.current.retained?.id).toBe(a.director.id);
  hook.rerender({ canvas: "b", node: "b-node" });
  await act(async () => {
    await Promise.resolve();
  });
  expect(hook.result.current.retained?.id).toBe(b.director.id);
});
it("reloads private recovery when the authenticated live session changes", async () => {
  api.read.mockResolvedValue({ draft: null });
  const hook = renderHook(
    ({ session }) => useDirectorRecovery(session, "canvas", "node"),
    { initialProps: { session: client() } },
  );
  await act(async () => {
    await Promise.resolve();
  });
  hook.rerender({ session: client() });
  await act(async () => {
    await Promise.resolve();
  });
  expect(api.read).toHaveBeenCalledTimes(2);
});
it("preserves a newer local retained edit when an older discard response arrives", async () => {
  api.read.mockResolvedValue({ draft: draft(5) });
  const removal = deferred<void>();
  api.remove.mockReturnValue(removal.promise);
  api.save.mockRejectedValue(
    new Error("the deleted revision no longer exists"),
  );
  const session = client(),
    hook = renderHook(() => useDirectorRecovery(session, "canvas", "node"));
  await act(async () => {
    await Promise.resolve();
  });
  let discard!: Promise<void>;
  act(() => {
    discard = hook.result.current.discard();
  });
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
  const newer = emptyDirectorConfig();
  await act(async () => {
    hook.result.current.retain(newer, newer, "epoch");
    await new Promise((resolve) => setTimeout(resolve, 20));
  });
  await act(async () => {
    removal.resolve();
    await discard;
  });
  expect(hook.result.current.retained?.id).toBe(newer.id);
});

it("ignores an old session's retained-save callback after changing canvas", async () => {
  const first = draft(1),
    second = draft(2);
  api.read
    .mockResolvedValueOnce({ draft: first })
    .mockResolvedValueOnce({ draft: second });
  api.save.mockResolvedValue({ revision: 3 });
  const hook = renderHook(
    ({ session, canvas }) => useDirectorRecovery(session, canvas, "node"),
    { initialProps: { session: client(), canvas: "first" } },
  );
  await act(async () => {
    await Promise.resolve();
  });
  const staleRetain = hook.result.current.retain;
  hook.rerender({ session: client(), canvas: "second" });
  await act(async () => {
    await Promise.resolve();
  });
  await act(async () => {
    staleRetain(first.director, first.director, "epoch");
    await new Promise((resolve) => setTimeout(resolve, 20));
  });
  expect(api.save).not.toHaveBeenCalled();
  expect(hook.result.current.retained?.id).toBe(second.director.id);
});
it("ignores an old session's discard callback after changing canvas", async () => {
  const first = draft(1),
    second = draft(2);
  api.read
    .mockResolvedValueOnce({ draft: first })
    .mockResolvedValueOnce({ draft: second });
  api.remove.mockResolvedValue(undefined);
  const hook = renderHook(
    ({ session, canvas }) => useDirectorRecovery(session, canvas, "node"),
    { initialProps: { session: client(), canvas: "first" } },
  );
  await act(async () => {
    await Promise.resolve();
  });
  const staleDiscard = hook.result.current.discard;
  hook.rerender({ session: client(), canvas: "second" });
  await act(async () => {
    await Promise.resolve();
  });
  await act(async () => {
    await staleDiscard();
  });
  expect(api.remove).not.toHaveBeenCalled();
  expect(hook.result.current.retained?.id).toBe(second.director.id);
});
