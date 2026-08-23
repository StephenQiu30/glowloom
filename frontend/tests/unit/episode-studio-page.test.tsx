import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  archiveMedia: vi.fn(),
  archiveSource: vi.fn(),
  completeUpload: vi.fn(),
  confirmStructure: vi.fn(),
  createShotFromCandidate: vi.fn(),
  decideCandidate: vi.fn(),
  deleteDraftVersion: vi.fn(),
  diffVersions: vi.fn(),
  getConfirmedStructure: vi.fn(),
  getCoverage: vi.fn(),
  getNarrativeStructure: vi.fn(),
  getBatch: vi.fn(),
  getAssetBible: vi.fn(),
  getEpisode: vi.fn(),
  getProject: vi.fn(),
  getSnapshot: vi.fn(),
  getVersion: vi.fn(),
  importScript: vi.fn(),
  initializeUpload: vi.fn(),
  initializeVersionUpload: vi.fn(),
  listAssets: vi.fn(),
  listCandidates: vi.fn(),
  listEpisodes: vi.fn(),
  listMedia: vi.fn(),
  listSources: vi.fn(),
  listArchivedShots: vi.fn(),
  listExports: vi.fn(),
  listShotReadiness: vi.fn(),
  listShots: vi.fn(),
  listShotSpecVersions: vi.fn(),
  listTasks: vi.fn(),
  listVersions: vi.fn(),
  me: vi.fn(),
  publishVersion: vi.fn(),
  reviseNarrativeStructure: vi.fn(),
  retryProbe: vi.fn(),
  restoreMedia: vi.fn(),
  restoreSource: vi.fn(),
  setCurrentMediaVersion: vi.fn(),
  setCurrentVersion: vi.fn(),
  startExtraction: vi.fn(),
  updateShot: vi.fn(),
}));

vi.mock("@/api/identity", async () => ({
  ...(await vi.importActual<typeof import("@/api/identity")>("@/api/identity")),
  meApiV1MeGet: apiMocks.me,
}));

vi.mock("@/api/projects", async () => ({
  ...(await vi.importActual<typeof import("@/api/projects")>("@/api/projects")),
  episodeProductionSnapshotApiV1EpisodesEpisodeIdProductionSnapshotGet:
    apiMocks.getSnapshot,
  getEpisodeApiV1EpisodesEpisodeIdGet: apiMocks.getEpisode,
  getProjectApiV1ProjectsProjectIdGet: apiMocks.getProject,
  listEpisodesApiV1ProjectsProjectIdEpisodesGet: apiMocks.listEpisodes,
}));

vi.mock("@/api/scripts", async () => ({
  ...(await vi.importActual<typeof import("@/api/scripts")>("@/api/scripts")),
  archiveSourceApiV1ScriptSourcesSourceIdArchivePost: apiMocks.archiveSource,
  confirmStructureApiV1ExtractionBatchesBatchIdConfirmStructurePost:
    apiMocks.confirmStructure,
  decideExtractionCandidateApiV1ExtractionCandidatesCandidateIdDecisionsPost:
    apiMocks.decideCandidate,
  deleteDraftVersionApiV1ScriptVersionsVersionIdDelete:
    apiMocks.deleteDraftVersion,
  diffVersionsApiV1ScriptVersionsVersionIdDiffGet: apiMocks.diffVersions,
  getExtractionBatchApiV1ExtractionBatchesBatchIdGet: apiMocks.getBatch,
  getConfirmedStructureApiV1ScriptVersionsVersionIdStructureGet:
    apiMocks.getConfirmedStructure,
  getNarrativeStructureApiV1ScriptVersionsVersionIdNarrativeStructureGet:
    apiMocks.getNarrativeStructure,
  getVersionApiV1ScriptVersionsVersionIdGet: apiMocks.getVersion,
  importTextSourceApiV1EpisodesEpisodeIdScriptSourcesPost: apiMocks.importScript,
  listExtractionCandidatesApiV1ExtractionBatchesBatchIdCandidatesGet:
    apiMocks.listCandidates,
  listSourcesApiV1EpisodesEpisodeIdScriptSourcesGet: apiMocks.listSources,
  listVersionsApiV1ScriptSourcesSourceIdVersionsGet: apiMocks.listVersions,
  publishVersionApiV1ScriptSourcesSourceIdVersionsPost: apiMocks.publishVersion,
  reviseNarrativeStructureApiV1NarrativeStructuresStructureIdRevisionsPost:
    apiMocks.reviseNarrativeStructure,
  restoreSourceApiV1ScriptSourcesSourceIdRestorePost: apiMocks.restoreSource,
  setCurrentVersionApiV1EpisodesEpisodeIdCurrentScriptVersionPost:
    apiMocks.setCurrentVersion,
  startExtractionApiV1ScriptVersionsVersionIdExtractionsPost:
    apiMocks.startExtraction,
}));

