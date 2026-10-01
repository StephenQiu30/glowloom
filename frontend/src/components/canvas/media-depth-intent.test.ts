import { expect, it } from "vitest";
import { ApiError } from "@/lib/request";
import {
  clearDepthIntent,
  depthIntentSchema,
  loadDepthIntent,
  saveDepthIntent,
  unknownDepthWrite,
} from "./media-depth-intent";

const scope = {
  origin: "http://127.0.0.1:3000",
  actorId: "8f0ec23e-bab6-4e95-b5f9-c61c782aa4c3",
  orgId: "f680082b-5b4d-4e49-8e15-f54714a80a18",
  projectId: "48dbb9a0-7e81-46a0-9aad-4b9592948d41",
  canvasId: "a87f391f-3d7b-4cb1-84f2-138e4b2b0fe2",
  nodeId: "fc489c5a-e07a-4816-b646-21ea5d9ab011",
};
const intent = {
  ...scope,
  version: 1 as const,
  key: "99fb0be4-b1c3-4941-a23d-60816d75473a",
  action: "create" as const,
  body: { canvas_id: scope.canvasId, node_id: scope.nodeId, revision: 7 },
};
function storage() {
  const entries = new Map<string, string>();
  return {
    entries,
    getItem: (key: string) => entries.get(key) ?? null,
    setItem: (key: string, value: string) => void entries.set(key, value),
    removeItem: (key: string) => void entries.delete(key),
  };
}
it("跨刷新只恢复原 UUID 和完整原正文，不能自动改变来源 CAS", () => {
  const saved = storage();
  saveDepthIntent(saved, intent);
  expect(loadDepthIntent(saved, scope)).toEqual(intent);
  expect([...saved.entries.values()][0]).not.toContain("preview");
  clearDepthIntent(saved, scope);
  expect(loadDepthIntent(saved, scope)).toBeNull();
});
it("按 origin、主体、组织、项目、画布与节点隔离，不清除其他 scope 的未知请求", () => {
  const saved = storage();
  saveDepthIntent(saved, intent);
  for (const field of [
    "origin",
    "actorId",
    "orgId",
    "projectId",
    "canvasId",
    "nodeId",
  ] as const) {
    const changed = {
      ...scope,
      [field]: field === "origin" ? "https://example.invalid" : intent.key,
    };
    expect(loadDepthIntent(saved, changed)).toBeNull();
  }
  expect(saved.entries.size).toBe(1);
  expect(loadDepthIntent(saved, scope)?.key).toBe(intent.key);
});
it("storage 抛错、静默不写、损坏正文与移除失败均保持明确失败", () => {
  const saved = storage();
  expect(() =>
    saveDepthIntent({ ...saved, setItem: () => {} }, intent),
  ).toThrow(/存储/);
  expect(() =>
    saveDepthIntent(
      {
        ...saved,
        setItem: () => {
          throw new Error("quota");
        },
      },
      intent,
    ),
  ).toThrow(/存储/);
  saveDepthIntent(saved, intent);
  expect(() =>
    clearDepthIntent({ ...saved, removeItem: () => {} }, scope),
  ).toThrow(/存储/);
  const key = [...saved.entries.keys()][0];
  saved.entries.set(key, "not JSON");
  expect(() => loadDepthIntent(saved, scope)).toThrow(/恢复/);
  expect(saved.entries.get(key)).toBe("not JSON");
});
it("不持久 DTO、媒体URL或不同 source/project 正文；审核必须明确 exact SHA + revision", () => {
  expect(
    depthIntentSchema.safeParse({
      ...intent,
      preview_url: "https://example.invalid/output.mp4",
    }).success,
  ).toBe(false);
  expect(
    depthIntentSchema.safeParse({
      ...intent,
      body: { ...intent.body, node_id: intent.key },
    }).success,
  ).toBe(false);
  const review = {
    ...intent,
    action: "review",
    jobId: intent.key,
    body: {
      project_id: scope.projectId,
      revision: 9,
      sha256: "a".repeat(64),
      local_review_confirmed: true,
    },
  };
  expect(depthIntentSchema.safeParse(review).success).toBe(true);
  expect(
    depthIntentSchema.safeParse({
      ...review,
      body: { ...review.body, local_review_confirmed: false },
    }).success,
  ).toBe(false);
  expect(
    depthIntentSchema.safeParse({
      ...review,
      body: { ...review.body, project_id: scope.nodeId },
    }).success,
  ).toBe(false);
});
it("网络、5xx、非法写 DTO 都是未知效果；确定409需要读取最新后另立明确意图", () => {
  for (const error of [
    new TypeError("network"),
    new ApiError(0, "dependency_unavailable"),
    new ApiError(503, "dependency_unavailable"),
    new ApiError(502, "invalid_response"),
  ])
    expect(unknownDepthWrite(error)).toBe(true);
  for (const status of [400, 401, 403, 404, 409, 422, 429])
    expect(
      unknownDepthWrite(new ApiError(status, "media_depth_conflict")),
    ).toBe(false);
});
