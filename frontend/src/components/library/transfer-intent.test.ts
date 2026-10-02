import { beforeEach, expect, it } from "vitest";
import {
  loadTransferIntent,
  saveTransferIntent,
  clearTransferIntent,
} from "./transfer-intent";
const a = "11111111-1111-4111-8111-111111111111",
  o = "22222222-2222-4222-8222-222222222222",
  p = "33333333-3333-4333-8333-333333333333",
  i = "44444444-4444-4444-8444-444444444444";
const identity = {
  origin: window.location.origin,
  actorId: a,
  orgId: o,
  libraryId: i,
  scope: { kind: "personal" as const },
};
const body = {
  source: identity.scope,
  target: { kind: "project" as const, project_id: p },
  items: [{ id: i, revision: 0 }],
  expected_source_revision: 0,
  expected_target_revision: 0,
  expected_project_revision: 1,
  target_folder_id: null,
  expected_folder_revision: 0,
};
const intent = {
  ...identity,
  version: 1 as const,
  key: p,
  action: "create" as const,
  body,
};
beforeEach(() => sessionStorage.clear());
it("跨刷新保持原键完整双scope及CAS，不缓存任务/媒体/URLs", () => {
  saveTransferIntent(sessionStorage, intent);
  expect(loadTransferIntent(sessionStorage, identity)).toEqual(intent);
  expect(
    JSON.stringify(loadTransferIntent(sessionStorage, identity)),
  ).not.toContain("updated_at");
  expect(() =>
    saveTransferIntent(sessionStorage, { ...intent, key: i }),
  ).toThrow();
  clearTransferIntent(sessionStorage, identity);
  expect(loadTransferIntent(sessionStorage, identity)).toBeNull();
});
it("不同actor/org/library/origin不能借用原迁移；损坏与quota失败不忽略", () => {
  saveTransferIntent(sessionStorage, intent);
  for (const scope of [
    { ...identity, actorId: p },
    { ...identity, orgId: p },
    { ...identity, libraryId: p },
    { ...identity, origin: "http://other.test" },
  ])
    expect(loadTransferIntent(sessionStorage, scope)).toBeNull();
  const key = sessionStorage.key(0)!;
  sessionStorage.setItem(key, '{"version":1}');
  expect(() => loadTransferIntent(sessionStorage, identity)).toThrow();
  expect(() =>
    saveTransferIntent(
      {
        getItem: () => null,
        setItem: () => {
          throw new Error("quota");
        },
        removeItem: () => {},
      },
      intent,
    ),
  ).toThrow();
});
