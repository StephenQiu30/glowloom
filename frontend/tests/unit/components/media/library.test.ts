import { expect, it } from "vitest";
import {
  requireLibraryPage,
  requireLibraryReceipt,
  validateUpload,
} from "@/components/media/library";
const actor = "11111111-1111-4111-8111-111111111111",
  library = "22222222-2222-4222-8222-222222222222";
const page = {
  current_actor_id: actor,
  current_org_id: library,
  library_id: library,
  scope: { kind: "personal", project_id: null },
  revision: 1,
  page: 1,
  page_size: 40,
  total: 0,
  items: [],
  folders: [],
};
it("拒绝跨账号、非个人范围及不完整回执", () => {
  expect(requireLibraryPage(page, actor).items).toEqual([]);
  expect(() =>
    requireLibraryPage({ ...page, current_actor_id: library }, actor),
  ).toThrow();
  expect(() =>
    requireLibraryPage({ ...page, scope: { kind: "project" } }, actor),
  ).toThrow();
  expect(() => requireLibraryPage({ ...page, total: -1 }, actor)).toThrow();
});
it("只接受同一素材库的新版本命令回执", () => {
  const p = requireLibraryPage(page, actor);
  expect(() =>
    requireLibraryReceipt({ ...page, revision: 2 }, p),
  ).not.toThrow();
  expect(() => requireLibraryReceipt({ ...page, revision: 1 }, p)).toThrow();
  expect(() =>
    requireLibraryReceipt({ ...page, revision: 2, library_id: actor }, p),
  ).toThrow();
});
it("上传在浏览器侧拒绝空文件、未知格式及超限图片", () => {
  expect(
    validateUpload(new File([], "empty.png", { type: "image/png" })),
  ).toBeTruthy();
  expect(validateUpload(new File(["x"], "script.exe"))).toBeTruthy();
  expect(
    validateUpload(new File(["x"], "image.png", { type: "image/png" })),
  ).toBe("");
});
