import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { useTimelineMedia } from "@/components/canvas/timeline/use-timeline-media";
import { RequestError } from "@/lib/request";
const fake = vi.hoisted(() => ({
  get: vi.fn(),
  scope: 0,
  session: 0,
  listeners: new Set<() => void>(),
}));
vi.mock("@/api/media", () => ({ getMediaPreview: fake.get }));
vi.mock("@/lib/request", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/request")>()),
  getScopeGeneration: () => fake.scope,
  getSessionGeneration: () => fake.session,
  subscribeContextChange: (fn: () => void) => {
    fake.listeners.add(fn);
    return () => fake.listeners.delete(fn);
  },
}));
const project = "11111111-1111-4111-8111-111111111111";
const asset = "22222222-2222-4222-8222-222222222222";
const receipt = (url = "https://media.example/one.png") => ({
  asset: { id: asset, project_id: project, kind: "image", status: "ready" },
  url,
  expires_at: new Date(Date.now() + 100000).toISOString(),
});
beforeEach(() => {
  vi.useFakeTimers();
  fake.get.mockReset();
  fake.scope = 0;
  fake.session = 0;
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});
it("refreshes before expiration and clears failed media until an explicit retry succeeds", async () => {
  fake.get
    .mockResolvedValueOnce(receipt())
    .mockRejectedValueOnce(new Error("revoked"));
  const view = renderHook(() =>
    useTimelineMedia(project, "canvas/node", JSON.stringify([asset])),
  );
  await act(async () => {});
  expect(view.result.current.media[asset].url).toContain("one.png");
  await act(async () => {
    await vi.advanceTimersByTimeAsync(90000);
  });
  expect(fake.get).toHaveBeenCalledTimes(2);
  expect(view.result.current.media[asset]).toBeUndefined();
  expect(view.result.current.error).toBe("素材读取失败，请重试。");
  fake.get.mockResolvedValueOnce(receipt("https://media.example/new.png"));
  await act(async () => {
    view.result.current.retry();
  });
  expect(view.result.current.media[asset].url).toContain("new.png");
  expect(view.result.current.error).toBe("");
});
it("cancels a stale identity's request and refuses its late response", async () => {
  let resolve!: (value: ReturnType<typeof receipt>) => void;
  fake.get
    .mockReturnValueOnce(
      new Promise((done) => {
        resolve = done;
      }),
    )
    .mockRejectedValueOnce(new Error("new identity"));
  const view = renderHook(() =>
    useTimelineMedia(project, "canvas/node", JSON.stringify([asset])),
  );
  await act(async () => {});
  const signal = fake.get.mock.calls[0][1].signal as AbortSignal;
  await act(async () => {
    fake.session++;
    fake.listeners.forEach((listener) => listener());
  });
  expect(signal.aborted).toBe(true);
  await act(async () => {
    resolve(receipt());
  });
  expect(view.result.current.media).toEqual({});
  expect(view.result.current.error).toBe("素材读取失败，请重试。");
});
it("removes old assets immediately when the requested set changes and cleans up refresh timers", async () => {
  fake.get.mockResolvedValue(receipt());
  const view = renderHook(
    ({ ids }) => useTimelineMedia(project, "canvas/node", ids),
    { initialProps: { ids: JSON.stringify([asset]) } },
  );
  await act(async () => {});
  view.rerender({ ids: "[]" });
  expect(view.result.current.media).toEqual({});
  await act(async () => {});
  view.unmount();
  await act(async () => {
    await vi.advanceTimersByTimeAsync(200000);
  });
  expect(fake.get).toHaveBeenCalledTimes(1);
});
it("stops exposing an expired URL when its refresh has not returned", async () => {
  fake.get
    .mockResolvedValueOnce(receipt())
    .mockReturnValueOnce(new Promise(() => {}));
  const view = renderHook(() =>
    useTimelineMedia(project, "canvas/node", JSON.stringify([asset])),
  );
  await act(async () => {});
  await act(async () => {
    await vi.advanceTimersByTimeAsync(100000);
  });
  expect(view.result.current.media).toEqual({});
  expect(view.result.current.error).toContain("过期");
});

