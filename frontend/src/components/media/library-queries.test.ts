import { beforeEach, expect, it, vi } from "vitest";
import * as api from "@/gen/api/media";
import {
  listLibrary,
  applyLibraryIntent,
  getLibraryDetail,
  libraryKey,
  previewLibrary,
  downloadLibraryOriginal,
} from "./library-queries";
import { defaultLibraryFilter } from "./library-model";
import { animatedGIFBytes } from "./gif-test-fixtures";
const sdk = vi.hoisted(() => ({
  download: vi.fn<(...args: unknown[]) => Promise<unknown>>(),
}));
vi.mock("@/gen/api/media", () => ({
  listMediaLibrary: vi.fn(),
  applyMediaLibraryCommand: vi.fn(),
  getMediaLibraryItem: vi.fn(),
  uploadPersonalMediaAsset: vi.fn(),
  previewLibraryMediaAsset: vi.fn(),
  downloadLibraryMediaAsset: sdk.download,
}));
const identity = {
  origin: "http://127.0.0.1:3000",
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  libraryId: "33333333-3333-5333-8333-333333333333",
  scope: { kind: "personal" as const },
};
const page = {
  current_actor_id: identity.actorId,
  current_org_id: identity.orgId,
  library_id: identity.libraryId,
  scope: identity.scope,
  revision: 0,
  page: 3,
  page_size: 40,
  total: 0,
  items: [],
  folders: [],
  category_counts: {},
  folder_counts: {},
};
beforeEach(() => vi.resetAllMocks());

