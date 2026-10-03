import { beforeEach, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/request";
import { listBibleEntries } from "@/api/bible";
import { getMediaPreview, listMediaAssets } from "@/api/media";
import { queryModels, type Model } from "@/components/catalog/queries";
import { getWorkspace } from "@/components/script/source-queries";
import {
  getEpisodes,
  listStructureVersions,
  getStructure,
} from "@/components/script/review-queries";
import {
  getBibleMediaCandidates,
  getBiblePreview,
  getBibleVoiceModel,
  getBibleFormalScenes,
} from "./bible-queries";
vi.mock("@/api/bible", () => ({ listBibleEntries: vi.fn() }));
vi.mock("@/api/media", () => ({
  listMediaAssets: vi.fn(),
  getMediaPreview: vi.fn(),
}));
vi.mock("@/components/catalog/queries", () => ({ queryModels: vi.fn() }));
vi.mock("@/components/script/source-queries", async (original) => ({
  ...(await original<typeof import("@/components/script/source-queries")>()),
  getWorkspace: vi.fn(),
}));
vi.mock("@/components/script/review-queries", () => ({
  getEpisodes: vi.fn(),
  listStructureVersions: vi.fn(),
  getStructure: vi.fn(),
}));
const identity = {
    origin: window.location.origin,
    actorId: "11111111-1111-4111-8111-111111111111",
    orgId: "22222222-2222-4222-8222-222222222222",
    projectId: "33333333-3333-4333-8333-333333333333",
  },
  id = "44444444-4444-4444-8444-444444444444",
  versionId = "55555555-5555-4555-8555-555555555555",
  structureId = "66666666-6666-4666-8666-666666666666";
const asset = {
  id,
  project_id: identity.projectId,
  kind: "image",
  file_name: "合成.png",
  mime_type: "image/png",
  byte_size: 200,
  revision: 1,
};
beforeEach(() => {
  vi.resetAllMocks();
  vi.mocked(listBibleEntries).mockResolvedValue({
    entries: [],
    current_actor_id: identity.actorId,
    current_org_id: identity.orgId,
  });
  vi.mocked(listMediaAssets).mockResolvedValue({
    items: [asset],
    next_cursor: undefined,
  });
});
it("素材候选严格限制原scope和种类，并拒重复身份及未知服务端私有字段", async () => {
  vi.mocked(listMediaAssets).mockResolvedValue({
    items: [asset],
    next_cursor: null,
  } as unknown as Awaited<ReturnType<typeof listMediaAssets>>);
  expect((await getBibleMediaCandidates(identity, "image")).items).toEqual([
    asset,
  ]);
  for (const response of [
    { items: [asset, asset], next_cursor: null },
    { items: [{ ...asset, project_id: id }], next_cursor: null },
    { items: [{ ...asset, kind: "audio" }], next_cursor: null },
    { items: [asset], next_cursor: null, object_key: "unexpected" },
  ]) {
    vi.mocked(listMediaAssets).mockResolvedValue(
      response as unknown as Awaited<ReturnType<typeof listMediaAssets>>,
    );
    await expect(
      getBibleMediaCandidates(identity, "image"),
    ).rejects.toMatchObject({ status: 502 });
  }
});
it("历史媒体预览核对精确原件revision且签名只允许当前HTTP源，fresh403不调用媒体API", async () => {
  const response = {
    asset,
    url: "http://localhost:9000/unit-private-preview",
    expires_at: "2099-01-01T00:00:00Z",
  };
  vi.mocked(getMediaPreview).mockResolvedValue(response);
  expect((await getBiblePreview(identity, id, "image", 1)).asset.id).toBe(id);
  await expect(getBiblePreview(identity, id, "image", 2)).rejects.toMatchObject(
    { status: 409 },
  );
  for (const url of [
    "file:///unit-private-preview",
    "https://user:pass@example.test/private",
  ]) {
    vi.mocked(getMediaPreview).mockResolvedValue({ ...response, url });
    await expect(
      getBiblePreview(identity, id, "image", 1),
    ).rejects.toMatchObject({ status: 502 });
  }
  vi.mocked(getMediaPreview).mockClear();
  vi.mocked(listBibleEntries).mockRejectedValue(new ApiError(403, "forbidden"));
  await expect(getBiblePreview(identity, id, "image", 1)).rejects.toMatchObject(
    { status: 403 },
  );
  expect(getMediaPreview).not.toHaveBeenCalled();
});
const choice = {
  model_key: "actual.tts",
  model_version: 2,
  voice_key: "actual.voice",
  display_name: "当前实际声音",
};
const model = {
  key: choice.model_key,
  current_version: {
    version_no: 2,
    param_schema: [
      {
        field: "voice_id",
        type: "string",
        component: "voice",
        enum: [choice.voice_key],
      },
    ],
  },
} as Model;
it("声音模型完整服务端分页定位并严格版本与正式枚举，失效不能补静态模型", async () => {
  vi.mocked(queryModels)
    .mockResolvedValueOnce({ items: [], next_cursor: "second-page" })
    .mockResolvedValue({ items: [model], next_cursor: null });
  expect(await getBibleVoiceModel(identity, choice)).toEqual(model);
  expect(queryModels).toHaveBeenLastCalledWith(
    identity.projectId,
    "audio.tts",
    undefined,
    "second-page",
  );
  vi.mocked(queryModels).mockResolvedValue({
    items: [model],
    next_cursor: null,
  });
  await expect(
    getBibleVoiceModel(identity, { ...choice, model_version: 3 }),
  ).rejects.toMatchObject({ status: 409 });
  await expect(
    getBibleVoiceModel(identity, { ...choice, voice_key: "stale.static" }),
  ).rejects.toMatchObject({ status: 409 });
  vi.mocked(queryModels).mockResolvedValue({
    items: [],
    next_cursor: "repeated",
  });
  await expect(getBibleVoiceModel(identity, choice)).rejects.toMatchObject({
    status: 502,
    code: "invalid_model_cursor",
  });
});
it("造型正式场景使用已确认结构的精确历史版本，当前未确认新结构不覆盖该事实", async () => {
  vi.mocked(getWorkspace).mockResolvedValue({
    current_actor_id: identity.actorId,
    current_org_id: identity.orgId,
    state: {
      project_id: identity.projectId,
      draft_version_id: id,
      adopted_version_id: versionId,
    },
  } as Awaited<ReturnType<typeof getWorkspace>>);
  vi.mocked(getEpisodes).mockResolvedValue({
    episodes: [{ id, confirmed_structure_id: structureId }],
  } as Awaited<ReturnType<typeof getEpisodes>>);
  vi.mocked(listStructureVersions)
    .mockResolvedValueOnce({ items: [], next_version_no: 6 } as Awaited<
      ReturnType<typeof listStructureVersions>
    >)
    .mockResolvedValue({
      items: [{ id: structureId, version_no: 4 }],
    } as Awaited<ReturnType<typeof listStructureVersions>>);
  const scenes = [{ scene_key: versionId, heading: "确认旧场景", seq_no: 1 }];
  vi.mocked(getStructure).mockResolvedValue({
    structure: { document: { scenes } },
  } as Awaited<ReturnType<typeof getStructure>>);
  expect(await getBibleFormalScenes(identity, id)).toEqual(scenes);
  expect(getEpisodes).toHaveBeenCalledWith(
    identity,
    versionId,
    undefined,
    undefined,
  );
  expect(getStructure).toHaveBeenCalledExactlyOnceWith(
    identity,
    id,
    4,
    undefined,
  );
});