vi.mock("@/api/storyboards", async () => ({
  ...(await vi.importActual<typeof import("@/api/storyboards")>(
    "@/api/storyboards",
  )),
  createFromConfirmedCandidateApiV1ExtractionCandidatesCandidateIdShotPost:
    apiMocks.createShotFromCandidate,
  getCoverageApiV1EpisodesEpisodeIdCoverageGet: apiMocks.getCoverage,
  getEpisodeReadinessApiV1EpisodesEpisodeIdShotReadinessGet:
    apiMocks.listShotReadiness,
  listArchivedShotsApiV1EpisodesEpisodeIdArchivedShotsGet:
    apiMocks.listArchivedShots,
  listExportsApiV1EpisodesEpisodeIdStoryboardExportsGet: apiMocks.listExports,
  listShotsApiV1EpisodesEpisodeIdShotsGet: apiMocks.listShots,
  listSpecVersionsApiV1ShotsShotIdSpecVersionsGet:
    apiMocks.listShotSpecVersions,
  updateShotApiV1ShotsShotIdPatch: apiMocks.updateShot,
}));

vi.mock("@/api/tasks", async () => ({
  ...(await vi.importActual<typeof import("@/api/tasks")>("@/api/tasks")),
  listTasksApiV1TasksGet: apiMocks.listTasks,
}));

vi.mock("@/api/media", async () => ({
  ...(await vi.importActual<typeof import("@/api/media")>("@/api/media")),
  archiveMediaApiV1MediaObjectsMediaObjectIdArchivePost: apiMocks.archiveMedia,
  completeUploadApiV1MediaUploadsUploadSessionIdCompletePost:
    apiMocks.completeUpload,
  initializeUploadApiV1MediaUploadsPost: apiMocks.initializeUpload,
  initializeVersionUploadApiV1MediaObjectsMediaObjectIdVersionsPost:
    apiMocks.initializeVersionUpload,
  listMediaApiV1MediaGet: apiMocks.listMedia,
  retryProbeApiV1MediaVersionIdProbeRetryPost: apiMocks.retryProbe,
  restoreMediaApiV1MediaObjectsMediaObjectIdRestorePost: apiMocks.restoreMedia,
  setCurrentMediaVersionApiV1MediaObjectsMediaObjectIdCurrentVersionPost:
    apiMocks.setCurrentMediaVersion,
}));

vi.mock("@/api/assets", async () => ({
  ...(await vi.importActual<typeof import("@/api/assets")>("@/api/assets")),
  getAssetBibleApiV1ProjectsProjectIdAssetBibleGet: apiMocks.getAssetBible,
  listAssetsApiV1ProjectsProjectIdAssetsGet: apiMocks.listAssets,
}));

import { AppProviders } from "@/app/providers";
import { EpisodeProductionStudio } from "@/app/studio/[episodeId]/episode-production-studio";
import { setAccessToken } from "@/lib/auth-session";

const workspaceId = "019fb2c0-a000-7000-8000-000000000001";
const projectId = "019fb2c0-a000-7000-8000-000000000002";
const episodeId = "019fb2c0-a000-7000-8000-000000000003";
const sourceId = "019fb2c0-a000-7000-8000-000000000004";
const versionId = "019fb2c0-a000-7000-8000-000000000005";
const batchId = "019fb2c0-a000-7000-8000-000000000007";
const taskId = "019fb2c0-a000-7000-8000-000000000008";
const firstCandidateId = "019fb2c0-a000-7000-8000-000000000009";
const secondCandidateId = "019fb2c0-a000-7000-8000-000000000010";
const shotCandidateId = "019fb2c0-a000-7000-8000-000000000011";
const sceneId = "019fb2c0-a000-7000-8000-000000000012";
const shotId = "019fb2c0-a000-7000-8000-000000000013";
const mediaObjectId = "019fb2c0-a000-7000-8000-000000000014";
const firstMediaVersionId = "019fb2c0-a000-7000-8000-000000000015";
const secondMediaVersionId = "019fb2c0-a000-7000-8000-000000000016";
const now = "2026-07-30T09:00:00Z";

function mediaVersion(
  id: string,
  versionNo: number,
  objectState: {
    currentVersionId: string;
    revision: number;
    status: API.MediaObjectResponse["status"];
  },
): API.MediaVersionResponse {
  return {
    id,
    workspace_id: workspaceId,
    media_object_id: mediaObjectId,
    media_object_kind: "image",
    media_object_source_type: "upload",
    media_object_status: objectState.status,
    media_object_current_version_id: objectState.currentVersionId,
    media_object_revision: objectState.revision,
    version_no: versionNo,
    filename: `角色参考-v${versionNo}.png`,
    sha256: String(versionNo).repeat(64),
    size_bytes: 2048,
    mime_type: "image/png",
    probe_status: "ready",
    probe_attempt: 1,
    probe_error_code: null,
    probe_error_summary: null,
    probe_next_action: null,
    width: 1024,
    height: 1024,
    duration_ms: null,
    codec: null,
    container: "png",
    created_at: now,
  };
}

