import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  completeUpload: vi.fn(),
  confirmStructure: vi.fn(),
  decideCandidate: vi.fn(),
  getBatch: vi.fn(),
  getEpisode: vi.fn(),
  getProject: vi.fn(),
  getSnapshot: vi.fn(),
  getVersion: vi.fn(),
  importScript: vi.fn(),
  initializeUpload: vi.fn(),
  listAssets: vi.fn(),
  listCandidates: vi.fn(),
  listEpisodes: vi.fn(),
  listMedia: vi.fn(),
  listSources: vi.fn(),
  listTasks: vi.fn(),
  listVersions: vi.fn(),
  me: vi.fn(),
  publishVersion: vi.fn(),
  retryProbe: vi.fn(),
  setCurrentVersion: vi.fn(),
  startExtraction: vi.fn(),
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
  confirmStructureApiV1ExtractionBatchesBatchIdConfirmStructurePost:
    apiMocks.confirmStructure,
  decideExtractionCandidateApiV1ExtractionCandidatesCandidateIdDecisionsPost:
    apiMocks.decideCandidate,
  getExtractionBatchApiV1ExtractionBatchesBatchIdGet: apiMocks.getBatch,
  getVersionApiV1ScriptVersionsVersionIdGet: apiMocks.getVersion,
  importTextSourceApiV1EpisodesEpisodeIdScriptSourcesPost: apiMocks.importScript,
  listExtractionCandidatesApiV1ExtractionBatchesBatchIdCandidatesGet:
    apiMocks.listCandidates,
  listSourcesApiV1EpisodesEpisodeIdScriptSourcesGet: apiMocks.listSources,
  listVersionsApiV1ScriptSourcesSourceIdVersionsGet: apiMocks.listVersions,
  publishVersionApiV1ScriptSourcesSourceIdVersionsPost: apiMocks.publishVersion,
  setCurrentVersionApiV1EpisodesEpisodeIdCurrentScriptVersionPost:
    apiMocks.setCurrentVersion,
  startExtractionApiV1ScriptVersionsVersionIdExtractionsPost:
    apiMocks.startExtraction,
}));

vi.mock("@/api/tasks", async () => ({
  ...(await vi.importActual<typeof import("@/api/tasks")>("@/api/tasks")),
  listTasksApiV1TasksGet: apiMocks.listTasks,
}));

vi.mock("@/api/media", async () => ({
  ...(await vi.importActual<typeof import("@/api/media")>("@/api/media")),
  completeUploadApiV1MediaUploadsUploadSessionIdCompletePost:
    apiMocks.completeUpload,
  initializeUploadApiV1MediaUploadsPost: apiMocks.initializeUpload,
  listMediaApiV1MediaGet: apiMocks.listMedia,
  retryProbeApiV1MediaVersionIdProbeRetryPost: apiMocks.retryProbe,
}));

vi.mock("@/api/assets", async () => ({
  ...(await vi.importActual<typeof import("@/api/assets")>("@/api/assets")),
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
const now = "2026-07-30T09:00:00Z";

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
    },
    confidence_note: null,
    required: true,
    status: "pending",
    revision: 1,
    created_at: now,
  },
];

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
    apiMocks.listVersions.mockResolvedValue({
      data: { items: [version], total: 1, limit: 100, offset: 0 },
    });
    apiMocks.listTasks.mockResolvedValue({
      data: { items: [], total: 0, limit: 100, offset: 0 },
    });
    apiMocks.listAssets.mockResolvedValue({
      data: { items: [], total: 0, limit: 100, offset: 0 },
    });
    apiMocks.listMedia.mockResolvedValue({
      data: { items: [], total: 0, limit: 100, offset: 0 },
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
});
