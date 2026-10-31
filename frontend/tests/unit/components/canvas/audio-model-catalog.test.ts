import { beforeEach, expect, it, vi } from "vitest";
const mocks = vi.hoisted(() => ({
  list: vi.fn(),
  scope: "scope",
  sessionGeneration: 1,
  scopeGeneration: 1,
}));
vi.mock("@/api/models", () => ({ listProjectModels: mocks.list }));
vi.mock("@/lib/request", () => ({
  getCreationScope: () => mocks.scope,
  getSessionGeneration: () => mocks.sessionGeneration,
  getScopeGeneration: () => mocks.scopeGeneration,
}));
import { readCanvasAudioModelCatalog } from "@/components/canvas/lib/canvas-audio-model-catalog";
const identity = { projectID: "project", scopeID: "scope" };
beforeEach(() => {
  mocks.list.mockReset();
  mocks.scope = "scope";
  mocks.scopeGeneration = 1;
  mocks.sessionGeneration = 1;
});
it("reads every formal audio catalog page in stable order with project identity and cancellation", async () => {
  mocks.list
    .mockResolvedValueOnce({ items: [{ id: "one" }], next_cursor: "next" })
    .mockResolvedValueOnce({ items: [{ id: "two" }] });
  const signal = new AbortController().signal;
  expect(await readCanvasAudioModelCatalog(identity, signal)).toEqual({
    ...identity,
    status: "ready",
    models: [{ id: "one" }, { id: "two" }],
  });
  expect(mocks.list).toHaveBeenNthCalledWith(
    1,
    { project_id: "project", capability: "audio.generate", limit: 100 },
    { signal },
  );
  expect(mocks.list).toHaveBeenNthCalledWith(
    2,
    {
      project_id: "project",
      capability: "audio.generate",
      limit: 100,
      cursor: "next",
    },
    { signal },
  );
});
it("does not publish a partially read registry or repeated cursor", async () => {
  mocks.list.mockResolvedValue({ items: [], next_cursor: "same" });
  await expect(readCanvasAudioModelCatalog(identity)).rejects.toThrow();
  expect(mocks.list).toHaveBeenCalledTimes(2);
  mocks.list
    .mockReset()
    .mockResolvedValueOnce({ items: [], next_cursor: "next" })
    .mockRejectedValueOnce(new Error("offline"));
  await expect(readCanvasAudioModelCatalog(identity)).rejects.toThrow(
    "offline",
  );
});
it.each(["scope", "scopeGeneration", "sessionGeneration"] as const)(
  "rejects stale %s responses",
  async (field) => {
    mocks.list.mockImplementation(async () => {
      if (field === "scope") mocks.scope = "other";
      else mocks[field] += 1;
      return { items: [] };
    });
    await expect(readCanvasAudioModelCatalog(identity)).rejects.toThrow();
  },
);
it("rejects a wrong scope before reading, malformed receipt, and caller cancellation", async () => {
  mocks.scope = "other";
  await expect(readCanvasAudioModelCatalog(identity)).rejects.toThrow();
  expect(mocks.list).not.toHaveBeenCalled();
  mocks.scope = "scope";
  mocks.list.mockResolvedValue({});
  await expect(readCanvasAudioModelCatalog(identity)).rejects.toThrow();
  const controller = new AbortController();
  controller.abort();
  mocks.list.mockClear();
  await expect(
    readCanvasAudioModelCatalog(identity, controller.signal),
  ).rejects.toThrow();
  expect(mocks.list).not.toHaveBeenCalled();
});