const episode: API.EpisodeResponse = {
  id: episodeId,
  workspace_id: workspaceId,
  project_id: projectId,
  name: "第一集 · 雨巷相逢",
  position: 1,
  target_duration_ms: 90_000,
  status: "active",
  revision: 2,
  current_script_version_id: versionId,
  current_timeline_version_id: null,
};

const project: API.ProjectResponse = {
  id: projectId,
  workspace_id: workspaceId,
  name: "镜中长安",
  description: "水墨幻想漫剧",
  aspect_ratio: "9:16",
  language: "zh-CN",
  visual_style: "水墨幻想",
  target_duration_ms: 90_000,
  budget_limit: "1000.000000",
  currency: "CNY",
  status: "active",
  revision: 2,
};

const source: API.ScriptSourceResponse = {
  id: sourceId,
  workspace_id: workspaceId,
  episode_id: episodeId,
  input_type: "text",
  title: "第一集",
  source_media_version_id: null,
  rights_declaration: "原创测试文本",
  status: "active",
  revision: 1,
  created_at: now,
};

const version: API.ScriptVersionResponse = {
  id: versionId,
  workspace_id: workspaceId,
  source_id: sourceId,
  version_no: 2,
  status: "published",
  body: "第一场 雨巷\n顾清禾：你终于来了。",
  content_hash: "a".repeat(64),
  created_by: "019fb2c0-a000-7000-8000-000000000006",
  created_at: now,
};

const narrativeStructure: API.NarrativeStructureResponse = {
  id: "019fb2c0-a000-7000-8000-000000000017",
  workspace_id: workspaceId,
  episode_id: episodeId,
  script_version_id: versionId,
  input_hash: version.content_hash,
  parser_version: "deterministic-lines-v1",
  structure_hash: "d".repeat(64),
  dependency_hash: "e".repeat(64),
  revision: 1,
  units: [
    {
      id: "019fb2c0-a000-7000-8000-000000000018",
      unit_id: "019fb2c0-a000-7000-8000-000000000019",
      kind: "action",
      position: 1,
      version_no: 1,
      source_range: { start: 0, end: 6 },
      exact_text: "第一场 雨巷",
      text_hash: "f".repeat(64),
      prefix_text: "",
      suffix_text: version.body.slice(6),
      required_for_coverage: true,
      source_scene_id: null,
      source_dialogue_id: null,
      origin: "deterministic",
      created_at: now,
    },
  ],
  created_at: now,
  updated_at: now,
};

function extractionBatch(
  status: API.TaskResponse["status"] = "succeeded",
): API.ExtractionBatchResponse {
  const failed = status === "failed";
  return {
    id: batchId,
    workspace_id: workspaceId,
    script_version_id: versionId,
    scope: "full",
    extractor_version:
      "deepseek-v4-pro:thinking-off:lc-deepseek-1.1.0:prompt-v1:schema-v1",
    input_hash: version.content_hash,
    status,
    confirmed_script_version_id: null,
    candidate_count: status === "succeeded" ? 2 : 0,
    task: {
      id: taskId,
      workspace_id: workspaceId,
      task_type: "script_extraction",
      request_type: "extraction_batch",
      request_id: batchId,
      scope: {
        episode_id: episodeId,
        render_snapshot_id: null,
        usage_type: null,
        usage_id: null,
        input_version_id: versionId,
        input_hash: version.content_hash,
      },
      status,
      progress_stage: failed ? "blocked" : "completed",
      error: failed
        ? {
            code: "ai_service_unavailable",
            retryable: true,
            summary: "DeepSeek service is temporarily unavailable",
          }
        : null,
      next_action: failed ? "start_new_extraction" : "review_candidates",
      cancel_status: "none",
      revision: 2,
    },
    created_at: now,
  };
}

const sceneCandidates: API.ExtractionCandidateResponse[] = [
  {
    id: firstCandidateId,
    batch_id: batchId,
    candidate_key: "scene-001",
    kind: "scene",
    source_range: { start: 0, end: 5 },
    proposal: {
      kind: "scene",
      heading: "第一场",
      location: "雨巷",
      time_of_day: "夜",
      summary: "顾清禾等候来客",
      episode_number: null,
      scene_number: null,
      story_beat: null,
      characters: [],
      props: [],
      environment_details: null,
      continuity_notes: [],
      production_tasks: [],
    },
    confidence_note: "场景明确",
    required: true,
    status: "pending",
    revision: 1,
    created_at: now,
  },
  {
    id: secondCandidateId,
    batch_id: batchId,
    candidate_key: "scene-002",
    kind: "scene",
    source_range: { start: 6, end: 12 },
    proposal: {
      kind: "scene",
      heading: "第二场",
      location: "雨巷",
      time_of_day: "夜",
      summary: "来客出现",
      episode_number: null,
      scene_number: null,
      story_beat: null,
      characters: [],
      props: [],
      environment_details: null,
      continuity_notes: [],
      production_tasks: [],
    },
    confidence_note: null,
    required: true,
    status: "pending",
    revision: 1,
    created_at: now,
  },
];

