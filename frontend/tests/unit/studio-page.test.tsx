import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  applyAssetUpgrade: vi.fn(),
  appendAssetVersion: vi.fn(),
  archiveAsset: vi.fn(),
  assetDisablePreflight: vi.fn(),
  assetDeletePreflight: vi.fn(),
  assetRenamePreflight: vi.fn(),
  assetStateDisablePreflight: vi.fn(),
  createAsset: vi.fn(),
  createAssetState: vi.fn(),
  deleteAsset: vi.fn(),
  getAssetReadiness: vi.fn(),
  getAssetBible: vi.fn(),
  listAssets: vi.fn(),
  listAssetShotUsages: vi.fn(),
  listAssetVersions: vi.fn(),
  listMedia: vi.fn(),
  listProjects: vi.fn(),
  me: vi.fn(),
  preflightAssetUpgrade: vi.fn(),
  restoreAsset: vi.fn(),
  renameAsset: vi.fn(),
  currentAssetVersionPreflight: vi.fn(),
  disableAsset: vi.fn(),
  disableAssetState: vi.fn(),
  setCurrentAssetVersion: vi.fn(),
  updateAsset: vi.fn(),
  updateAssetState: vi.fn(),
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
    appendAssetVersionApiV1AssetStatesStateIdVersionsPost:
      apiMocks.appendAssetVersion,
    archiveAssetApiV1AssetsAssetIdArchivePost: apiMocks.archiveAsset,
    assetDisablePreflightApiV1AssetsAssetIdDisablePreflightPost:
      apiMocks.assetDisablePreflight,
    assetDeletePreflightApiV1AssetsAssetIdDeletePreflightGet:
      apiMocks.assetDeletePreflight,
    assetRenamePreflightApiV1AssetsAssetIdRenamePreflightPost:
      apiMocks.assetRenamePreflight,
    assetStateDisablePreflightApiV1AssetStatesStateIdDisablePreflightPost:
      apiMocks.assetStateDisablePreflight,
    createAssetApiV1ProjectsProjectIdAssetsPost: apiMocks.createAsset,
    createAssetStateApiV1AssetsAssetIdStatesPost: apiMocks.createAssetState,
    deleteAssetApiV1AssetsAssetIdDelete: apiMocks.deleteAsset,
    getAssetReadinessApiV1AssetVersionsVersionIdReadinessGet:
      apiMocks.getAssetReadiness,
    getAssetBibleApiV1ProjectsProjectIdAssetBibleGet: apiMocks.getAssetBible,
    listAssetsApiV1ProjectsProjectIdAssetsGet: apiMocks.listAssets,
    listAssetVersionsApiV1AssetStatesStateIdVersionsGet: apiMocks.listAssetVersions,
    restoreAssetApiV1AssetsAssetIdRestorePost: apiMocks.restoreAsset,
    renameAssetApiV1AssetsAssetIdRenamePost: apiMocks.renameAsset,
    currentAssetVersionPreflightApiV1AssetStatesStateIdCurrentVersionPreflightPost:
      apiMocks.currentAssetVersionPreflight,
    disableAssetApiV1AssetsAssetIdDisablePost: apiMocks.disableAsset,
    disableAssetStateApiV1AssetStatesStateIdDisablePost:
      apiMocks.disableAssetState,
    setCurrentAssetVersionApiV1AssetStatesStateIdCurrentVersionPost:
      apiMocks.setCurrentAssetVersion,
    updateAssetApiV1AssetsAssetIdPatch: apiMocks.updateAsset,
    updateAssetStateApiV1AssetStatesStateIdPatch: apiMocks.updateAssetState,
  };
});

vi.mock("@/api/storyboards", async () => {
  const actual = await vi.importActual<typeof import("@/api/storyboards")>(
    "@/api/storyboards",
  );
  return {
    ...actual,
    applyAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePost:
      apiMocks.applyAssetUpgrade,
    listAssetShotUsagesApiV1AssetVersionsAssetVersionIdShotUsagesGet:
      apiMocks.listAssetShotUsages,
    preflightAssetUpgradeApiV1AssetVersionsAssetVersionIdUpgradePreflightPost:
      apiMocks.preflightAssetUpgrade,
  };
});

