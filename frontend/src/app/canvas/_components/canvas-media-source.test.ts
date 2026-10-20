import { beforeEach, expect, it, vi } from "vitest";
import {
  downloadLibraryMediaAsset,
  getMediaPreview,
  listMediaLibrary,
} from "@/api/media";
import { canvasMediaSource } from "./canvas-media-source";
vi.mock("@/api/media", () => ({
  downloadLibraryMediaAsset: vi.fn(),
  getMediaPreview: vi.fn(),
  listMediaLibrary: vi.fn(),
}));
const project = crypto.randomUUID(),
  asset = crypto.randomUUID(),
  item = crypto.randomUUID();
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(getMediaPreview).mockResolvedValue({
    asset: { id: asset, project_id: project, kind: "image" },
  });
});
it("下载使用真实素材库条目身份，不能误用asset UUID；支持跨页定位", async () => {
  vi.mocked(listMediaLibrary)
    .mockResolvedValueOnce({
      scope: { kind: "project", project_id: project },
      page: 1,
      total: 121,
      items: [{ asset_id: crypto.randomUUID() }],
    })
    .mockResolvedValueOnce({
      scope: { kind: "project", project_id: project },
      page: 2,
      total: 121,
      items: [{ id: item, asset_id: asset, catalog_state: "active" }],
    });
  const blob = new Blob(["actual-source"], { type: "image/png" });
  vi.mocked(downloadLibraryMediaAsset).mockResolvedValue(blob);
  const signal = new AbortController().signal;
  expect((await canvasMediaSource(project, asset, signal)).blob).toBe(blob);
  expect(downloadLibraryMediaAsset).toHaveBeenCalledWith(
    { item_id: item, scope: "project", project_id: project },
    { signal, responseType: "blob" },
  );
});
it("跨项目回执或已移至回收站的素材不下载", async () => {
  vi.mocked(listMediaLibrary).mockResolvedValue({
    scope: { kind: "project", project_id: crypto.randomUUID() },
    page: 1,
    total: 1,
    items: [],
  });
  await expect(
    canvasMediaSource(project, asset, new AbortController().signal),
  ).rejects.toThrow("invalid_receipt");
  vi.mocked(listMediaLibrary).mockResolvedValue({
    scope: { kind: "project", project_id: project },
    page: 1,
    total: 1,
    items: [{ id: item, asset_id: asset, catalog_state: "trashed" }],
  });
  await expect(
    canvasMediaSource(project, asset, new AbortController().signal),
  ).rejects.toThrow("不在当前项目");
  expect(downloadLibraryMediaAsset).not.toHaveBeenCalled();
});
