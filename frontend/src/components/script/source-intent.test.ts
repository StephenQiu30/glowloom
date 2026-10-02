import { expect, it } from "vitest";
import { ApiError } from "@/lib/request";
import {
  clearSourceIntent,
  loadSourceIntent,
  saveSourceIntent,
  sameScriptScope,
  unknownSourceWrite,
} from "./source-intent";
const scope = {
  origin: "http://127.0.0.1:3000",
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  projectId: "33333333-3333-4333-8333-333333333333",
};
const intent = {
  ...scope,
  version: 1 as const,
  key: "44444444-4444-4444-8444-444444444444",
  action: "create" as const,
  body: {
    expected_revision: 0,
    rights_confirmed: true as const,
    source_kind: "chapter" as const,
    title: "第一章",
    status: "draft" as const,
    document: {
      type: "doc" as const,
      content: [
        {
          type: "paragraph" as const,
          content: [{ type: "text" as const, text: "😀正文" }],
        },
      ],
    },
    provenance: {},
  },
};
function storage() {
  const data = new Map<string, string>();
  return {
    data,
    getItem: (key: string) => data.get(key) ?? null,
    setItem: (key: string, value: string) => {
      data.set(key, value);
    },
    removeItem: (key: string) => {
      data.delete(key);
    },
  };
}
it("刷新恢复原key/CAS/完整body，严格origin/actor/org/project隔离", () => {
  const store = storage();
  saveSourceIntent(store, intent);
  expect(loadSourceIntent(store, scope)).toEqual(intent);
  for (const changed of [
    { ...scope, origin: "http://localhost:3000" },
    { ...scope, actorId: scope.orgId },
    { ...scope, orgId: scope.actorId },
    { ...scope, projectId: scope.actorId },
  ]) {
    expect(sameScriptScope(changed, scope)).toBe(false);
    expect(loadSourceIntent(store, changed)).toBeNull();
  }
  clearSourceIntent(store, scope);
  expect(loadSourceIntent(store, scope)).toBeNull();
});
it("持久失败、损坏或伪造scope保留恢复错误，不声称可以发送新DML", () => {
  const store = storage();
  expect(() =>
    saveSourceIntent({ ...store, setItem: () => {} }, intent),
  ).toThrow(/存储/);
  expect(() =>
    saveSourceIntent(
      {
        ...store,
        setItem: () => {
          throw new Error("blocked");
        },
      },
      intent,
    ),
  ).toThrow(/存储/);
  saveSourceIntent(store, intent);
  const key = [...store.data.keys()][0];
  store.data.set(key, JSON.stringify({ ...intent, actorId: scope.orgId }));
  expect(() => loadSourceIntent(store, scope)).toThrow(/恢复/);
  store.data.set(key, "not-json");
  expect(() => loadSourceIntent(store, scope)).toThrow(/恢复/);
});
it("网络/非法成功DTO/依赖503为unknown，确定409可保留草稿后读取新事实", () => {
  for (const error of [
    new Error("network"),
    new ApiError(0, "network_error"),
    new ApiError(502, "invalid_response"),
    new ApiError(503, "needs_reconciliation"),
  ])
    expect(unknownSourceWrite(error)).toBe(true);
  for (const status of [400, 401, 403, 404, 409, 413, 415, 422, 429])
    expect(unknownSourceWrite(new ApiError(status, "rejected"))).toBe(false);
});
