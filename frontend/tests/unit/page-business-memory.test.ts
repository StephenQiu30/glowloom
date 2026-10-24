import { afterEach, expect, it, vi } from "vitest";
import { pageBusinessMemory } from "@/lib/page-business-memory";
import { setCreationScope, setCSRFToken } from "@/lib/request";
afterEach(() => {
  pageBusinessMemory.clear();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
it("retains the exact request in this page without reading or writing legacy storage", () => {
  localStorage.setItem("pending", "old-business-marker");
  const read = vi.spyOn(Storage.prototype, "getItem");
  const write = vi.spyOn(Storage.prototype, "setItem");
  expect(pageBusinessMemory.getItem("pending")).toBeNull();
  const original = JSON.stringify({
    id: "original-id",
    body: { text: "未确认中文🌏" },
  });
  pageBusinessMemory.setItem("pending", original);
  expect(pageBusinessMemory.getItem("pending")).toBe(original);
  expect(read).not.toHaveBeenCalled();
  expect(write).not.toHaveBeenCalled();
});
it("scope and account changes release prior page business state", () => {
  setCSRFToken("account-a");
  setCreationScope("team-a");
  pageBusinessMemory.setItem("request", "body");
  setCreationScope("team-b");
  expect(pageBusinessMemory.length).toBe(0);
  pageBusinessMemory.setItem("request", "new-body");
  setCSRFToken("account-b");
  expect(pageBusinessMemory.length).toBe(0);
});
it("a new page module cannot recover an old page's pending request", async () => {
  pageBusinessMemory.setItem("pending", "do-not-replay");
  vi.resetModules();
  const fresh = (await import("@/lib/page-business-memory")).pageBusinessMemory;
  expect(fresh.getItem("pending")).toBeNull();
  expect(pageBusinessMemory.getItem("pending")).toBe("do-not-replay");
});
it("budget rejection does not evict an earlier unresolved request", () => {
  pageBusinessMemory.setItem("pending", "original");
  expect(() =>
    pageBusinessMemory.setItem("large", "x".repeat(8 * 1024 * 1024)),
  ).toThrow("page_memory_budget");
  expect(pageBusinessMemory.getItem("pending")).toBe("original");
});
