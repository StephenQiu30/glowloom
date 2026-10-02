import { beforeEach, expect, it } from "vitest";
import {
  loadDocumentUpload,
  saveDocumentUpload,
  clearDocumentUpload,
} from "./document-upload-intent";
const scope = {
  origin: "http://127.0.0.1:3000",
  projectId: "11111111-1111-4111-8111-111111111111",
  actorId: "22222222-2222-4222-8222-222222222222",
  orgId: "33333333-3333-4333-8333-333333333333",
};
const intent = {
  ...scope,
  version: 1 as const,
  key: "44444444-4444-4444-8444-444444444444",
  fileName: "原件.txt",
  byteSize: 9,
  sha256: "a".repeat(64),
  localReviewConfirmed: true as const,
};
beforeEach(() => sessionStorage.clear());
it("未知文档上传刷新恢复同原键/文件指纹，scope隔离且不缓存文件/对象URL", () => {
  saveDocumentUpload(sessionStorage, intent);
  expect(loadDocumentUpload(sessionStorage, scope)).toEqual(intent);
  expect(
    loadDocumentUpload(sessionStorage, { ...scope, actorId: intent.key }),
  ).toBeNull();
  expect(
    loadDocumentUpload(sessionStorage, {
      ...scope,
      origin: "http://localhost:3000",
    }),
  ).toBeNull();
  const stored = sessionStorage.getItem(sessionStorage.key(0)!)!;
  expect(Object.keys(JSON.parse(stored)).sort()).toEqual(
    Object.keys(intent).sort(),
  );
  clearDocumentUpload(sessionStorage, scope);
  expect(loadDocumentUpload(sessionStorage, scope)).toBeNull();
});
it("quota/readback损坏时拒绝宣称存储成功，不发送新上传意图", () => {
  const failing = {
    getItem: () => null,
    setItem: () => {
      throw new Error("quota");
    },
    removeItem: () => {},
  };
  expect(() => saveDocumentUpload(failing, intent)).toThrow(/存储/);
  saveDocumentUpload(sessionStorage, intent);
  sessionStorage.setItem(
    sessionStorage.key(0)!,
    JSON.stringify({ ...intent, actorId: intent.key }),
  );
  expect(() => loadDocumentUpload(sessionStorage, scope)).toThrow(/存储/);
});
