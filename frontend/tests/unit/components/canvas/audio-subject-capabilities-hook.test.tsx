import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createCanvasNode } from "@/components/canvas/lib/canvas-node-factory";
import { useCanvasAudioSubjectCapabilities } from "@/components/canvas/hooks/use-canvas-audio-subject-capabilities";
const mocks = vi.hoisted(() => ({
  read: vi.fn(),
  require: vi.fn((value: unknown) => value),
  generation: 0,
  scope: "scope",
  listeners: new Set<() => void>(),
}));
vi.mock("@/api/subjects", () => ({ getSubjectVersion: mocks.read }));
vi.mock("@/components/canvas/lib/canvas-subject-source", () => ({
  requireCanvasSubjectVersion: mocks.require,
}));
vi.mock("@/lib/request", () => ({
  getCreationScope: () => mocks.scope,
  getSessionGeneration: () => mocks.generation,
  getScopeGeneration: () => 0,
  subscribeContextChange: (listener: () => void) => {
    mocks.listeners.add(listener);
    return () => mocks.listeners.delete(listener);
  },
}));
const identity = { projectID: "project", scopeID: "scope" };
const subject = {
  id: "a1111111-1111-4111-8111-111111111111",
  version_id: "b1111111-1111-4111-8111-111111111111",
  primary: { kind: "image" },
};
function source() {
  const node = createCanvasNode("subject", { x: 0, y: 0 });
  node.serviceData!.config!.subject = {
    version: 1,
    subject_id: "a1111111-1111-4111-8111-111111111111",
    version_id: "b1111111-1111-4111-8111-111111111111",
  };
  return node;
}
beforeEach(() => {
  mocks.read.mockReset();
  mocks.require.mockClear();
  mocks.require.mockImplementation((value) => value);
  mocks.generation = 0;
  mocks.scope = "scope";
});
afterEach(cleanup);
it("fetches only prepare-selected pinned sources, deduplicates their exact pin and publishes both node mappings", async () => {
  mocks.read.mockResolvedValue(subject);
  const first = source(),
    second = source();
  const { result } = renderHook(() =>
    useCanvasAudioSubjectCapabilities(identity),
  );
  expect(mocks.read).not.toHaveBeenCalled();
  act(() =>
    result.current.prepare([
      first,
      second,
      createCanvasNode("image", { x: 0, y: 0 }),
    ]),
  );
  await waitFor(() => expect(result.current.pinnedSubjects.size).toBe(2));
  expect(mocks.read).toHaveBeenCalledTimes(1);
  expect(mocks.require).toHaveBeenCalledWith(subject, "scope", {
    subject_id: "a1111111-1111-4111-8111-111111111111",
    version_id: "b1111111-1111-4111-8111-111111111111",
  });
  act(() => result.current.prepare([first, second]));
  expect(mocks.read).toHaveBeenCalledTimes(1);
});
it("does not fetch when the formal model lacks subject capability", async () => {
  const { result } = renderHook(() =>
    useCanvasAudioSubjectCapabilities(identity, false),
  );
  act(() => result.current.prepare([source()]));
  expect(mocks.read).not.toHaveBeenCalled();
  expect(result.current.pinnedSubjects.size).toBe(0);
});
it("aborts deselected requests and ignores delayed results", async () => {
  let resolve!: (value: unknown) => void;
  mocks.read.mockImplementation(
    () =>
      new Promise((done) => {
        resolve = done;
      }),
  );
  const { result } = renderHook(() =>
    useCanvasAudioSubjectCapabilities(identity),
  );
  act(() => result.current.prepare([source()]));
  const signal = mocks.read.mock.calls[0][1].signal as AbortSignal;
  act(() => result.current.prepare([]));
  expect(signal.aborted).toBe(true);
  await act(async () => {
    resolve(subject);
  });
  expect(result.current.pinnedSubjects.size).toBe(0);
});
it("separates project/scope contexts, clears old mappings and cancels unmounted work", async () => {
  mocks.read
    .mockResolvedValueOnce(subject)
    .mockImplementationOnce(() => new Promise(() => {}));
  const node = source();
  const { result, rerender, unmount } = renderHook(
    ({ projectID }) =>
      useCanvasAudioSubjectCapabilities({ ...identity, projectID }),
    { initialProps: { projectID: "project" } },
  );
  act(() => result.current.prepare([node]));
  await waitFor(() => expect(result.current.pinnedSubjects.size).toBe(1));
  rerender({ projectID: "other" });
  expect(result.current.pinnedSubjects.size).toBe(0);
  act(() => result.current.prepare([node]));
  const signal = mocks.read.mock.calls[1][1].signal as AbortSignal;
  unmount();
  expect(signal.aborted).toBe(true);
});
it("never publishes a mismatched or failed pin receipt", async () => {
  mocks.read.mockResolvedValue(subject);
  mocks.require.mockImplementationOnce(() => {
    throw new Error("wrong version");
  });
  const { result } = renderHook(() =>
    useCanvasAudioSubjectCapabilities(identity),
  );
  await act(async () => result.current.prepare([source()]));
  expect(result.current.pinnedSubjects.size).toBe(0);
});
it("replaces changed pins and clears a completed private cache on identity invalidation", async () => {
  mocks.read.mockResolvedValue(subject);
  const node = source();
  const { result } = renderHook(() =>
    useCanvasAudioSubjectCapabilities(identity),
  );
  act(() => result.current.prepare([node]));
  await waitFor(() => expect(result.current.pinnedSubjects.size).toBe(1));
  const changed = structuredClone(node);
  changed.serviceData!.config!.subject!.version_id =
    "c1111111-1111-4111-8111-111111111111";
  act(() => result.current.prepare([changed]));
  await waitFor(() => expect(mocks.read).toHaveBeenCalledTimes(2));
  expect(mocks.read.mock.calls[1][0]).toEqual({
    id: "a1111111-1111-4111-8111-111111111111",
    version: "c1111111-1111-4111-8111-111111111111",
  });
  act(() => {
    mocks.generation++;
    mocks.listeners.forEach((listener) => listener());
  });
  expect(result.current.pinnedSubjects.size).toBe(0);
});
