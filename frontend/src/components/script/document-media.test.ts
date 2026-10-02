import { expect, it, vi } from "vitest";
import * as media from "@/gen/api/media";
import {
  readDocumentPage,
  uploadDocument,
  readDocumentUpload,
  findDocument,
} from "./document-media";
vi.mock("@/gen/api/media", () => ({
  uploadMediaAsset: vi.fn(),
  listMediaAssets: vi.fn(),
  downloadDocumentAsset: vi.fn(),
}));
const pid = "11111111-1111-4111-8111-111111111111";
const id = "22222222-2222-4222-8222-222222222222";
const asset = {
  id,
  project_id: pid,
  kind: "document",
  file_name: "原件.txt",
  mime_type: "text/plain",
  byte_size: 7,
  revision: 1,
};
it("历史原件按确切身份分页查找再下载，重复cursor拒绝而不猜来源", async () => {
  const scope = {
    origin: "http://127.0.0.1:3000",
    projectId: pid,
    actorId: id,
    orgId: id,
  };
  vi.mocked(media.listMediaAssets)
    .mockResolvedValueOnce({ items: [], next_cursor: pid })
    .mockResolvedValueOnce(
      JSON.parse(JSON.stringify({ items: [asset], next_cursor: null })),
    );
  expect(await findDocument(scope, id)).toEqual(asset);
  vi.mocked(media.listMediaAssets).mockResolvedValue({
    items: [],
    next_cursor: pid,
  });
  await expect(findDocument(scope, id)).rejects.toThrow();
});
it("文档列表只接受当前项目的有界TXT/DOCX，拒外来项目/可渲染素材/重复ID", () => {
  expect(
    readDocumentPage({ items: [asset], next_cursor: null }, pid).items[0],
  ).toEqual(asset);
  for (const item of [
    { ...asset, project_id: id },
    { ...asset, kind: "image" },
    { ...asset, mime_type: "text/html" },
    { ...asset, byte_size: 20 * 1024 * 1024 + 1 },
  ])
    expect(() =>
      readDocumentPage({ items: [item], next_cursor: null }, pid),
    ).toThrow();
  expect(() =>
    readDocumentPage({ items: [asset, asset], next_cursor: null }, pid),
  ).toThrow();
  expect(() => readDocumentUpload({ asset, duplicate_of: pid }, pid)).toThrow();
});
it("通过正式生成上传消费者提交确切原文件/原键并拒绝失效成功DTO", async () => {
  const file = new File(["原件!"], "原件.txt", { type: "text/plain" });
  vi.mocked(media.uploadMediaAsset).mockResolvedValue({
    asset,
    duplicate_of: undefined,
  });
  expect(
    await uploadDocument(
      pid,
      file,
      id,
      "http://127.0.0.1:3000",
      new AbortController().signal,
      vi.fn(),
    ),
  ).toEqual(asset);
  expect(media.uploadMediaAsset).toHaveBeenCalledWith(
    { pid },
    { local_review_confirmed: true },
    file,
    expect.objectContaining({
      headers: expect.objectContaining({
        "Idempotency-Key": id,
        Origin: "http://127.0.0.1:3000",
      }),
    }),
  );
  vi.mocked(media.uploadMediaAsset).mockResolvedValue({
    asset: { ...asset, kind: "video" },
  });
  await expect(
    uploadDocument(
      pid,
      file,
      id,
      "http://127.0.0.1:3000",
      new AbortController().signal,
      vi.fn(),
    ),
  ).rejects.toThrow();
});
