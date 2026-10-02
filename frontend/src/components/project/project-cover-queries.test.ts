import { beforeEach, expect, it, vi } from "vitest";
import {
  getProjectCoverPreview,
  listProjectCoverImages,
  projectCoverPreviewKey,
} from "./project-cover-queries";
const api = vi.hoisted(() => ({ list: vi.fn(), preview: vi.fn() }));
vi.mock("@/gen/api/media", () => ({
  listMediaAssets: api.list,
  getMediaPreview: api.preview,
}));
const pid = "9817c918-e49d-4dc8-b8b6-c92833b135e4",
  aid = "c13b18f1-cd43-4f35-bef4-f06801746458";
const image = {
  id: aid,
  project_id: pid,
  kind: "image",
  file_name: "主图.png",
  mime_type: "image/png",
  byte_size: 12,
  revision: 1,
};
beforeEach(() => vi.resetAllMocks());
it("正式图片分页接受 document 等非图片但仅选 image，拒跨项目污染", async () => {
  api.list.mockResolvedValue({
    items: [
      image,
      { ...image, id: pid, kind: "document", mime_type: "text/plain" },
    ],
    next_cursor: "next",
  });
  expect(await listProjectCoverImages(pid, "cursor")).toEqual({
    items: [image],
    next_cursor: "next",
  });
  expect(api.list).toHaveBeenCalledExactlyOnceWith(
    { pid, limit: 50, cursor: "cursor" },
    { signal: undefined },
  );
  api.list.mockResolvedValue({
    items: [{ ...image, project_id: aid }],
    next_cursor: null,
  });
  await expect(listProjectCoverImages(pid)).rejects.toMatchObject({
    status: 502,
    code: "invalid_response",
  });
});
it("签名预览核实际 image/asset/project 与有效租约，不接受其他素材和错误身份", async () => {
  const valid = {
    asset: image,
    url: "https://objects.invalid/private-image",
    expires_at: new Date(Date.now() + 600_000).toISOString(),
  };
  api.preview.mockResolvedValue(valid);
  expect(await getProjectCoverPreview(pid, aid)).toEqual(valid);
  for (const change of [
    { asset: { ...image, id: pid } },
    { asset: { ...image, project_id: aid } },
    { asset: { ...image, kind: "video" } },
    { url: "https://user:secret@objects.invalid/x" },
    { expires_at: new Date(0).toISOString() },
  ]) {
    api.preview.mockResolvedValue({ ...valid, ...change });
    await expect(getProjectCoverPreview(pid, aid)).rejects.toBeInstanceOf(
      Error,
    );
  }
});
it("缓存包含当前主体/组织/源/项目/素材，不共享私有 URL", () => {
  const scope = { origin: "http://localhost:3000", actorId: pid, orgId: aid };
  const key = projectCoverPreviewKey(scope, pid, aid);
  for (const other of [
    { ...scope, actorId: aid },
    { ...scope, orgId: pid },
    { ...scope, origin: "http://127.0.0.1:3000" },
  ])
    expect(projectCoverPreviewKey(other, pid, aid)).not.toEqual(key);
  expect(projectCoverPreviewKey(scope, aid, aid)).not.toEqual(key);
});