import { AppProviders } from "@/app/providers";
import { AssetVersionUsage } from "@/app/studio/asset-version-usage";
import { ComicProductionStudio } from "@/app/studio/comic-production-studio";
import { setAccessToken } from "@/lib/auth-session";

const workspaceId = "019fb1e0-a00a-70f6-99dc-0b4e9e085565";
const projectId = "019fb1e0-a010-70f6-99dc-0b4e9e085566";
const assetId = "019fb1e0-a020-70f6-99dc-0b4e9e085567";
const stateId = "019fb1e0-a025-70f6-99dc-0b4e9e085567";
const versionId = "019fb1e0-a030-70f6-99dc-0b4e9e085568";
const oldVersionId = "019fb1e0-a030-70f6-99dc-0b4e9e085560";
const mediaVersionId = "019fb1e0-a040-70f6-99dc-0b4e9e085569";
const episodeId = "019fb1e0-a060-70f6-99dc-0b4e9e085570";
const shotId = "019fb1e0-a070-70f6-99dc-0b4e9e085571";
const shotSpecVersionId = "019fb1e0-a080-70f6-99dc-0b4e9e085572";
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
  availability: "enabled",
  name_revision: 1,
  revision: 3,
  created_at: now,
  updated_at: now,
  warnings: [],
};

const assetState: API.AssetStateResponse = {
  id: stateId,
  workspace_id: workspaceId,
  asset_id: assetId,
  state_key: "base",
  label: "基础状态",
  description: "",
  status: "active",
  current_version_id: versionId,
  revision: 3,
  created_by: "019fb1e0-a000-7000-8000-000000000001",
  created_at: now,
  updated_at: now,
};

const version: API.AssetVersionResponse = {
  id: versionId,
  workspace_id: workspaceId,
  asset_id: assetId,
  asset_state_id: stateId,
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

const oldVersion: API.AssetVersionResponse = {
  ...version,
  id: oldVersionId,
  version_no: 2,
  content_hash: "c".repeat(64),
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
    asset_state_id: stateId,
    asset_state_revision: 3,
    media_version_ids: [mediaVersionId],
    consent_ids: [],
    evaluated_at: now,
  },
};

