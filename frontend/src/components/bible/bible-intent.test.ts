import { expect, it } from "vitest";
import { ApiError } from "@/lib/request";
import {
  loadBibleIntent,
  saveBibleIntent,
  clearBibleIntent,
  unknownBibleWrite,
} from "./bible-intent";

const identity = {
  origin: "http://localhost:3000",
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  projectId: "33333333-3333-4333-8333-333333333333",
};
const intent = {
  ...identity,
  version: 1 as const,
  key: "44444444-4444-4444-8444-444444444444",
  command: {
    action: "create" as const,
    kind: "character" as const,
    body: {
      expected_revision: 0,
      character: { name: "原正文角色", definition: { role: "原完整输入" } },
    },
  },
};
function storage() {
  const values = new Map<string, string>();
  return {
    values,
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value);
    },
    removeItem: (key: string) => {
      values.delete(key);
    },
  };
}
it("跨刷新只保原UUID/body/CAS与origin actor org project，scope变化无自动借旧意图", () => {
  const local = storage();
  saveBibleIntent(local, intent);
  expect(loadBibleIntent(local, identity)).toEqual(intent);
  for (const scope of [
    { ...identity, origin: "http://127.0.0.1:3000" },
    { ...identity, actorId: intent.key },
    { ...identity, orgId: intent.key },
    { ...identity, projectId: intent.key },
  ])
    expect(loadBibleIntent(local, scope)).toBeNull();
  clearBibleIntent(local, identity);
  expect(loadBibleIntent(local, identity)).toBeNull();
});
it("存储读写失败、写后读不等与损坏闭合数据明确阻断，不返回伪空状态", () => {
  expect(() =>
    saveBibleIntent(
      {
        ...storage(),
        setItem: () => {
          throw new Error("quota");
        },
      },
      intent,
    ),
  ).toThrow(/存储/);
  expect(() =>
    saveBibleIntent({ ...storage(), setItem: () => {} }, intent),
  ).toThrow(/存储/);
  const local = storage();
  saveBibleIntent(local, intent);
  for (const key of local.values.keys())
    local.values.set(
      key,
      JSON.stringify({ ...intent, response: { private_url: "not-persisted" } }),
    );
  expect(() => loadBibleIntent(local, identity)).toThrow(/存储/);
});
it("同scope不可覆盖原未知body/key，非法200与5xx留原意图，409为确定冲突", () => {
  const local = storage();
  saveBibleIntent(local, intent);
  expect(() =>
    saveBibleIntent(local, { ...intent, key: identity.actorId }),
  ).toThrow();
  expect(() =>
    saveBibleIntent(local, {
      ...intent,
      command: {
        ...intent.command,
        body: {
          ...intent.command.body,
          character: { name: "另一次意图", definition: {} },
        },
      },
    }),
  ).toThrow();
  for (const error of [
    new Error("response lost"),
    new ApiError(0, "dependency_unavailable"),
    new ApiError(502, "invalid_bible_response"),
    new ApiError(503, "context_unavailable"),
    new ApiError(408, "timeout"),
  ])
    expect(unknownBibleWrite(error)).toBe(true);
  expect(unknownBibleWrite(new ApiError(409, "revision_conflict"))).toBe(false);
});