const acceptedShotCandidate: API.ExtractionCandidateResponse = {
  id: shotCandidateId,
  batch_id: batchId,
  candidate_key: "shot-001",
  kind: "shot",
  source_range: { start: 13, end: 24 },
  proposal: {
    kind: "shot",
    scene_candidate_key: "scene-001",
    title: "雨中回望",
    purpose: "交代角色发现来客",
    shot_number: null,
    shot_type: null,
    framing: null,
    camera_movement: null,
    action: null,
    visual_prompt: null,
    dialogue_excerpt: null,
    asset_names: [],
    duration_ms: null,
    continuity_notes: [],
  },
  confidence_note: "镜头意图明确",
  required: false,
  status: "accepted",
  revision: 2,
  created_at: now,
};

const storyboardShot: API.ShotResponse = {
  id: shotId,
  workspace_id: workspaceId,
  episode_id: episodeId,
  position: 1,
  title: "雨巷建立镜头",
  source_script_version_id: versionId,
  source_scene_id: sceneId,
  source_candidate_id: null,
  source_draft_shot_id: null,
  status: "active",
  current_spec_version_id: null,
  revision: 1,
  created_at: now,
  updated_at: now,
};

function snapshot(
  scriptStatus: API.ScriptSummary["status"] = "published",
): API.EpisodeProductionSnapshot {
  return {
    episode_id: episodeId,
    current_stage: scriptStatus === "not_started" ? "script_import" : "structure_review",
    completion: scriptStatus === "not_started" ? 0 : 20,
    blocking_reasons: [
      {
        code: scriptStatus === "not_started" ? "SCRIPT_MISSING" : "EXTRACTION_MISSING",
        summary:
          scriptStatus === "not_started"
            ? "单集尚未导入剧本"
            : "当前剧本尚未提取结构",
        resource_type: "episode",
        resource_id: episodeId,
      },
    ],
    next_actions: [
      {
        code: scriptStatus === "not_started" ? "import_script" : "start_extraction",
        label: scriptStatus === "not_started" ? "导入剧本" : "开始结构提取",
        href: `/studio/${episodeId}/script`,
      },
    ],
    script_summary: {
      status: scriptStatus,
      current_version_id: scriptStatus === "not_started" ? null : versionId,
      extraction_batch_id: null,
      pending_required_candidates: 0,
    },
    asset_summary: {
      status: "not_started",
      total: 0,
      versioned: 0,
      ready: 0,
      draft: 0,
      blocked: 0,
      ready_kinds: [],
      required_kinds: ["character", "location", "voice"],
    },
    storyboard_summary: {
      status: "not_started",
      total: 0,
      ready: 0,
      blocked: 0,
      unavailable: 0,
    },
    task_summary: {
      status: "not_started",
      running: 0,
      failed: 0,
      succeeded: 0,
      unknown: 0,
    },
    review_summary: { status: "not_started", pending: 0 },
    cost_summary: {
      status: "not_started",
      currency: "CNY",
      reserved: "0.000000",
      used: "0.000000",
    },
    partial_failures: [],
    computed_at: now,
  };
}

