import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import {
  clearCopyIntent,
  loadCopyIntent,
  saveCopyIntent,
  sameCopyScope,
  unknownCopyWrite,
  type CopyIntent,
  type CopyScope,
} from "./copy-intent";

const scope: CopyScope = {
  origin: "http://localhost:3000",
  actorId: "b51e05a5-3e86-4685-a624-bf28841d5618",
  orgId: "9c06e9e9-c029-4404-b6c0-68509d031341",
  sourceId: "e61dbdb9-d428-45dc-a034-53f3f1bb1068",
};
const intent: CopyIntent = {
  ...scope,
  version: 1,
  key: "511054be-4f0d-4a9a-b9ad-2a6af5a3979f",
  action: "create",
  body: { expected_revision: 7, target_name: "完整副本" },
};
beforeEach(() => {
  sessionStorage.clear();
  vi.restoreAllMocks();
});
it("未知写入跨刷新保留原 UUID、原正文与 CAS，不保存 DTO 或媒体", () => {
  saveCopyIntent(sessionStorage, intent);
  expect(loadCopyIntent(sessionStorage, scope)).toEqual(intent);
  expect(sessionStorage.length).toBe(1);
  expect(JSON.parse(sessionStorage.getItem(sessionStorage.key(0)!)!)).toEqual(
    intent,
  );
  clearCopyIntent(sessionStorage, scope);
  expect(loadCopyIntent(sessionStorage, scope)).toBeNull();
});
it.each(["origin", "actorId", "orgId", "sourceId"] as const)(
  "%s 变更不恢复或重放其他作用域的意图",
  (field) => {
    saveCopyIntent(sessionStorage, intent);
    const other = {
      ...scope,
      [field]:
        field === "origin"
          ? "https://example.com"
          : "115ff25e-2e2f-4f0a-b71b-567ecaf51f4d",
    };
    expect(sameCopyScope(scope, other)).toBe(false);
    expect(loadCopyIntent(sessionStorage, other)).toBeNull();
    expect(loadCopyIntent(sessionStorage, scope)).toEqual(intent);
  },
);
it("存储读、写、回读不可靠时拒绝继续，不伪装成没有未知意图", () => {
  const blocked = {
    getItem: vi.fn(() => {
      throw new Error("blocked");
    }),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  };
  expect(() => loadCopyIntent(blocked, scope)).toThrow(/浏览器存储/);
  const dropped = {
    getItem: vi.fn(() => null),
    setItem: vi.fn(),
    removeItem: vi.fn(),
  };
  expect(() => saveCopyIntent(dropped, intent)).toThrow(/浏览器存储/);
});
it("存储非法正文不会被静默清空为可重新提交", () => {
  saveCopyIntent(sessionStorage, intent);
  sessionStorage.setItem(
    sessionStorage.key(0)!,
    JSON.stringify({
      ...intent,
      body: { expected_revision: 99, private_key: "synthetic" },
    }),
  );
  expect(() => loadCopyIntent(sessionStorage, scope)).toThrow(/浏览器存储/);
});
it("确定 409 可重新读后形成新意图，断线、服务错误及写入非法 DTO 保留原意图", () => {
  expect(unknownCopyWrite(new ApiError(409, "revision_conflict"))).toBe(false);
  expect(unknownCopyWrite(new ApiError(422, "invalid_request"))).toBe(false);
  for (const failure of [
    new ApiError(0, "dependency_unavailable"),
    new ApiError(503, "dependency_unavailable"),
    new ApiError(502, "invalid_response"),
    new Error("lost"),
  ])
    expect(unknownCopyWrite(failure)).toBe(true);
});