it("GIF原件预览及下载保持全动画身份，首帧PNG只在renditions，不借同ID偷换内容", async () => {
  const bytes = animatedGIFBytes();
  const asset = {
    id: identity.actorId,
    kind: "image" as const,
    file_name: "three-colors.gif",
    mime_type: "image/gif",
    byte_size: bytes.length,
    width: 64,
    height: 64,
    duration_ms: null,
    revision: 1,
  };
  const expires = new Date(Date.now() + 60000).toISOString();
  const body = {
    asset,
    url: "https://private.invalid/whole.gif",
    expires_at: expires,
    renditions: [
      {
        kind: "thumb_256" as const,
        url: "https://private.invalid/first-frame.png",
        expires_at: expires,
        width: 64,
        height: 64,
      },
    ],
  };
  vi.mocked(api.previewLibraryMediaAsset).mockResolvedValue(body);
  const preview = await previewLibrary(identity, asset);
  expect(preview.url).toBe(body.url);
  expect(preview.renditions[0].url).toBe(body.renditions[0].url);
  for (const changed of [
    { mime_type: "image/png" },
    { byte_size: bytes.length - 1 },
  ]) {
    vi.mocked(api.previewLibraryMediaAsset).mockResolvedValue({
      ...body,
      asset: { ...asset, ...changed },
    });
    await expect(previewLibrary(identity, asset)).rejects.toMatchObject({
      status: 502,
    });
  }
  sdk.download.mockResolvedValue(new Blob([bytes], { type: "image/gif" }));
  const download = await downloadLibraryOriginal(identity, asset);
  expect(download.blob.size).toBe(339);
  expect(download.blob.type).toBe("image/gif");
  expect(download.sha256).toBe(
    "1c39e96ef7998d8b8f507f95b74b9028cabcd19532ced0331e493a1301b9bba1",
  );
});
it("完整filter在服务端分页前提交，query缓存含origin/actor/org/library", async () => {
  vi.mocked(api.listMediaLibrary).mockResolvedValue(page);
  const query = {
    ...defaultLibraryFilter(identity.scope),
    page: 3,
    search: "全库",
    category: "material" as const,
    folder: "root",
    favorite_only: true,
    recent_only: true,
    order: "name_asc" as const,
  };
  const signal = new AbortController().signal;
  await listLibrary(identity.scope, query, signal, identity);
  expect(api.listMediaLibrary).toHaveBeenCalledWith(
    {
      scope: "personal",
      page: 3,
      page_size: 40,
      search: "全库",
      category: "material",
      root_only: true,
      favorite_only: true,
      recent_only: true,
      catalog_state: "active",
      order: "name_asc",
    },
    { signal },
  );
  expect(libraryKey(identity)).toEqual([
    "media-library",
    identity.origin,
    identity.actorId,
    identity.orgId,
    identity.libraryId,
    "personal",
  ]);
});
it("非法写回执作为unknown，旧202/200永远只核原body不覆盖新缓存", async () => {
  const body = {
    scope: identity.scope,
    action: "delete_folder" as const,
    expected_revision: 4,
    folder_id: identity.actorId,
    expected_folder_revision: 1,
  };
  const intent = {
    version: 1 as const,
    key: identity.orgId,
    ...identity,
    body,
  };
  const invalid = { ...page, revision: 5 };
  vi.mocked(api.applyMediaLibraryCommand).mockResolvedValue(invalid);
  await expect(applyLibraryIntent(intent)).rejects.toMatchObject({
    status: 502,
  });
  vi.mocked(api.applyMediaLibraryCommand).mockResolvedValue({
    current_actor_id: identity.actorId,
    current_org_id: identity.orgId,
    library_id: identity.libraryId,
    scope: identity.scope,
    revision: 5,
    items: [],
  });
  expect((await applyLibraryIntent(intent)).revision).toBe(5);
  expect(api.applyMediaLibraryCommand).toHaveBeenLastCalledWith(body, {
    headers: { "Idempotency-Key": intent.key, Origin: identity.origin },
  });
});
it("详情ID严格匹配且不接受额外私有字段", async () => {
  vi.mocked(api.getMediaLibraryItem).mockResolvedValue({
    id: identity.actorId,
  });
  await expect(
    getLibraryDetail(identity, identity.actorId),
  ).rejects.toMatchObject({ status: 502 });
});
it("预览严格绑定asset身份与revision、有限lease/闭合renditions，不缓存私有URL", async () => {
  const asset = {
    id: identity.actorId,
    kind: "image" as const,
    file_name: "photo.png",
    mime_type: "image/png",
    byte_size: 4,
    width: 1,
    height: 1,
    duration_ms: null,
    revision: 1,
  };
  const good = {
    asset,
    url: "https://private.invalid/image",
    expires_at: new Date(Date.now() + 60000).toISOString(),
    renditions: [],
  };
  vi.mocked(api.previewLibraryMediaAsset).mockResolvedValue(good);
  expect((await previewLibrary(identity, asset)).asset.id).toBe(asset.id);
  for (const invalid of [
    { ...good, asset: { ...asset, revision: 2 } },
    { ...good, url: "https://user:secret@private.invalid/image" },
    { ...good, expires_at: new Date(Date.now() - 1000).toISOString() },
    {
      ...good,
      renditions: [
        {
          kind: "thumbnail",
          url: good.url,
          expires_at: good.expires_at,
          width: 1,
          height: 1,
        },
      ],
    },
  ]) {
    const sdk = vi.mocked(api.previewLibraryMediaAsset);
    sdk.mockImplementation(async () => invalid);
    await expect(previewLibrary(identity, asset)).rejects.toMatchObject({
      status: 502,
    });
  }
});
it("attachment下载实际全SHA，容量/MIME/unsafe filename失败，不拿200当可用原件", async () => {
  const asset = {
    id: identity.actorId,
    kind: "document" as const,
    file_name: "original.txt",
    mime_type: "text/plain",
    byte_size: 4,
    width: null,
    height: null,
    duration_ms: null,
    revision: 1,
  };
  const response = new Blob(["real"], { type: "text/plain" });
  sdk.download.mockResolvedValue(response);
  const result = await downloadLibraryOriginal(identity, asset);
  expect(result.sha256).toBe(
    "aa33996d60e89311b4d1a920dae03c6d7fa3ae1956c52662e273aad4683e577f",
  );
  for (const bad of [
    { ...asset, byte_size: 3 },
    { ...asset, mime_type: "application/pdf" },
    { ...asset, file_name: "../original.txt" },
  ])
    await expect(downloadLibraryOriginal(identity, bad)).rejects.toMatchObject({
      status: 502,
    });
  expect(api.downloadLibraryMediaAsset).toHaveBeenCalledWith(
    { item_id: asset.id, scope: "personal" },
    expect.objectContaining({ responseType: "blob" }),
  );
});