describe("单集统一生产工作台", () => {
  beforeEach(() => {
    sessionStorage.clear();
    setAccessToken("test-access-token");
    vi.clearAllMocks();
    apiMocks.me.mockResolvedValue({
      data: {
        user: {
          id: version.created_by,
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
    apiMocks.getEpisode.mockResolvedValue({ data: episode });
    apiMocks.getProject.mockResolvedValue({ data: project });
    apiMocks.listEpisodes.mockResolvedValue({ data: [episode] });
    apiMocks.getSnapshot.mockResolvedValue({ data: snapshot() });
    apiMocks.listSources.mockResolvedValue({
      data: { items: [source], total: 1, limit: 100, offset: 0 },
    });
    apiMocks.getVersion.mockResolvedValue({ data: version });
    apiMocks.getNarrativeStructure.mockResolvedValue({ data: narrativeStructure });
    apiMocks.listVersions.mockResolvedValue({
      data: { items: [version], total: 1, limit: 100, offset: 0 },
    });
    apiMocks.listTasks.mockResolvedValue({
      data: { items: [], total: 0, limit: 100, offset: 0 },
    });
    apiMocks.listAssets.mockResolvedValue({
      data: { items: [], total: 0, limit: 100, offset: 0 },
    });
    apiMocks.getAssetBible.mockResolvedValue({
      data: {
        items: [],
        summary: {
          asset_count: 0,
          state_count: 0,
          ready: 0,
          draft: 0,
          blocked: 0,
          unavailable: 0,
        },
      },
    });
    apiMocks.listMedia.mockResolvedValue({
      data: { items: [], total: 0, limit: 100, offset: 0 },
    });
    apiMocks.listExports.mockResolvedValue({
      data: { items: [], total: 0 },
    });
    apiMocks.startExtraction.mockResolvedValue({
      data: {
        id: "019fb2c0-a000-7000-8000-000000000007",
        workspace_id: workspaceId,
        script_version_id: versionId,
        scope: "full",
        extractor_version: "unconfigured",
        input_hash: version.content_hash,
        status: "queued",
        confirmed_script_version_id: null,
        candidate_count: 0,
        task: {
          id: "019fb2c0-a000-7000-8000-000000000008",
          workspace_id: workspaceId,
          task_type: "script_extraction",
          request_type: "extraction_batch",
          request_id: "019fb2c0-a000-7000-8000-000000000007",
          scope: {
            episode_id: episodeId,
            render_snapshot_id: null,
            usage_type: null,
            usage_id: null,
            input_version_id: versionId,
            input_hash: version.content_hash,
          },
          status: "queued",
          progress_stage: "queued",
          error: null,
          next_action: "poll_task",
          cancel_status: "none",
          revision: 1,
        },
        created_at: now,
      },
    });
  });

  it("从服务端阶段进入当前剧本并启动可恢复提取任务", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <EpisodeProductionStudio episodeId={episodeId} initialPanel="script" />
      </AppProviders>,
    );

    expect(
      await screen.findByRole("heading", { name: "第一集 · 雨巷相逢" }),
    ).toBeInTheDocument();
    const workflow = screen.getByRole("complementary", { name: "项目制作流程导航" });
    expect(within(workflow).getByRole("link", { name: /剧本结构/ })).toHaveAttribute(
      "aria-current",
      "page",
    );
    expect(within(workflow).getByRole("link", { name: /分镜设计/ })).toHaveAttribute(
      "href",
      `/studio/${episodeId}/storyboard`,
    );
    await waitFor(() => expect(apiMocks.getVersion).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(apiMocks.listVersions).toHaveBeenCalledTimes(1));
    const editor = await screen.findByLabelText("当前剧本文本");
    await waitFor(() => expect(editor).toHaveValue(version.body));
    expect(screen.getAllByText("结构确认").length).toBeGreaterThan(0);

    await user.click(screen.getByRole("button", { name: "开始结构提取" }));
    await waitFor(() => expect(apiMocks.startExtraction).toHaveBeenCalledTimes(1));
    expect(apiMocks.startExtraction).toHaveBeenCalledWith(
      { version_id: versionId },
      expect.objectContaining({ scope: "full" }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent("提取任务已创建");
  });

  it("版本历史支持服务端差异、影响确认与安全清理", async () => {
    const user = userEvent.setup();
    const draftVersion: API.ScriptVersionResponse = {
      ...version,
      id: "019fb2c0-a000-7000-8000-000000000021",
      version_no: 1,
      status: "draft",
      body: "第一场 雨巷\n顾清禾：初稿。",
      content_hash: "b".repeat(64),
    };
    const previousVersion: API.ScriptVersionResponse = {
      ...version,
      id: "019fb2c0-a000-7000-8000-000000000022",
      version_no: 2,
      body: "第一场 雨巷\n顾清禾：旧版本。",
      content_hash: "c".repeat(64),
    };
    const currentVersion = { ...version, version_no: 3 };
    apiMocks.getVersion.mockResolvedValue({ data: currentVersion });
    apiMocks.listVersions.mockResolvedValue({
      data: {
        items: [draftVersion, previousVersion, currentVersion],
        total: 3,
        limit: 100,
        offset: 0,
      },
    });
    apiMocks.diffVersions.mockResolvedValue({
      data: {
        base_version_id: previousVersion.id,
        target_version_id: currentVersion.id,
        added_lines: 1,
        removed_lines: 1,
        diff_lines: [
          "--- version-2",
          "+++ version-3",
          "-顾清禾：旧版本。",
          "+顾清禾：你终于来了。",
        ],
      },
    });
    apiMocks.setCurrentVersion.mockResolvedValue({
      data: {
        episode_id: episodeId,
        current_script_version_id: previousVersion.id,
        episode_revision: 3,
        impact: {
          previous_script_version_id: currentVersion.id,
          current_script_version_id: previousVersion.id,
          affected_shot_ids: [shotId],
          narrative_impact_id: "019fb2c0-a000-7000-8000-000000000020",
          previous_narrative_dependency_hash: "1".repeat(64),
          current_narrative_dependency_hash: "2".repeat(64),
          invalidated_scopes: ["shot_readiness", "coverage", "export"],
        },
      },
    });
    apiMocks.deleteDraftVersion.mockResolvedValue({
      data: { deleted: true, script_version_id: draftVersion.id },
    });
    apiMocks.archiveSource.mockResolvedValue({
      data: { ...source, status: "archived", revision: 2 },
    });

    render(
      <AppProviders>
        <EpisodeProductionStudio episodeId={episodeId} initialPanel="script" />
      </AppProviders>,
    );

    await user.click(
      await screen.findByRole("button", { name: "比较 v2 与当前版本" }),
    );
    const diffDialog = await screen.findByRole("dialog", {
      name: "剧本版本差异",
    });
    expect(within(diffDialog).getByText("新增 1 行 · 删除 1 行")).toBeInTheDocument();
    expect(within(diffDialog).getByText("+顾清禾：你终于来了。")).toBeInTheDocument();
    expect(apiMocks.diffVersions).toHaveBeenCalledWith({
      version_id: previousVersion.id,
      other_version_id: currentVersion.id,
    });
    await user.click(within(diffDialog).getByRole("button", { name: "关闭" }));

    await user.click(screen.getByRole("button", { name: "设为当前 v2" }));
    const impactDialog = await screen.findByRole("dialog", {
      name: "版本切换影响",
    });
    expect(within(impactDialog).getByText("1 个镜头仍引用其他剧本版本")).toBeInTheDocument();
    expect(apiMocks.setCurrentVersion).toHaveBeenCalledWith(
      { episode_id: episodeId },
      {
        version_id: previousVersion.id,
        expected_current_version_id: currentVersion.id,
      },
    );
    await user.click(within(impactDialog).getByRole("button", { name: "知道了" }));

    await user.click(screen.getByRole("button", { name: "删除草稿 v1" }));
    const deleteDialog = await screen.findByRole("dialog", {
      name: "删除剧本草稿",
    });
    await user.click(
      within(deleteDialog).getByRole("button", { name: "确认删除 v1 草稿" }),
    );
    await waitFor(() => expect(apiMocks.deleteDraftVersion).toHaveBeenCalledWith({
      version_id: draftVersion.id,
      confirm: true,
    }));

    await user.click(screen.getByRole("button", { name: "归档剧本来源" }));
    await waitFor(() => expect(apiMocks.archiveSource).toHaveBeenCalledWith(
      { source_id: sourceId },
      { expected_revision: 1 },
    ));
  });

  it("在没有 current 剧本时导入真实文本来源", async () => {
    const user = userEvent.setup();
    apiMocks.getEpisode.mockResolvedValue({
      data: { ...episode, current_script_version_id: null, revision: 1 },
    });
    apiMocks.getSnapshot.mockResolvedValue({ data: snapshot("not_started") });
    apiMocks.listSources.mockResolvedValue({
      data: { items: [], total: 0, limit: 100, offset: 0 },
    });
    apiMocks.importScript.mockResolvedValue({ data: { source, version } });

    render(
      <AppProviders>
        <EpisodeProductionStudio episodeId={episodeId} initialPanel="script" />
      </AppProviders>,
    );

    await user.type(await screen.findByLabelText("剧本标题"), "第一集");
    await user.type(screen.getByLabelText("权利声明"), "原创测试文本");
    await user.type(screen.getByLabelText("剧本文本"), "第一场\n顾清禾：开始吧。");
    await user.click(screen.getByRole("button", { name: "导入剧本" }));

    await waitFor(() => expect(apiMocks.importScript).toHaveBeenCalledTimes(1));
    expect(apiMocks.importScript).toHaveBeenCalledWith(
      { episode_id: episodeId },
      expect.objectContaining({
        input_type: "text",
        title: "第一集",
        rights_declaration: "原创测试文本",
        body: "第一场\n顾清禾：开始吧。",
      }),
    );
  });

  it("允许对失败批次显式创建新的提取任务", async () => {
    const user = userEvent.setup();
    apiMocks.getSnapshot.mockResolvedValue({
      data: {
        ...snapshot("extraction_blocked"),
        script_summary: {
          status: "extraction_blocked",
          current_version_id: versionId,
          extraction_batch_id: batchId,
          pending_required_candidates: 0,
        },
      },
    });
    apiMocks.getBatch.mockResolvedValue({ data: extractionBatch("failed") });
    apiMocks.listCandidates.mockResolvedValue({
      data: { items: [], total: 0, limit: 100, offset: 0 },
    });

    render(
      <AppProviders>
        <EpisodeProductionStudio episodeId={episodeId} initialPanel="script" />
      </AppProviders>,
    );

    await user.click(
      await screen.findByRole("button", { name: "重新提取结构" }),
    );

    await waitFor(() => expect(apiMocks.startExtraction).toHaveBeenCalledTimes(1));
    expect(apiMocks.startExtraction).toHaveBeenCalledWith(
      { version_id: versionId },
      expect.objectContaining({
        scope: "full",
        idempotency_key: expect.stringMatching(
          new RegExp(`^studio-extraction:${versionId}:`),
        ),
      }),
    );
  });

  it("允许在人工决议前修改候选内容", async () => {
    const user = userEvent.setup();
    apiMocks.getSnapshot.mockResolvedValue({
      data: {
        ...snapshot("review_required"),
        blocking_reasons: [],
        next_actions: [],
        script_summary: {
          status: "review_required",
          current_version_id: versionId,
          extraction_batch_id: batchId,
          pending_required_candidates: 2,
        },
      },
    });
    apiMocks.getBatch.mockResolvedValue({ data: extractionBatch() });
    apiMocks.listCandidates.mockResolvedValue({
      data: { items: sceneCandidates, total: 2, limit: 100, offset: 0 },
    });
    apiMocks.decideCandidate.mockResolvedValue({ data: {} });

    render(
      <AppProviders>
        <EpisodeProductionStudio episodeId={episodeId} initialPanel="script" />
      </AppProviders>,
    );

    await user.click(
      await screen.findByRole("button", { name: "修改 scene-001 后接受" }),
    );
    const dialog = await screen.findByRole("dialog", { name: "修改后接受" });
    const summaryInput = within(dialog).getByLabelText("候选说明");
    await user.clear(summaryInput);
    await user.type(summaryInput, "顾清禾在雨巷等待重要来客");
    await user.click(within(dialog).getByRole("button", { name: "保存并接受" }));

    await waitFor(() => expect(apiMocks.decideCandidate).toHaveBeenCalledTimes(1));
    expect(apiMocks.decideCandidate).toHaveBeenCalledWith(
      { candidate_id: firstCandidateId },
      expect.objectContaining({
        expected_revision: 1,
        decision: {
          action: "accept_with_changes",
          proposal: {
            ...sceneCandidates[0].proposal,
            summary: "顾清禾在雨巷等待重要来客",
          },
        },
      }),
    );
  });

  it("允许将同类型候选合并到明确目标", async () => {
    const user = userEvent.setup();
    apiMocks.getSnapshot.mockResolvedValue({
      data: {
        ...snapshot("review_required"),
        blocking_reasons: [],
        next_actions: [],
        script_summary: {
          status: "review_required",
          current_version_id: versionId,
          extraction_batch_id: batchId,
          pending_required_candidates: 2,
        },
      },
    });
    apiMocks.getBatch.mockResolvedValue({ data: extractionBatch() });
    apiMocks.listCandidates.mockResolvedValue({
      data: { items: sceneCandidates, total: 2, limit: 100, offset: 0 },
    });
    apiMocks.decideCandidate.mockResolvedValue({ data: {} });

    render(
      <AppProviders>
        <EpisodeProductionStudio episodeId={episodeId} initialPanel="script" />
      </AppProviders>,
    );

    await user.click(
      await screen.findByRole("button", { name: "合并 scene-001" }),
    );
    const dialog = await screen.findByRole("dialog", { name: "合并候选" });
    await user.click(
      within(dialog).getByRole("button", { name: "合并到 scene-002" }),
    );

    await waitFor(() => expect(apiMocks.decideCandidate).toHaveBeenCalledTimes(1));
    expect(apiMocks.decideCandidate).toHaveBeenCalledWith(
      { candidate_id: firstCandidateId },
      expect.objectContaining({
        expected_revision: 1,
        decision: {
          action: "merge_into",
          target_candidate_id: secondCandidateId,
        },
      }),
    );
  });

  it("在分镜工作台接入候选建镜和标题修改命令", async () => {
    const user = userEvent.setup();
    apiMocks.getSnapshot.mockResolvedValue({
      data: {
        ...snapshot("confirmed"),
        current_stage: "storyboard_preparation",
        blocking_reasons: [],
        next_actions: [],
        script_summary: {
          status: "confirmed",
          current_version_id: versionId,
          extraction_batch_id: batchId,
          pending_required_candidates: 0,
        },
      },
    });
    apiMocks.getBatch.mockResolvedValue({
      data: {
        ...extractionBatch(),
        confirmed_script_version_id: versionId,
      },
    });
    apiMocks.listCandidates.mockResolvedValue({
      data: {
        items: [acceptedShotCandidate],
        total: 1,
        limit: 100,
        offset: 0,
      },
    });
    apiMocks.getConfirmedStructure.mockResolvedValue({
      data: {
        script_version_id: versionId,
        scenes: [
          {
            id: sceneId,
            script_version_id: versionId,
            position: 1,
            heading: "第一场 · 雨巷",
            location: "雨巷",
            time_of_day: "夜",
            summary: "顾清禾等待来客",
            source_range: { start: 0, end: 24 },
            dialogues: [],
            created_at: now,
          },
        ],
      },
    });
    apiMocks.listShots.mockResolvedValue({
      data: { items: [storyboardShot], order_hash: "a".repeat(64) },
    });
    apiMocks.listArchivedShots.mockResolvedValue({ data: [] });
    apiMocks.listShotSpecVersions.mockResolvedValue({ data: [] });
    apiMocks.listShotReadiness.mockResolvedValue({
      data: {
        episode_id: episodeId,
        summary: { total: 1, ready: 0, blocked: 1, unavailable: 0 },
        items: [],
        evaluation_hash: "b".repeat(64),
      },
    });
    apiMocks.getCoverage.mockResolvedValue({
      data: {
        episode_id: episodeId,
        status: "blocked",
        ready: false,
        basis_hash: "c".repeat(64),
        evaluation_hash: "d".repeat(64),
        summary: {
          required_total: 1,
          covered: 0,
          approved_omitted: 0,
          uncovered: 1,
          shots_total: 1,
          linked: 0,
          approved_invented: 0,
          orphan: 1,
          stale: 0,
        },
        units: [
          {
            narrative_unit_id: narrativeStructure.units[0].unit_id,
            unit_version_id: narrativeStructure.units[0].id,
            position: 1,
            kind: "action",
            exact_text: narrativeStructure.units[0].exact_text,
            required_for_coverage: true,
            required_channel: "visual",
            status: "uncovered",
            shot_ids: [],
          },
        ],
        shots: [
          {
            shot_id: shotId,
            spec_version_id: storyboardShot.current_spec_version_id,
            position: 1,
            title: storyboardShot.title,
            status: "orphan",
            unit_version_ids: [],
          },
        ],
        references: [],
        stale_reference_ids: [],
        stale_decision_ids: [],
        next_actions: [
          "map_or_omit_narrative_units",
          "map_or_approve_invented_shots",
        ],
      },
    });
    apiMocks.updateShot.mockResolvedValue({
      data: { ...storyboardShot, title: "雨巷全景", revision: 2 },
    });
    apiMocks.createShotFromCandidate.mockResolvedValue({
      data: {
        ...storyboardShot,
        id: "019fb2c0-a000-7000-8000-000000000014",
        position: 2,
        title: "雨中回望",
        source_candidate_id: shotCandidateId,
      },
    });

    render(
      <AppProviders>
        <EpisodeProductionStudio episodeId={episodeId} initialPanel="storyboard" />
      </AppProviders>,
    );

    expect(
      await screen.findByRole("heading", { name: "分镜设计" }),
    ).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "修改镜头标题" }));
    const titleInput = screen.getByLabelText("镜头标题");
    await user.clear(titleInput);
    await user.type(titleInput, "雨巷全景");
    await user.click(screen.getByRole("button", { name: "保存标题" }));
    await waitFor(() => expect(apiMocks.updateShot).toHaveBeenCalledTimes(1));
    expect(apiMocks.updateShot).toHaveBeenCalledWith(
      { shot_id: shotId },
      { expected_revision: 1, title: "雨巷全景" },
    );

    await user.click(screen.getByRole("button", { name: "新建镜头" }));
    await user.click(
      screen.getByRole("button", { name: "从候选建立 雨中回望" }),
    );
    await waitFor(() =>
      expect(apiMocks.createShotFromCandidate).toHaveBeenCalledTimes(1),
    );
    expect(apiMocks.createShotFromCandidate).toHaveBeenCalledWith({
      candidate_id: shotCandidateId,
    });
  });

  it("通过生成客户端切换媒体当前版本并完成归档恢复", async () => {
    const user = userEvent.setup();
    const objectState: {
      currentVersionId: string;
      revision: number;
      status: API.MediaObjectResponse["status"];
    } = {
      currentVersionId: secondMediaVersionId,
      revision: 2,
      status: "active",
    };
    const objectResponse = (): API.MediaObjectResponse => ({
      id: mediaObjectId,
      workspace_id: workspaceId,
      kind: "image",
      source_type: "upload",
      status: objectState.status,
      current_version_id: objectState.currentVersionId,
      revision: objectState.revision,
    });
    apiMocks.listMedia.mockImplementation(async () => ({
      data: {
        items: [
          mediaVersion(secondMediaVersionId, 2, objectState),
          mediaVersion(firstMediaVersionId, 1, objectState),
        ],
        total: 2,
        limit: 100,
        offset: 0,
      },
    }));
    apiMocks.setCurrentMediaVersion.mockImplementation(
      async (_params: unknown, body: API.CurrentMediaVersionRequest) => {
        objectState.currentVersionId = body.version_id;
        objectState.revision += 1;
        return { data: objectResponse() };
      },
    );
    apiMocks.archiveMedia.mockImplementation(async () => {
      objectState.status = "archived";
      objectState.revision += 1;
      return { data: objectResponse() };
    });
    apiMocks.restoreMedia.mockImplementation(async () => {
      objectState.status = "active";
      objectState.revision += 1;
      return { data: objectResponse() };
    });

    render(
      <AppProviders>
        <EpisodeProductionStudio episodeId={episodeId} initialPanel="media" />
      </AppProviders>,
    );

    expect(await screen.findByText("当前版本 v2")).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: "设为当前媒体版本 v1" }),
    );
    await waitFor(() =>
      expect(apiMocks.setCurrentMediaVersion).toHaveBeenCalledWith(
        { media_object_id: mediaObjectId },
        {
          version_id: firstMediaVersionId,
          expected_current_version_id: secondMediaVersionId,
          expected_revision: 2,
        },
      ),
    );
    expect(await screen.findByText("当前版本 v1")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "归档媒体" }));
    await waitFor(() =>
      expect(apiMocks.archiveMedia).toHaveBeenCalledWith(
        { media_object_id: mediaObjectId },
        { expected_revision: 3 },
      ),
    );
    expect(await screen.findByText("已归档")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "恢复媒体" }));
    await waitFor(() =>
      expect(apiMocks.restoreMedia).toHaveBeenCalledWith(
        { media_object_id: mediaObjectId },
        { expected_revision: 4 },
      ),
    );
    await waitFor(() =>
      expect(screen.queryByText("已归档")).not.toBeInTheDocument(),
    );
  });
});
