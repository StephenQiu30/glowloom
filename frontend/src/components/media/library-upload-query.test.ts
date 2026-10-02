import { beforeEach, expect, it, vi } from "vitest";
import * as media from "@/gen/api/media";
import { uploadLibraryOriginal } from "./library-upload-query";
import * as queries from "./library-queries";
const sdk = vi.hoisted(() => ({
  personal: vi.fn<(...args: unknown[]) => Promise<unknown>>(),
  project: vi.fn<(...args: unknown[]) => Promise<unknown>>(),
}));
vi.mock("@/gen/api/media", () => ({
  uploadPersonalMediaAsset: sdk.personal,
  uploadMediaAsset: sdk.project,
}));
vi.mock("./library-queries", () => ({ freshLibrary: vi.fn() }));
const identity = {
  origin: window.location.origin,
  actorId: "11111111-1111-4111-8111-111111111111",
  orgId: "22222222-2222-4222-8222-222222222222",
  libraryId: "33333333-3333-5333-8333-333333333333",
  scope: { kind: "personal" as const },
};
const file = new File(["real"], "a.txt", { type: "text/plain" }),
  intent = {
    key: identity.actorId,
    fileName: file.name,
    byteSize: file.size,
    sha256: "a".repeat(64),
    local_review_confirmed: true as const,
  };
const asset = {
  id: identity.orgId,
  kind: "document" as const,
  file_name: file.name,
  mime_type: file.type,
  byte_size: file.size,
  width: null,
  height: null,
  duration_ms: null,
  revision: 1,
};
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(queries.freshLibrary).mockResolvedValue(
    {} as Awaited<ReturnType<typeof queries.freshLibrary>>,
  );
});
it.each([
  ["模型.gltf", "model/gltf+json", "model"],
  ["模型.glb", "model/gltf-binary", "model"],
  ["动画.gif", "image/gif", "image"],
])(
  "%s真实上传回执按原格式与整件字节校验，原File和key不改写",
  async (name, mime, kind) => {
    const original = new File(["original bytes"], name, { type: mime });
    const input = { ...intent, fileName: name, byteSize: original.size };
    const result = {
      ...asset,
      kind,
      mime_type: mime,
      file_name: name,
      byte_size: original.size,
    };
    sdk.personal.mockResolvedValue({ asset: result, duplicate_of: null });
    expect(
      await uploadLibraryOriginal(
        identity,
        original,
        input,
        new AbortController().signal,
        vi.fn(),
      ),
    ).toEqual(result);
    expect(sdk.personal).toHaveBeenCalledWith(
      { local_review_confirmed: true },
      original,
      expect.objectContaining({
        headers: { "Idempotency-Key": input.key, Origin: identity.origin },
      }),
    );
    for (const invalid of [
      { ...result, byte_size: original.size - 1 },
      { ...result, mime_type: "image/png" },
      { ...result, kind: "document" },
    ]) {
      sdk.personal.mockResolvedValue({ asset: invalid, duplicate_of: null });
      await expect(
        uploadLibraryOriginal(
          identity,
          original,
          input,
          new AbortController().signal,
          vi.fn(),
        ),
      ).rejects.toMatchObject({ status: 502 });
    }
  },
);
it("个人上传不用伪项目，原key/Origin/review与原文件送正式generated SDK", async () => {
  vi.mocked(media.uploadPersonalMediaAsset).mockResolvedValue({
    asset,
    duplicate_of: null,
  });
  const signal = new AbortController().signal;
  expect(
    await uploadLibraryOriginal(identity, file, intent, signal, vi.fn()),
  ).toEqual(asset);
  expect(media.uploadPersonalMediaAsset).toHaveBeenCalledWith(
    { local_review_confirmed: true },
    file,
    expect.objectContaining({
      signal,
      headers: { "Idempotency-Key": intent.key, Origin: identity.origin },
    }),
  );
  expect(media.uploadMediaAsset).not.toHaveBeenCalled();
});
it("非法201、额外私有字段、错kind/documentSize作为unknown，project UUID严格匹配", async () => {
  for (const invalid of [
    { asset: { ...asset, object_key: "private" }, duplicate_of: null },
    { asset: { ...asset, byte_size: 3 }, duplicate_of: null },
    { asset: { ...asset, kind: "image" as const }, duplicate_of: null },
  ]) {
    vi.mocked(media.uploadPersonalMediaAsset).mockResolvedValue(invalid);
    await expect(
      uploadLibraryOriginal(
        identity,
        file,
        intent,
        new AbortController().signal,
        vi.fn(),
      ),
    ).rejects.toMatchObject({ status: 502 });
  }
  const project = {
    ...identity,
    scope: { kind: "project" as const, project_id: identity.actorId },
  };
  sdk.project.mockResolvedValue({
    asset: {
      ...asset,
      project_id: identity.orgId,
      width: undefined,
      height: undefined,
      duration_ms: undefined,
    },
    duplicate_of: null,
  });
  await expect(
    uploadLibraryOriginal(
      project,
      file,
      intent,
      new AbortController().signal,
      vi.fn(),
    ),
  ).rejects.toMatchObject({ status: 502 });
});
