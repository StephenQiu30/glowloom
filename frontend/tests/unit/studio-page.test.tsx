import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  appendAssetVersion: vi.fn(),
  archiveAsset: vi.fn(),
  createAsset: vi.fn(),
  getAssetReadiness: vi.fn(),
  listAssets: vi.fn(),
  listAssetVersions: vi.fn(),
  listMedia: vi.fn(),
  listProjects: vi.fn(),
  me: vi.fn(),
  restoreAsset: vi.fn(),
}));

vi.mock("@/api/identity", async () => {
  const actual = await vi.importActual<typeof import("@/api/identity")>(
    "@/api/identity",
  );
  return { ...actual, meApiV1MeGet: apiMocks.me };
});

vi.mock("@/api/projects", async () => {
  const actual = await vi.importActual<typeof import("@/api/projects")>(
    "@/api/projects",
  );
  return { ...actual, listProjectsApiV1ProjectsGet: apiMocks.listProjects };
});

vi.mock("@/api/media", async () => {
  const actual = await vi.importActual<typeof import("@/api/media")>("@/api/media");
  return { ...actual, listMediaApiV1MediaGet: apiMocks.listMedia };
});

vi.mock("@/api/assets", async () => {
  const actual = await vi.importActual<typeof import("@/api/assets")>("@/api/assets");
  return {
    ...actual,
    appendAssetVersionApiV1AssetsAssetIdVersionsPost:
      apiMocks.appendAssetVersion,
    archiveAssetApiV1AssetsAssetIdArchivePost: apiMocks.archiveAsset,
    createAssetApiV1ProjectsProjectIdAssetsPost: apiMocks.createAsset,
    getAssetReadinessApiV1AssetVersionsVersionIdReadinessGet:
      apiMocks.getAssetReadiness,
    listAssetsApiV1ProjectsProjectIdAssetsGet: apiMocks.listAssets,
    listAssetVersionsApiV1AssetsAssetIdVersionsGet: apiMocks.listAssetVersions,
    restoreAssetApiV1AssetsAssetIdRestorePost: apiMocks.restoreAsset,
  };
});

import { AppProviders } from "@/app/providers";
import { ComicProductionStudio } from "@/app/studio/comic-production-studio";
import { setAccessToken } from "@/lib/auth-session";

const workspaceId = "019fb1e0-a00a-70f6-99dc-0b4e9e085565";
const projectId = "019fb1e0-a010-70f6-99dc-0b4e9e085566";
const assetId = "019fb1e0-a020-70f6-99dc-0b4e9e085567";
const versionId = "019fb1e0-a030-70f6-99dc-0b4e9e085568";
const mediaVersionId = "019fb1e0-a040-70f6-99dc-0b4e9e085569";
const now = "2026-07-30T08:00:00Z";

const asset: API.AssetResponse = {
  id: assetId,
  workspace_id: workspaceId,
  project_id: projectId,
  kind: "character",
  name: "顾清禾",
  aliases: ["清禾"],
  tags: ["主角"],
  status: "active",
  current_version_id: versionId,
  revision: 3,
  created_at: now,
  updated_at: now,
  warnings: [],
};

const version: API.AssetVersionResponse = {
  id: versionId,
  workspace_id: workspaceId,
  asset_id: assetId,
  version_no: 3,
  schema_version: 1,
  spec: {
    kind: "character",
    identity: "女主角",
    appearance: "乌发高髻，青灰色长衫",
    age_impression: "二十五岁",
    temperament: ["清冷", "克制"],
  },
  prompt_description: "保持右眼下方泪痣与青灰色长衫。",
  source_type: "manual",
  source_id: null,
  content_hash: "a".repeat(64),
  media_references: [
    { media_version_id: mediaVersionId, purpose: "portrait", position: 1 },
  ],
  created_by: "019fb1e0-a000-7000-8000-000000000001",
  created_at: now,
};

const readiness: API.AssetReadinessResponse = {
  status: "blocked",
  blockers: [
    {
      code: "consent_missing",
      field_path: null,
      dependency_type: "CONSENT",
      dependency_id: null,
      summary: "资产版本缺少有效授权",
      next_action: "review_asset_consent",
    },
  ],
  warnings: [],
  next_actions: ["review_asset_consent"],
  dependency_snapshot: {
    asset_version_id: versionId,
    media_version_ids: [mediaVersionId],
    consent_ids: [],
    evaluated_at: now,
  },
};

