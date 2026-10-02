import { expect, it } from "vitest";
import {
  clearFileImportIntent,
  loadFileImportIntent,
  saveFileImportIntent,
} from "./file-import-intent";
const id = (n: number) =>
  `${String(n).repeat(8)}-${String(n).repeat(4)}-4${String(n).repeat(3)}-8${String(n).repeat(3)}-${String(n).repeat(12)}`;
const original = {
  version: 1 as const,
  origin: "http://127.0.0.1:3000",
  actorId: id(1),
  orgId: id(2),
  projectId: id(3),
  key: id(4),
  action: "create" as const,
  body: {
    expected_revision: 5,
    base_version_id: id(5),
    rights_confirmed: true as const,
    asset_ids: [id(6), id(7)],
  },
};
it("跨刷新只存原key/正文与actor org scope，原序不变且拒覆盖不同键", () => {
  const values = new Map<string, string>();
  const storage = {
    getItem: (key: string) => values.get(key) ?? null,
    setItem: (key: string, value: string) => {
      values.set(key, value);
    },
    removeItem: (key: string) => {
      values.delete(key);
    },
  };
  saveFileImportIntent(storage, original);
  expect(loadFileImportIntent(storage, original)).toEqual(original);
  expect(
    loadFileImportIntent(storage, { ...original, actorId: id(9) }),
  ).toBeNull();
  expect(() =>
    saveFileImportIntent(storage, { ...original, key: id(8) }),
  ).toThrow();
  clearFileImportIntent(storage, original);
  expect(loadFileImportIntent(storage, original)).toBeNull();
});
it("持久存储读写失败闭合，未读回原体不允许提交", () => {
  const storage = {
    getItem: () => null,
    setItem: () => {},
    removeItem: () => {},
  };
  expect(() => saveFileImportIntent(storage, original)).toThrow();
  expect(() =>
    loadFileImportIntent({ ...storage, getItem: () => "{}" }, original),
  ).toThrow();
});