const changeImpact: API.AssetImpactResponse = {
  operation: "set_current",
  asset_id: assetId,
  state_id: stateId,
  old_version_id: versionId,
  new_version_id: oldVersionId,
  summary: {
    episode_count: 1,
    shot_count: 1,
    spec_version_count: 1,
    prompt_snapshot_count: 1,
    active_task_count: 1,
  },
  episodes: [
    {
      episode_id: episodeId,
      shot_count: 1,
      prompt_snapshot_count: 1,
      active_task_count: 1,
    },
  ],
  shots: [
    {
      shot_id: shotId,
      shot_title: "雨夜相逢",
      episode_id: episodeId,
      spec_version_ids: [shotSpecVersionId],
      current_spec_version_id: shotSpecVersionId,
      slot_keys: ["character-main"],
    },
  ],
  prompt_snapshots: [
    {
      generation_request_id: "019fb1e0-a090-70f6-99dc-0b4e9e085573",
      episode_id: episodeId,
      shot_id: shotId,
      shot_spec_version_id: shotSpecVersionId,
      input_hash: "d".repeat(64),
    },
  ],
  active_tasks: [
    {
      task_id: "019fb1e0-a091-70f6-99dc-0b4e9e085573",
      generation_request_id: "019fb1e0-a090-70f6-99dc-0b4e9e085573",
      status: "running",
      revision: 1,
    },
  ],
  impact_hash: "f".repeat(64),
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
    apiMocks.getAssetBible.mockResolvedValue({
      data: {
        items: [
          {
            asset,
            states: [
              {
                state: assetState,
                current_version: version,
                occurrences: [],
                readiness: {
                  status: "blocked",
                  blockers: readiness.blockers,
                  warnings: [],
                  next_actions: readiness.next_actions,
                  dependency_snapshot: {
                    asset_state_id: stateId,
                    asset_state_revision: 3,
                    current_version_id: versionId,
                    occurrence_decision_ids: [],
                    media_version_ids: [mediaVersionId],
                    consent_ids: [],
                    evaluated_at: now,
                  },
                },
              },
            ],
          },
        ],
        summary: {
          asset_count: 1,
          state_count: 1,
          ready: 0,
          draft: 0,
          blocked: 1,
          unavailable: 0,
        },
      },
    });
    apiMocks.listAssetVersions.mockResolvedValue({
      data: { items: [version, oldVersion], total: 2, limit: 100, offset: 0 },
    });
    apiMocks.listAssetShotUsages.mockResolvedValue({
      data: {
        items: [
          {
            shot_id: shotId,
            shot_title: "雨夜相逢",
            episode_id: episodeId,
            spec_version_id: shotSpecVersionId,
            spec_version_no: 2,
            slot_keys: ["character-main"],
            is_current: true,
          },
          {
            shot_id: shotId,
            shot_title: "雨夜相逢",
            episode_id: episodeId,
            spec_version_id: "019fb1e0-a080-70f6-99dc-0b4e9e085573",
            spec_version_no: 1,
            slot_keys: ["character-main"],
            is_current: false,
          },
        ],
        total: 2,
        limit: 20,
        offset: 0,
      },
    });
    apiMocks.preflightAssetUpgrade.mockResolvedValue({
      data: {
        old_asset_version_id: oldVersionId,
        new_asset_version_id: versionId,
        targets: [
          {
            shot_id: shotId,
            expected_spec_version_id: shotSpecVersionId,
            expected_shot_revision: 3,
            slot_keys: ["character-main"],
            new_input_hash: "d".repeat(64),
          },
        ],
        preflight_hash: "e".repeat(64),
      },
    });
    apiMocks.applyAssetUpgrade.mockResolvedValue({
      data: {
        shots: [
          {
            id: shotId,
            workspace_id: workspaceId,
            episode_id: episodeId,
            position: 1,
            title: "雨夜相逢",
            source_script_version_id: null,
            source_scene_id: null,
            source_candidate_id: null,
            creation_key: "asset-upgrade-shot-1",
            status: "active",
            current_spec_version_id: "019fb1e0-a080-70f6-99dc-0b4e9e085574",
            revision: 4,
            created_at: now,
            updated_at: now,
          },
        ],
        spec_versions: [],
      },
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
      data: { ...asset, id: "019fb1e0-a050-7000-8000-000000000001", name: "陆沉舟", revision: 1 },
    });
    apiMocks.createAssetState.mockResolvedValue({
      data: {
        asset: { ...asset, revision: 4 },
        state: {
          ...assetState,
          id: "019fb1e0-a025-70f6-99dc-0b4e9e085590",
          state_key: "injured",
          label: "受伤状态",
          description: "第 3 集雨夜追逐后，左臂带伤",
          current_version_id: null,
          revision: 1,
        },
      },
    });
    apiMocks.appendAssetVersion.mockResolvedValue({
      data: {
        state: { ...assetState, revision: 4 },
        version: { ...version, version_no: 4 },
        readiness: { ...readiness, status: "ready", blockers: [], next_actions: [] },
      },
    });
    apiMocks.archiveAsset.mockResolvedValue({
      data: { ...asset, status: "archived", revision: 4 },
    });
    apiMocks.restoreAsset.mockResolvedValue({ data: asset });
    apiMocks.updateAsset.mockResolvedValue({
      data: {
        ...asset,
        aliases: ["清禾", "顾小姐"],
        tags: ["主角", "雨巷"],
        revision: 4,
      },
    });
    apiMocks.assetRenamePreflight.mockResolvedValue({
      data: { ...changeImpact, operation: "rename", new_version_id: null },
    });
    apiMocks.renameAsset.mockResolvedValue({
      data: {
        asset: {
          ...asset,
          name: "顾清禾（雨巷）",
          aliases: ["清禾", "顾清禾"],
          name_revision: 2,
          revision: 4,
        },
        impact: { ...changeImpact, operation: "rename", new_version_id: null },
      },
    });
    apiMocks.currentAssetVersionPreflight.mockResolvedValue({ data: changeImpact });
    apiMocks.assetStateDisablePreflight.mockResolvedValue({
      data: { ...changeImpact, operation: "disable_state", new_version_id: null },
    });
    apiMocks.assetDisablePreflight.mockResolvedValue({
      data: { ...changeImpact, operation: "disable_asset", new_version_id: null },
    });
    apiMocks.disableAsset.mockResolvedValue({
      data: {
        asset: { ...asset, availability: "disabled", revision: 4 },
        impact: { ...changeImpact, operation: "disable_asset", new_version_id: null },
      },
    });
    apiMocks.disableAssetState.mockResolvedValue({
      data: {
        state: { ...assetState, status: "disabled", revision: 4 },
        impact: { ...changeImpact, operation: "disable_state", new_version_id: null },
      },
    });
    apiMocks.updateAssetState.mockResolvedValue({
      data: { ...assetState, label: "雨夜状态", description: "衣物湿透", revision: 4 },
    });
    apiMocks.setCurrentAssetVersion.mockResolvedValue({
      data: {
        state: { ...assetState, current_version_id: oldVersionId, revision: 4 },
        impact: changeImpact,
      },
    });
    apiMocks.assetDeletePreflight.mockResolvedValue({
      data: {
        allowed: false,
        blockers: [
          {
            code: "asset_has_versions",
            summary: "Asset has 2 immutable version(s)",
            version_count: 2,
            decision_count: 0,
            related_version_count: 0,
          },
        ],
      },
    });
    apiMocks.deleteAsset.mockResolvedValue({ data: { deleted: true } });
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
    expect(
      screen.getByRole("link", { name: "前往授权治理" }),
    ).toHaveAttribute(
      "href",
      `/governance?subjectType=ASSET_VERSION&subjectId=${versionId}&proofMediaVersionId=${mediaVersionId}`,
    );

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
      { state_id: stateId },
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
        expected_revision: 3,
        expected_current_version_id: versionId,
        set_as_current: true,
      }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent("版本 v4 已保存");
  });

  it("为资产身份建立语义化剧情状态", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <ComicProductionStudio />
      </AppProviders>,
    );

    expect(await screen.findByRole("button", { name: /基础状态/ })).toBePressed();
    await user.click(screen.getByRole("button", { name: "新建状态" }));
    await user.type(screen.getByLabelText("状态键"), "injured");
    await user.type(screen.getByLabelText("显示名称"), "受伤状态");
    await user.type(
      screen.getByLabelText("状态说明"),
      "第 3 集雨夜追逐后，左臂带伤",
    );
    await user.click(screen.getByRole("button", { name: "创建状态" }));

    await waitFor(() =>
      expect(apiMocks.createAssetState).toHaveBeenCalledWith(
        { asset_id: assetId },
        {
          state_key: "injured",
          label: "受伤状态",
          description: "第 3 集雨夜追逐后，左臂带伤",
          expected_asset_revision: 3,
          idempotency_key: expect.any(String),
        },
      ),
    );
    expect(await screen.findByRole("status")).toHaveTextContent(
      "剧情状态已创建：受伤状态",
    );
  });

  it("分离元数据编辑与重命名，并在影响确认后切换当前版本", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <ComicProductionStudio />
      </AppProviders>,
    );

    await user.click(await screen.findByRole("button", { name: "编辑资产身份" }));
    expect(screen.queryByLabelText("资产名称")).not.toBeInTheDocument();
    const aliases = screen.getByLabelText("别名（逗号分隔）");
    await user.clear(aliases);
    await user.type(aliases, "清禾, 顾小姐");
    const tags = screen.getByLabelText("标签（逗号分隔）");
    await user.clear(tags);
    await user.type(tags, "主角, 雨巷");
    await user.click(screen.getByRole("button", { name: "保存身份信息" }));

    await waitFor(() => expect(apiMocks.updateAsset).toHaveBeenCalledWith(
      { asset_id: assetId },
      {
        expected_revision: 3,
        aliases: ["清禾", "顾小姐"],
        tags: ["主角", "雨巷"],
      },
    ));

    await user.click(screen.getByRole("button", { name: "重命名资产" }));
    await user.clear(screen.getByLabelText("新资产名称"));
    await user.type(screen.getByLabelText("新资产名称"), "顾清禾（雨巷）");
    await user.click(screen.getByRole("button", { name: "检查影响" }));
    await waitFor(() => expect(apiMocks.assetRenamePreflight).toHaveBeenCalledWith(
      { asset_id: assetId },
      { new_name: "顾清禾（雨巷）", expected_revision: 3 },
    ));
    expect(await screen.findByText("影响 1 个分镜")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "确认重命名" }));
    await waitFor(() => expect(apiMocks.renameAsset).toHaveBeenCalledWith(
      { asset_id: assetId },
      expect.objectContaining({
        new_name: "顾清禾（雨巷）",
        expected_revision: 3,
        impact_hash: "f".repeat(64),
        idempotency_key: expect.any(String),
      }),
    ));

    await user.click(screen.getByRole("button", { name: "设为当前资产版本 v2" }));
    await waitFor(() =>
      expect(apiMocks.currentAssetVersionPreflight).toHaveBeenCalledWith(
        { state_id: stateId },
        {
          version_id: oldVersionId,
          expected_current_version_id: versionId,
          expected_revision: 3,
        },
      ),
    );
    expect(await screen.findByRole("dialog", { name: "确认切换当前版本" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "确认切换" }));
    await waitFor(() => expect(apiMocks.setCurrentAssetVersion).toHaveBeenCalledWith(
      { state_id: stateId },
      expect.objectContaining({
        version_id: oldVersionId,
        expected_current_version_id: versionId,
        expected_revision: 3,
        impact_hash: "f".repeat(64),
        idempotency_key: expect.any(String),
      }),
    ));
  });

  it("编辑剧情状态并在影响确认后停用", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <ComicProductionStudio />
      </AppProviders>,
    );

    await user.click(await screen.findByRole("button", { name: "编辑状态" }));
    const label = screen.getByLabelText("状态名称");
    await user.clear(label);
    await user.type(label, "雨夜状态");
    await user.type(screen.getByLabelText("状态说明"), "衣物湿透");
    await user.click(screen.getByRole("button", { name: "保存状态" }));
    await waitFor(() => expect(apiMocks.updateAssetState).toHaveBeenCalledWith(
      { state_id: stateId },
      expect.objectContaining({
        expected_revision: 3,
        idempotency_key: expect.any(String),
        label: "雨夜状态",
        description: "衣物湿透",
      }),
    ));

    await user.click(screen.getByRole("button", { name: "停用状态" }));
    await waitFor(() => expect(apiMocks.assetStateDisablePreflight).toHaveBeenCalledWith(
      { state_id: stateId },
      { expected_revision: 3 },
    ));
    expect(
      await screen.findByRole("dialog", { name: "确认停用剧情状态" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "确认停用" }));
    await waitFor(() => expect(apiMocks.disableAssetState).toHaveBeenCalledWith(
      { state_id: stateId },
      expect.objectContaining({
        expected_revision: 3,
        impact_hash: "f".repeat(64),
        idempotency_key: expect.any(String),
      }),
    ));
  });

  it("预览影响后停用资产且不伪装成归档", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <ComicProductionStudio />
      </AppProviders>,
    );

    await user.click(await screen.findByRole("button", { name: "停用资产" }));
    await waitFor(() => expect(apiMocks.assetDisablePreflight).toHaveBeenCalledWith(
      { asset_id: assetId },
      { expected_revision: 3 },
    ));
    expect(
      await screen.findByRole("dialog", { name: "确认停用资产" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "确认停用" }));
    await waitFor(() => expect(apiMocks.disableAsset).toHaveBeenCalledWith(
      { asset_id: assetId },
      expect.objectContaining({
        expected_revision: 3,
        impact_hash: "f".repeat(64),
        idempotency_key: expect.any(String),
      }),
    ));
    expect(apiMocks.archiveAsset).not.toHaveBeenCalled();
  });

  it("仅在删除预检允许时删除空资产身份", async () => {
    const user = userEvent.setup();
    const emptyAsset: API.AssetResponse = {
      ...asset,
      id: "019fb1e0-a020-70f6-99dc-0b4e9e085599",
      name: "临时空角色",
      aliases: [],
      tags: [],
      revision: 1,
    };
    apiMocks.listAssets.mockResolvedValue({
      data: { items: [emptyAsset], total: 1, limit: 100, offset: 0 },
    });
    apiMocks.listAssetVersions.mockResolvedValue({
      data: { items: [], total: 0, limit: 100, offset: 0 },
    });
    apiMocks.getAssetBible.mockResolvedValue({
      data: {
        items: [
          {
            asset: emptyAsset,
            states: [
              {
                state: {
                  ...assetState,
                  id: `${stateId.slice(0, -1)}9`,
                  asset_id: emptyAsset.id,
                  current_version_id: null,
                  revision: 1,
                },
                current_version: null,
                occurrences: [],
                readiness: {
                  status: "draft",
                  blockers: [],
                  warnings: [],
                  next_actions: [],
                  dependency_snapshot: {
                    asset_state_id: `${stateId.slice(0, -1)}9`,
                    asset_state_revision: 1,
                    current_version_id: null,
                    occurrence_decision_ids: [],
                    media_version_ids: [],
                    consent_ids: [],
                    evaluated_at: now,
                  },
                },
              },
            ],
          },
        ],
        summary: {
          asset_count: 1,
          state_count: 1,
          ready: 0,
          draft: 1,
          blocked: 0,
          unavailable: 0,
        },
      },
    });
    apiMocks.assetDeletePreflight.mockResolvedValue({
      data: { allowed: true, blockers: [] },
    });

    render(
      <AppProviders>
        <ComicProductionStudio />
      </AppProviders>,
    );

    await user.click(await screen.findByRole("button", { name: "删除资产身份" }));
    expect(
      await screen.findByRole("dialog", { name: "删除资产身份" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "确认删除空资产" }));

    await waitFor(() => expect(apiMocks.assetDeletePreflight).toHaveBeenCalledWith({
      asset_id: emptyAsset.id,
    }));
    expect(apiMocks.deleteAsset).toHaveBeenCalledWith({
      asset_id: emptyAsset.id,
      expected_revision: 1,
    });
  });

  it("使用结构化版本数量展示本地化删除阻塞", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <ComicProductionStudio />
      </AppProviders>,
    );

    await user.click(await screen.findByRole("button", { name: "删除资产身份" }));
    const dialog = await screen.findByRole("dialog", { name: "删除资产身份" });
    expect(dialog).toHaveTextContent("资产包含 2 个不可变版本，不能删除。");
    expect(
      screen.queryByRole("button", { name: "确认删除空资产" }),
    ).not.toBeInTheDocument();
  });

  it("展示剧本候选决议关联并引导归档资产", async () => {
    const user = userEvent.setup();
    apiMocks.assetDeletePreflight.mockResolvedValue({
      data: {
        allowed: false,
        blockers: [
          {
            code: "asset_has_candidate_decisions",
            summary: "Asset is linked from 1 script candidate decision(s)",
            version_count: 0,
            decision_count: 1,
            related_version_count: 0,
          },
        ],
      },
    });
    render(
      <AppProviders>
        <ComicProductionStudio />
      </AppProviders>,
    );

    await user.click(await screen.findByRole("button", { name: "删除资产身份" }));
    const dialog = await screen.findByRole("dialog", { name: "删除资产身份" });
    expect(dialog).toHaveTextContent(
      "资产已被 1 条剧本候选决议关联，只能归档。",
    );
    expect(
      screen.queryByRole("button", { name: "确认删除空资产" }),
    ).not.toBeInTheDocument();
  });

  it("展示道具或服装历史版本引用并引导归档角色", async () => {
    const user = userEvent.setup();
    apiMocks.assetDeletePreflight.mockResolvedValue({
      data: {
        allowed: false,
        blockers: [
          {
            code: "asset_has_related_versions",
            summary: "Asset is referenced by 1 related asset version(s)",
            version_count: 0,
            decision_count: 0,
            related_version_count: 1,
          },
        ],
      },
    });
    render(
      <AppProviders>
        <ComicProductionStudio />
      </AppProviders>,
    );

    await user.click(await screen.findByRole("button", { name: "删除资产身份" }));
    const dialog = await screen.findByRole("dialog", { name: "删除资产身份" });
    expect(dialog).toHaveTextContent(
      "资产已被 1 个道具或服装版本引用，只能归档。",
    );
    expect(
      screen.queryByRole("button", { name: "确认删除空资产" }),
    ).not.toBeInTheDocument();
  });

  it("检查历史资产版本的分镜引用并在预检后批量升级", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <ComicProductionStudio />
      </AppProviders>,
    );

    expect(
      await screen.findByRole("heading", { name: "分镜引用与版本升级" }),
    ).toBeInTheDocument();
    await waitFor(() =>
      expect(apiMocks.listAssetShotUsages).toHaveBeenCalledWith({
        asset_version_id: oldVersionId,
        limit: 20,
        offset: 0,
      }),
    );
    expect(
      await screen.findByText("历史规格引用，不参与批量升级"),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("checkbox", { name: "选择镜头 雨夜相逢" }));
    await user.click(screen.getByRole("button", { name: "生成升级预检" }));

    await waitFor(() =>
      expect(apiMocks.preflightAssetUpgrade).toHaveBeenCalledWith(
        { asset_version_id: oldVersionId },
        { new_asset_version_id: versionId, shot_ids: [shotId] },
      ),
    );
    expect(
      await screen.findByRole("heading", { name: "确认资产版本升级" }),
    ).toBeInTheDocument();
    expect(screen.getByText("旧规格和历史引用会继续保留")).toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: "应用升级并创建新规格版本" }),
    );
    await waitFor(() =>
      expect(apiMocks.applyAssetUpgrade).toHaveBeenCalledWith(
        { asset_version_id: oldVersionId },
        {
          new_asset_version_id: versionId,
          targets: [
            {
              shot_id: shotId,
              expected_spec_version_id: shotSpecVersionId,
              expected_shot_revision: 3,
              slot_keys: ["character-main"],
              new_input_hash: "d".repeat(64),
            },
          ],
          preflight_hash: "e".repeat(64),
        },
      ),
    );
    expect(await screen.findByRole("status")).toHaveTextContent(
      "已为 1 个镜头创建新的规格版本",
    );
  });

  it("来源版本随资产当前版本变化时不沿用旧选择", async () => {
    const user = userEvent.setup();
    const onCompleted = vi.fn();
    const onError = vi.fn();
    const view = render(
      <AppProviders>
        <AssetVersionUsage
          asset={asset}
          currentVersionId={versionId}
          onCompleted={onCompleted}
          onError={onError}
          versions={[version, oldVersion]}
        />
      </AppProviders>,
    );

    await user.click(
      await screen.findByRole("checkbox", { name: "选择镜头 雨夜相逢" }),
    );
    const preflightButton = screen.getByRole("button", {
      name: "生成升级预检",
    });
    expect(preflightButton.parentElement).toHaveTextContent(
      "已选择 1 个当前镜头",
    );

    view.rerender(
      <AppProviders>
        <AssetVersionUsage
          asset={{ ...asset, revision: 4 }}
          currentVersionId={oldVersionId}
          onCompleted={onCompleted}
          onError={onError}
          versions={[version, oldVersion]}
        />
      </AppProviders>,
    );

    expect(preflightButton.parentElement).toHaveTextContent(
      "已选择 0 个当前镜头",
    );
    expect(preflightButton).toBeDisabled();
  });
});
