import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import {
  clearFolderIntent,
  loadFolderIntent,
  saveFolderIntent,
  unknownFolderWrite,
} from "./folder-intent";

const scope = {
  origin: "http://127.0.0.1:3000",
  actorId: "acafc61c-2bf8-48ba-b6c4-f7a84d95c518",
  orgId: "1a39f9d1-a43c-4d98-8947-901b385d5457",
};
const intent = {
  ...scope,
  version: 1 as const,
  key: "b663726e-1e9a-4da8-8b73-9c0b3bf4d7ab",
  action: "patch" as const,
  folderId: "9f3e948c-550b-47c8-8fa2-f2c31c88927c",
  body: { expected_revision: 3, cover: null },
};
beforeEach(() => sessionStorage.clear());
it("刷新保留原UUID、正文和旧CAS，身份或组织变化不读取旧意图", () => {
  saveFolderIntent(sessionStorage, intent);
  expect(loadFolderIntent(sessionStorage, scope)).toEqual(intent);
  expect(
    loadFolderIntent(sessionStorage, { ...scope, actorId: intent.folderId }),
  ).toBeNull();
  expect(
    loadFolderIntent(sessionStorage, { ...scope, orgId: intent.folderId }),
  ).toBeNull();
  expect(
    loadFolderIntent(sessionStorage, {
      ...scope,
      origin: "http://localhost:3000",
    }),
  ).toBeNull();
  expect(sessionStorage.getItem(sessionStorage.key(0)!)).not.toContain("url");
});
it("存储写入读回失败、损坏或不能清除都阻止安全发送新意图", () => {
  const storage = {
    getItem: vi.fn().mockReturnValue(null),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  };
  expect(() => saveFolderIntent(storage, intent)).toThrow(/存储/);
  saveFolderIntent(sessionStorage, intent);
  sessionStorage.setItem(sessionStorage.key(0)!, "{}");
  expect(() => loadFolderIntent(sessionStorage, scope)).toThrow(/存储/);
  storage.getItem.mockReturnValue(JSON.stringify(intent));
  expect(() => clearFolderIntent(storage, scope)).toThrow(/存储/);
});
it("不持久DTO、媒体URL或其他字段；503及非法成功回执为unknown，409确定拒绝", () => {
  expect(() =>
    saveFolderIntent(sessionStorage, {
      ...intent,
      url: "https://private.example/media",
    } as typeof intent),
  ).toThrow();
  expect(unknownFolderWrite(new ApiError(503, "dependency_unavailable"))).toBe(
    true,
  );
  expect(unknownFolderWrite(new ApiError(502, "invalid_response"))).toBe(true);
  expect(unknownFolderWrite(new Error("network"))).toBe(true);
  expect(unknownFolderWrite(new ApiError(409, "revision_conflict"))).toBe(
    false,
  );
});
