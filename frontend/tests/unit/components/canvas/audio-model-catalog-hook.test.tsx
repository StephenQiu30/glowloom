import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { SWRConfig } from "swr";
import type { ReactNode } from "react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
const mocks = vi.hoisted(() => ({
  read: vi.fn(),
  generation: 0,
  listeners: new Set<() => void>(),
}));
vi.mock("@/components/canvas/lib/canvas-audio-model-catalog", () => ({
  readCanvasAudioModelCatalog: mocks.read,
}));
vi.mock("@/lib/request", () => ({
  getSessionGeneration: () => mocks.generation,
  getScopeGeneration: () => 0,
  subscribeContextChange: (listener: () => void) => {
    mocks.listeners.add(listener);
    return () => mocks.listeners.delete(listener);
  },
}));
import { useCanvasAudioModelCatalog } from "@/components/canvas/hooks/use-canvas-audio-model-catalog";
afterEach(cleanup);
const identity = { projectID: "p", scopeID: "s" };
function wrapper({ children }: { children: ReactNode }) {
  return (
    <SWRConfig value={{ provider: () => new Map(), dedupingInterval: 0 }}>
      {children}
    </SWRConfig>
  );
}
beforeEach(() => {
  mocks.read.mockReset();
  mocks.generation = 0;
});
it("keeps loading/error unavailable and publishes only the current scoped result", async () => {
  mocks.read
    .mockResolvedValueOnce({ ...identity, status: "ready", models: [] })
    .mockRejectedValueOnce(new Error("offline"));
  const { result } = renderHook(() => useCanvasAudioModelCatalog(identity), {
    wrapper,
  });
  expect(result.current.catalog.status).toBe("loading");
  await waitFor(() => expect(result.current.catalog.status).toBe("ready"));
  await act(async () => {
    await result.current.reload();
  });
  expect(result.current.catalog.status).toBe("error");
});
it("aborts a superseded scope request and ignores its late result", async () => {
  let resolveOld!: (value: unknown) => void;
  mocks.read
    .mockImplementationOnce(
      () =>
        new Promise((resolve) => {
          resolveOld = resolve;
        }),
    )
    .mockResolvedValueOnce({
      projectID: "next",
      scopeID: "s",
      status: "ready",
      models: [],
    });
  const { result, rerender, unmount } = renderHook(
    ({ projectID }) => useCanvasAudioModelCatalog({ ...identity, projectID }),
    { wrapper, initialProps: { projectID: "p" } },
  );
  await waitFor(() => expect(mocks.read).toHaveBeenCalledTimes(1));
  const signal = mocks.read.mock.calls[0][1] as AbortSignal;
  rerender({ projectID: "next" });
  await waitFor(() =>
    expect(result.current.catalog).toMatchObject({
      projectID: "next",
      status: "ready",
    }),
  );
  expect(signal.aborted).toBe(true);
  await act(async () => {
    resolveOld({ ...identity, status: "ready", models: [{ id: "old" }] });
  });
  expect(result.current.catalog.projectID).toBe("next");
  unmount();
});
it("invalidates ready cached capabilities when identity generation changes", async () => {
  mocks.read
    .mockResolvedValueOnce({ ...identity, status: "ready", models: [] })
    .mockImplementationOnce(() => new Promise(() => {}));
  const { result, unmount } = renderHook(
    () => useCanvasAudioModelCatalog(identity),
    { wrapper },
  );
  await waitFor(() => expect(result.current.catalog.status).toBe("ready"));
  act(() => {
    mocks.generation++;
    mocks.listeners.forEach((listener) => listener());
  });
  await waitFor(() => expect(result.current.catalog.status).toBe("loading"));
  expect(mocks.read).toHaveBeenCalledTimes(2);
  unmount();
});
