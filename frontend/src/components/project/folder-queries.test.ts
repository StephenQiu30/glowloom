import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import {
  findFolder,
  listFolders,
  runFolderIntent,
  getFolderCoverPreview,
  listCoverImages,
} from "./folder-queries";

const ports = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  patch: vi.fn(),
  recycle: vi.fn(),
  move: vi.fn(),
  media: vi.fn(),
  preview: vi.fn(),
}));
vi.mock("@/api/projects", () => ({
  listProjectFolders: ports.list,
  createProjectFolder: ports.create,
  updateProjectFolder: ports.patch,
  recycleProjectFolder: ports.recycle,
  moveProjectToFolder: ports.move,
}));
vi.mock("@/api/media", () => ({
  listMediaAssets: ports.media,
  getMediaPreview: ports.preview,
}));
const actor = "acafc61c-2bf8-48ba-b6c4-f7a84d95c518";
const org = "1a39f9d1-a43c-4d98-8947-901b385d5457";
const id = "9f3e948c-550b-47c8-8fa2-f2c31c88927c";
const project = "322f64a1-6ce2-40aa-bf94-6f1f85b8bba4";
const scope = { origin: "http://127.0.0.1:3000", actorId: actor, orgId: org };
const folder = {
  id,
  name: "目录",
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
beforeEach(() => vi.resetAllMocks());
it("通过正式生成操作读25条目录且完整验证scope和数量", async () => {
  const signal = new AbortController().signal;
  ports.list.mockResolvedValue(page);
  expect(await listFolders("cursor", signal)).toEqual(page);
  expect(ports.list).toHaveBeenCalledExactlyOnceWith(
    { limit: 25, cursor: "cursor" },
    { signal },
  );
  ports.list.mockResolvedValue({
    ...page,
    items: Array.from({ length: 26 }, (_, i) => ({
      ...folder,
      id: `00000000-0000-4000-8000-${String(i + 1).padStart(12, "0")}`,
    })),
  });
  await expect(listFolders()).rejects.toBeInstanceOf(ApiError);
});
it("按真实cursor读取晚页目录，跨scope或循环游标拒绝，不以首页缺项冒充404", async () => {
  ports.list
    .mockResolvedValueOnce({ ...page, items: [], next_cursor: "later" })
    .mockResolvedValueOnce(page);
  expect(await findFolder(id, scope)).toEqual(folder);
  expect(ports.list.mock.calls[1][0].cursor).toBe("later");
  ports.list.mockResolvedValue({ ...page, current_actor_id: id });
  await expect(findFolder(id, scope)).rejects.toMatchObject({
    code: "scope_changed",
  });
  ports.list.mockResolvedValue({ ...page, items: [], next_cursor: "same" });
  await expect(findFolder(id, scope)).rejects.toMatchObject({
    code: "invalid_response",
  });
});
it("4种写命令使用原UUID正文Origin，只返回原回执而不替换新列表", async () => {
  const base = { ...scope, version: 1 as const, key: project };
  const options = {
    headers: { "Idempotency-Key": project, Origin: scope.origin },
  };
  const receipt = { folder: { ...folder, revision: 4 } };
  ports.create.mockResolvedValue({ folder });
  await runFolderIntent({ ...base, action: "create", body: { name: "目录" } });
  expect(ports.create).toHaveBeenCalledExactlyOnceWith(
    { name: "目录" },
    options,
  );
  ports.recycle.mockResolvedValue({
    folder: {
      ...folder,
      revision: 4,
      is_delete: true,
      delete_time: "2026-10-02T00:01:00Z",
    },
    recycled_project_ids: [project],
  });
  await runFolderIntent({
    ...base,
    action: "recycle",
    folderId: id,
    body: { expected_revision: 3 },
  });
  expect(ports.recycle).toHaveBeenCalledExactlyOnceWith(
    { folder_id: id },
    { expected_revision: 3 },
    options,
  );
  const moveBody = {
    expected_project_revision: 8,
    expected_placement_revision: 2,
    folder_id: null,
    expected_folder_revision: 0,
  };
  ports.move.mockResolvedValue({
    placement: { project_id: project, folder_id: null, revision: 3 },
  });
  await runFolderIntent({
    ...base,
    action: "move",
    projectId: project,
    body: moveBody,
  });
  expect(ports.move).toHaveBeenCalledExactlyOnceWith(
    { pid: project },
    moveBody,
    options,
  );
  ports.patch.mockResolvedValue(receipt);
  await runFolderIntent({
    ...base,
    action: "patch",
    folderId: id,
    body: { expected_revision: 3, cover: null },
  });
  expect(ports.patch).toHaveBeenCalledExactlyOnceWith(
    { folder_id: id },
    { expected_revision: 3, cover: null },
    { headers: { "Idempotency-Key": project, Origin: scope.origin } },
  );
  ports.patch.mockResolvedValue({
    folder: { ...folder, id: project, revision: 4 },
  });
  await expect(
    runFolderIntent({
      ...base,
      action: "patch",
      folderId: id,
      body: { expected_revision: 3, cover: null },
    }),
  ).rejects.toMatchObject({ code: "invalid_response" });
});
it("封面只从正式image列表选取并绑定授权预览身份和有效期", async () => {
  const image = {
    id,
    project_id: project,
    kind: "image",
    file_name: "cover.png",
    mime_type: "image/png",
    byte_size: 200,
    width: 64,
    height: 64,
    revision: 1,
  };
  ports.media.mockResolvedValue({
    items: [
      image,
      { ...image, id: actor, kind: "video", mime_type: "video/mp4" },
    ],
    next_cursor: "next",
  });
  expect((await listCoverImages(project)).items).toEqual([image]);
  ports.preview.mockResolvedValue({
    asset: image,
    url: "https://example.test/private",
    expires_at: new Date(Date.now() + 60000).toISOString(),
  });
  expect(
    (await getFolderCoverPreview({ project_id: project, asset_id: id })).asset
      .id,
  ).toBe(id);
  ports.preview.mockResolvedValue({
    asset: { ...image, project_id: actor },
    url: "https://example.test/private",
    expires_at: new Date(Date.now() + 60000).toISOString(),
  });
  await expect(
    getFolderCoverPreview({ project_id: project, asset_id: id }),
  ).rejects.toMatchObject({ code: "invalid_response" });
});
