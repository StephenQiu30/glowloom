import { beforeEach, expect, it, vi } from "vitest";
import {
  loadLibraryIntent,
  saveLibraryIntent,
  clearLibraryIntent,
  type LibraryIntent,
} from "./library-intent";
const identity = {
  origin: "http://127.0.0.1:3000",
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  libraryId: "33333333-3333-5333-8333-333333333333",
  scope: { kind: "personal" as const },
};
const intent: LibraryIntent = {
  version: 1,
  key: "44444444-4444-4444-8444-444444444444",
  ...identity,
  body: {
    scope: identity.scope,
    action: "create_text",
    expected_revision: 1,
    metadata: {
      title: "未提交正文",
      category: "material",
      tags: [],
      folder_id: null,
      note: "",
      source_label: "",
      favorite: false,
      plain_text: "保留原正文",
    },
  },
};
beforeEach(() => {
  sessionStorage.clear();
  vi.restoreAllMocks();
});
it("跨刷新只保存原键、正文和作用域；换组织、actor、origin或库不重放", () => {
  saveLibraryIntent(sessionStorage, intent);
  expect(loadLibraryIntent(sessionStorage, identity)).toEqual(intent);
  for (const change of [
    { actorId: intent.key },
    { orgId: intent.key },
    { libraryId: intent.key },
    { origin: "http://localhost:3000" },
  ])
    expect(
      loadLibraryIntent(sessionStorage, { ...identity, ...change }),
    ).toBeNull();
  expect(() =>
    saveLibraryIntent(sessionStorage, { ...intent, key: identity.orgId }),
  ).toThrow();
  clearLibraryIntent(sessionStorage, identity);
  expect(loadLibraryIntent(sessionStorage, identity)).toBeNull();
});
it("quota、损坏和写入丢失都会阻断新DML，不覆盖旧原键", () => {
  const broken = {
    getItem: () => null,
    setItem: () => {},
    removeItem: () => {},
  };
  expect(() => saveLibraryIntent(broken, intent)).toThrow();
  const quota = {
    ...broken,
    setItem: () => {
      throw new Error("quota");
    },
  };
  expect(() => saveLibraryIntent(quota, intent)).toThrow();
  saveLibraryIntent(sessionStorage, intent);
  const key = sessionStorage.key(0)!;
  sessionStorage.setItem(
    key,
    JSON.stringify({ ...intent, preview_url: "https://private" }),
  );
  expect(() => loadLibraryIntent(sessionStorage, identity)).toThrow();
});
