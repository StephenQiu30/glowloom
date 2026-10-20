import { afterEach, expect, it, vi } from "vitest";
import {
  developmentMediaURL,
  developmentJournal,
  developmentResponse,
  installDevelopmentTransport,
} from "@/lib/development-transport";
afterEach(() => {
  vi.unstubAllEnvs();
  window.history.replaceState({}, "", "/");
});
it("开发Mock仅匹配显式入口，离开或卸载后正式请求不受影响", async () => {
  const handler = vi.fn(async () => ({ mock: true }));
  const journal = { checkpoint: async () => null };
  const remove = installDevelopmentTransport({
    matches: (path) => path === "/canvas/preview",
    request: handler,
    journal,
  });
  try {
    expect(developmentResponse("/api/session", {})).toBeNull();
    expect(developmentJournal()).toBeNull();
    window.history.replaceState({}, "", "/canvas/preview");
    expect(developmentJournal()).toBe(journal);
    expect(await developmentResponse("/api/session", {})).toEqual({
      mock: true,
    });
    expect(developmentMediaURL(`blob:${location.origin}/preview`)).toBe(true);
    expect(developmentMediaURL("blob:https://another.example/preview")).toBe(
      false,
    );
    remove();
    expect(developmentResponse("/api/session", {})).toBeNull();
    expect(developmentJournal()).toBeNull();
    expect(developmentMediaURL(`blob:${location.origin}/preview`)).toBe(false);
  } finally {
    remove();
  }
});
it("生产环境不能安装Mock；取消的预览请求不会返回迟到回执", async () => {
  vi.stubEnv("NODE_ENV", "production");
  expect(() =>
    installDevelopmentTransport({
      matches: () => true,
      request: async () => null,
    }),
  ).toThrow("disabled");
  vi.stubEnv("NODE_ENV", "test");
  const controller = new AbortController();
  const remove = installDevelopmentTransport({
    matches: () => true,
    request: async () => {
      controller.abort();
      return "late";
    },
  });
  try {
    await expect(
      developmentResponse("/api/session", { signal: controller.signal }),
    ).rejects.toThrow("cancelled");
  } finally {
    remove();
  }
});