describe("AI 漫剧资产工作台", () => {
  beforeEach(() => {
    sessionStorage.clear();
    setAccessToken("test-access-token");
    vi.clearAllMocks();
    apiMocks.me.mockResolvedValue({
      data: {
        user: {
          id: "019fb1e0-a000-7000-8000-000000000001",
          email: "creator@example.com",
          display_name: "创作者",
          avatar_url: null,
        },
        workspace: {
          id: workspaceId,
          name: "个人创作空间",
          status: "active",
          role: "owner",
          revision: 1,
        },
      },
    });
    apiMocks.listProjects.mockResolvedValue({
      data: {
        items: [
          {
            id: projectId,
            workspace_id: workspaceId,
            name: "镜中长安",
            description: "水墨幻想漫剧",
            aspect_ratio: "9:16",
            language: "zh-CN",
            visual_style: "水墨幻想",
            target_duration_ms: 90_000,
            budget_limit: "1000.00",
            currency: "CNY",
            status: "active",
            revision: 2,
          },
        ],
        total: 1,
        limit: 50,
        offset: 0,
      },
    });
    apiMocks.listAssets.mockResolvedValue({
      data: { items: [asset], total: 1, limit: 100, offset: 0 },
    });
    apiMocks.listAssetVersions.mockResolvedValue({
      data: { items: [version], total: 1, limit: 100, offset: 0 },
    });
    apiMocks.getAssetReadiness.mockResolvedValue({ data: readiness });
    apiMocks.listMedia.mockResolvedValue({
      data: {
        items: [
          {
            id: mediaVersionId,
            workspace_id: workspaceId,
            media_object_id: "019fb1e0-a041-7000-8000-000000000001",
            version_no: 1,
            filename: "gu-qinghe-portrait.png",
            sha256: "b".repeat(64),
            size_bytes: 2048,
            mime_type: "image/png",
            probe_status: "ready",
            probe_attempt: 1,
            probe_error_code: null,
            probe_error_summary: null,
            probe_next_action: null,
            width: 1080,
            height: 1920,
            duration_ms: null,
            codec: null,
            container: "png",
            created_at: now,
          },
        ],
        total: 1,
        limit: 100,
        offset: 0,
      },
    });
    apiMocks.createAsset.mockResolvedValue({
      data: { ...asset, id: "019fb1e0-a050-7000-8000-000000000001", name: "陆沉舟", current_version_id: null, revision: 1 },
    });
    apiMocks.appendAssetVersion.mockResolvedValue({
      data: {
        asset: { ...asset, revision: 4 },
        version: { ...version, version_no: 4 },
        readiness: { ...readiness, status: "ready", blockers: [], next_actions: [] },
      },
    });
    apiMocks.archiveAsset.mockResolvedValue({
      data: { ...asset, status: "archived", revision: 4 },
    });
    apiMocks.restoreAsset.mockResolvedValue({ data: asset });
  });

  it("读取真实资产事实并创建新的资产身份", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <ComicProductionStudio />
      </AppProviders>,
    );

    expect(await screen.findByRole("heading", { name: "资产库" })).toBeInTheDocument();
    expect(await screen.findByRole("button", { name: "选择资产 顾清禾" })).toBeInTheDocument();
    expect(
      await screen.findByText("缺少覆盖当前用途的有效授权"),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "新建资产" }));
    await user.clear(screen.getByLabelText("资产名称"));
    await user.type(screen.getByLabelText("资产名称"), "陆沉舟");
    await user.type(screen.getByLabelText("别名（逗号分隔）"), "沉舟, 陆公子");
    await user.type(screen.getByLabelText("标签（逗号分隔）"), "主角, 剑客");
    await user.click(screen.getByRole("button", { name: "创建资产" }));

    await waitFor(() => expect(apiMocks.createAsset).toHaveBeenCalledTimes(1));
    expect(apiMocks.createAsset).toHaveBeenCalledWith(
      { project_id: projectId },
      {
        kind: "character",
        name: "陆沉舟",
        aliases: ["沉舟", "陆公子"],
        tags: ["主角", "剑客"],
      },
    );
    expect(await screen.findByRole("status")).toHaveTextContent("资产身份已创建");
  });

  it("为选中资产追加可追溯版本并绑定固定媒体版本", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <ComicProductionStudio />
      </AppProviders>,
    );

    await screen.findByRole("button", { name: "选择资产 顾清禾" });
    await user.click(screen.getByRole("button", { name: "添加新版本" }));
    await user.type(screen.getByLabelText("身份定位"), "女主角");
    await user.type(screen.getByLabelText("外观描述"), "乌发高髻，青灰色长衫");
    await user.type(screen.getByLabelText("年龄观感"), "二十五岁");
    await user.type(screen.getByLabelText("性格特征（逗号分隔）"), "清冷, 克制");
    await user.selectOptions(screen.getByLabelText("参考媒体"), mediaVersionId);
    await user.type(screen.getByLabelText("提示词描述"), "保持右眼泪痣。");
    await user.click(screen.getByRole("button", { name: "保存版本" }));

    await waitFor(() =>
      expect(apiMocks.appendAssetVersion).toHaveBeenCalledTimes(1),
    );
    expect(apiMocks.appendAssetVersion).toHaveBeenCalledWith(
      { asset_id: assetId },
      expect.objectContaining({
        spec: {
          kind: "character",
          identity: "女主角",
          appearance: "乌发高髻，青灰色长衫",
          age_impression: "二十五岁",
          temperament: ["清冷", "克制"],
        },
        prompt_description: "保持右眼泪痣。",
        media_references: [
          { media_version_id: mediaVersionId, purpose: "portrait", position: 1 },
        ],
        source_type: "manual",
        source_id: null,
        expected_current_version_id: versionId,
        set_as_current: true,
      }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent("版本 v4 已保存");
  });
});