it("isolates a failed asset while retaining successful preview media", async () => {
  const failed = "33333333-3333-4333-8333-333333333333";
  fake.get.mockImplementation(({ asset_id }: { asset_id: string }) =>
    asset_id === asset
      ? Promise.resolve(receipt())
      : Promise.reject(new Error("missing")),
  );
  const view = renderHook(() =>
    useTimelineMedia(project, "canvas/node", JSON.stringify([asset, failed])),
  );
  await act(async () => {});
  expect(view.result.current.media[asset].url).toContain("one.png");
  expect(view.result.current.media[failed]).toBeUndefined();
  expect(view.result.current.errors[failed]).toBe("素材读取失败，请重试。");
});

it("never retains an expired URL when an explicit retry is still pending", async () => {
  fake.get
    .mockResolvedValueOnce(receipt())
    .mockReturnValueOnce(new Promise(() => {}));
  const view = renderHook(() =>
    useTimelineMedia(project, "canvas/node", JSON.stringify([asset])),
  );
  await act(async () => {});
  expect(view.result.current.media[asset]).toBeDefined();
  await act(async () => {
    view.result.current.retry();
  });
  await act(async () => {
    await vi.advanceTimersByTimeAsync(100000);
  });
  expect(view.result.current.media[asset]).toBeUndefined();
});

it("retries only the requested failed asset without interrupting healthy playback", async () => {
  const failed = "33333333-3333-4333-8333-333333333333";
  let recovered = false;
  fake.get.mockImplementation(({ asset_id }: { asset_id: string }) => {
    if (asset_id === asset) return Promise.resolve(receipt());
    if (!recovered) return Promise.reject(new Error("missing"));
    return Promise.resolve({
      ...receipt("https://media.example/recovered.png"),
      asset: { ...receipt().asset, id: failed },
    });
  });
  const view = renderHook(() =>
    useTimelineMedia(project, "canvas/node", JSON.stringify([asset, failed])),
  );
  await act(async () => {});
  const healthy = view.result.current.media[asset];
  recovered = true;
  await act(async () => {
    view.result.current.retry(failed);
  });
  expect(view.result.current.media[asset]).toBe(healthy);
  expect(view.result.current.media[failed].url).toContain("recovered.png");
  expect(
    fake.get.mock.calls.filter(([input]) => input.asset_id === asset),
  ).toHaveLength(1);
  expect(view.result.current.error).toBe("");
});

it("keeps overlapping asset retries and timed refreshes within six concurrent requests", async () => {
  const ids = Array.from(
    { length: 10 },
    (_, index) => `22222222-2222-4222-8222-${String(index).padStart(12, "0")}`,
  );
  const pending: Array<() => void> = [];
  let active = 0,
    maximum = 0;
  fake.get.mockImplementation(({ asset_id }: { asset_id: string }) => {
    active++;
    maximum = Math.max(maximum, active);
    return new Promise((resolve) =>
      pending.push(() => {
        active--;
        resolve({ ...receipt(), asset: { ...receipt().asset, id: asset_id } });
      }),
    );
  });
  const view = renderHook(() =>
    useTimelineMedia(project, "canvas/node", JSON.stringify(ids)),
  );
  await act(async () => {});
  expect(active).toBe(6);
  await act(async () => {
    for (const id of ids) view.result.current.retry(id);
  });
  expect(active).toBe(6);
  while (pending.length)
    await act(async () => {
      pending.splice(0).forEach((done) => done());
    });
  expect(maximum).toBe(6);
  expect(Object.keys(view.result.current.media)).toHaveLength(10);
});
it("translates repeated service failures without exposing internal codes or hiding healthy media", async () => {
  const failedA = "33333333-3333-4333-8333-333333333333";
  const failedB = "44444444-4444-4444-8444-444444444444";
  fake.get.mockImplementation(({ asset_id }: { asset_id: string }) =>
    asset_id === asset
      ? Promise.resolve(receipt())
      : Promise.reject(new RequestError("dependency_unavailable", 503)),
  );
  const view = renderHook(() =>
    useTimelineMedia(
      project,
      "canvas/node",
      JSON.stringify([asset, failedA, failedB]),
    ),
  );
  await act(async () => {});
  expect(view.result.current.media[asset].url).toContain("one.png");
  expect(Object.keys(view.result.current.errors)).toHaveLength(2);
  expect(view.result.current.error).toBe("素材服务暂时不可用，请稍后重试。");
});
