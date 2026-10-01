import { expect, it } from "vitest";
import { ApiError } from "@/lib/request";
import {
  folderName,
  folderPageSchema,
  folderMoveSchema,
  readFolderChange,
  projectPlacementSchema,
} from "./folder-model";

const id = "9f3e948c-550b-47c8-8fa2-f2c31c88927c";
const project = "322f64a1-6ce2-40aa-bf94-6f1f85b8bba4";
const actor = "acafc61c-2bf8-48ba-b6c4-f7a84d95c518";
const org = "1a39f9d1-a43c-4d98-8947-901b385d5457";
const folder = {
  id,
  name: "同名目录",
  cover: null,
  cover_unavailable: false,
  project_count: 2,
  revision: 3,
  is_delete: false,
  delete_time: null,
  create_time: "2026-10-02T00:00:00Z",
  update_time: "2026-10-02T00:00:00Z",
};
const page = {
  current_actor_id: actor,
  current_org_id: org,
  items: [folder],
  next_cursor: null,
};
it("名称按Unicode scalar计数并符合Go空白、NUL和UTF-8边界", () => {
  expect(folderName.parse("\u0085 目录 \u0085")).toBe("目录");
  expect(folderName.parse("😀".repeat(160))).toBe("😀".repeat(160));
  for (const value of ["😀".repeat(161), " \u0085 ", "a\u0000b", "\uD800"]) {
    expect(folderName.safeParse(value).success).toBe(false);
  }
});
it("列表冻结当前scope、真实计数与封面不可用事实，同名UUID独立", () => {
  const second = { ...folder, id: project };
  expect(
    folderPageSchema.parse({ ...page, items: [folder, second] }).items,
  ).toHaveLength(2);
  for (const value of [
    { ...page, current_actor_id: "00000000-0000-0000-0000-000000000000" },
    { ...page, items: [folder, folder] },
    { ...page, items: [{ ...folder, project_count: -1 }] },
    { ...page, items: [{ ...folder, project_count: undefined }] },
    { ...page, items: [{ ...folder, is_delete: true }] },
    {
      ...page,
      items: [
        {
          ...folder,
          cover_unavailable: true,
          cover: { project_id: project, asset_id: id },
        },
      ],
    },
  ])
    expect(folderPageSchema.safeParse(value).success).toBe(false);
  expect(
    folderPageSchema.parse({
      ...page,
      items: [{ ...folder, cover_unavailable: true }],
    }).items[0].cover,
  ).toBeNull();
});
it("移动必须显式null根或完整目录CAS，placement零修订只允许根", () => {
  const move = {
    expected_project_revision: 4,
    expected_placement_revision: 0,
    folder_id: null,
    expected_folder_revision: 0,
  };
  expect(folderMoveSchema.parse(move)).toEqual(move);
  expect(folderMoveSchema.safeParse({ ...move, folder_id: id }).success).toBe(
    false,
  );
  expect(
    folderMoveSchema.safeParse({ ...move, folder_id: undefined }).success,
  ).toBe(false);
  expect(
    folderMoveSchema.safeParse({ ...move, expected_folder_revision: 1 })
      .success,
  ).toBe(false);
  expect(
    folderMoveSchema.parse({
      ...move,
      folder_id: id,
      expected_folder_revision: 3,
    }).folder_id,
  ).toBe(id);
  expect(
    projectPlacementSchema.safeParse({
      project_id: project,
      folder_id: id,
      revision: 0,
    }).success,
  ).toBe(false);
});
it("写回执绑定原目录或项目身份，原回执不作为最新列表事实", () => {
  const intent = {
    action: "patch" as const,
    folderId: id,
    body: { expected_revision: 3, cover: null },
  };
  const { project_count: ignored, ...receipt } = folder;
  void ignored;
  expect(
    readFolderChange(intent, { folder: { ...receipt, revision: 4 } }).folder
      ?.revision,
  ).toBe(4);
  expect(() =>
    readFolderChange(intent, { folder: { ...receipt, id: project } }),
  ).toThrow(ApiError);
  expect(() =>
    readFolderChange(intent, {
      folder: { ...receipt, cover: { project_id: project, asset_id: id } },
    }),
  ).toThrow(ApiError);
  const move = {
    action: "move" as const,
    projectId: project,
    body: {
      expected_project_revision: 4,
      expected_placement_revision: 0,
      folder_id: id,
      expected_folder_revision: 3,
    },
  };
  expect(() =>
    readFolderChange(move, {
      placement: { project_id: id, folder_id: id, revision: 1 },
    }),
  ).toThrow(ApiError);
  expect(() =>
    readFolderChange(move, {
      placement: { project_id: project, folder_id: null, revision: 1 },
    }),
  ).toThrow(ApiError);
  expect(
    readFolderChange(move, {
      folder: receipt,
      placement: { project_id: project, folder_id: id, revision: 1 },
    }).placement?.revision,
  ).toBe(1);
  expect(
    readFolderChange(
      { ...move, body: { ...move.body, expected_placement_revision: 1 } },
      {
        folder: receipt,
        placement: { project_id: project, folder_id: id, revision: 1 },
      },
    ).placement?.revision,
  ).toBe(1);
  expect(() =>
    readFolderChange(
      { action: "recycle", folderId: id, body: { expected_revision: 3 } },
      { folder: receipt, recycled_project_ids: [project] },
    ),
  ).toThrow(ApiError);
  expect(() =>
    readFolderChange(
      { action: "recycle", folderId: id, body: { expected_revision: 3 } },
      {
        folder: {
          ...receipt,
          revision: 4,
          is_delete: true,
          delete_time: "2026-10-02T00:00:00Z",
        },
        recycled_project_ids: [project, project],
      },
    ),
  ).toThrow(ApiError);
});
