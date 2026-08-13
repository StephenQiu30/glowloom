import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  createEpisode: vi.fn(),
  completeUpload: vi.fn(),
  getProject: vi.fn(),
  getMedia: vi.fn(),
  getSnapshot: vi.fn(),
  importScriptDocument: vi.fn(),
  initializeUpload: vi.fn(),
  listEpisodes: vi.fn(),
  listScriptDocuments: vi.fn(),
  me: vi.fn(),
}));

vi.mock("@/api/identity", async () => ({
  ...(await vi.importActual<typeof import("@/api/identity")>("@/api/identity")),
  meApiV1MeGet: apiMocks.me,
}));

vi.mock("@/api/projects", async () => ({
  ...(await vi.importActual<typeof import("@/api/projects")>("@/api/projects")),
  createEpisodeApiV1ProjectsProjectIdEpisodesPost: apiMocks.createEpisode,
  getProjectApiV1ProjectsProjectIdGet: apiMocks.getProject,
  listEpisodesApiV1ProjectsProjectIdEpisodesGet: apiMocks.listEpisodes,
  projectProductionSnapshotApiV1ProjectsProjectIdProductionSnapshotGet:
    apiMocks.getSnapshot,
}));

vi.mock("@/api/media", async () => ({
  ...(await vi.importActual<typeof import("@/api/media")>("@/api/media")),
  completeUploadApiV1MediaUploadsUploadSessionIdCompletePost:
    apiMocks.completeUpload,
  getMediaApiV1MediaVersionIdGet: apiMocks.getMedia,
  initializeUploadApiV1MediaUploadsPost: apiMocks.initializeUpload,
}));

vi.mock("@/api/scriptDocuments", async () => ({
  ...(await vi.importActual<typeof import("@/api/scriptDocuments")>(
    "@/api/scriptDocuments",
  )),
  importDocumentApiV1ProjectsProjectIdScriptImportsPost:
    apiMocks.importScriptDocument,
  listDocumentsApiV1ProjectsProjectIdScriptDocumentsGet:
    apiMocks.listScriptDocuments,
}));

import { AppProviders } from "@/app/providers";
import { ProjectWorkspace } from "@/app/projects/[projectId]/project-workspace";
import { setAccessToken } from "@/lib/auth-session";

const workspaceId = "019fb2d0-a000-7000-8000-000000000001";
const projectId = "019fb2d0-a000-7000-8000-000000000002";
const episodeId = "019fb2d0-a000-7000-8000-000000000003";
const mediaVersionId = "019fb2d0-a000-7000-8000-000000000020";

const project: API.ProjectResponse = {
  id: projectId,
  workspace_id: workspaceId,
  name: "镜中长安",
  description: "水墨幻想 AI 漫剧",
  aspect_ratio: "9:16",
  language: "zh-CN",
  visual_style: "水墨幻想",
  target_duration_ms: 90_000,
  budget_limit: "1000.000000",
  currency: "CNY",
  status: "active",
  revision: 2,
};

const episode: API.EpisodeResponse = {
  id: episodeId,
  workspace_id: workspaceId,
  project_id: projectId,
  name: "第一集 · 雨巷相逢",
  position: 1,
  target_duration_ms: 90_000,
  status: "active",
  revision: 2,
  current_script_version_id: null,
  current_timeline_version_id: null,
};

const secondEpisode: API.EpisodeResponse = {
  ...episode,
  id: "019fb2d0-a000-7000-8000-000000000005",
  name: "第二集 · 城门旧事",
  position: 2,
};

const episodeSnapshot: API.EpisodeProductionSnapshot = {
  episode_id: episodeId,
  current_stage: "script_import",
  completion: 0,
  blocking_reasons: [{
    code: "SCRIPT_MISSING",
    summary: "单集尚未导入剧本",
    resource_type: "episode",
    resource_id: episodeId,
  }],
  next_actions: [{
    code: "import_script",
    label: "导入第一集剧本",
    href: `/studio/${episodeId}/script`,
  }],
  script_summary: {
    status: "not_started",
    current_version_id: null,
    extraction_batch_id: null,
    pending_required_candidates: 0,
  },
  asset_summary: {
    status: "ready",
    total: 3,
    versioned: 3,
    ready: 3,
    draft: 0,
    blocked: 0,
    ready_kinds: ["character", "location", "voice"],
    required_kinds: ["character", "location", "voice"],
  },
  storyboard_summary: {
    status: "blocked",
    total: 2,
    ready: 1,
    blocked: 1,
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
  computed_at: "2026-07-30T09:00:00Z",
};

const documentAnalysis: API.ScriptDocumentAnalysisResponse = {
  document: {
    id: "019fb2d0-a000-7000-8000-000000000010",
    workspace_id: workspaceId,
    project_id: projectId,
    title: `${project.name} · 整剧原稿`,
    source_type: "text",
    source_media_version_id: null,
    language: "zh-CN",
    rights_declaration: "我确认拥有该剧本用于本项目制作与分析的权利",
    status: "active",
    revision: 1,
    created_by: "019fb2d0-a000-7000-8000-000000000004",
    created_at: "2026-08-13T03:00:00Z",
  },
  revision: {
    id: "019fb2d0-a000-7000-8000-000000000011",
    workspace_id: workspaceId,
    document_id: "019fb2d0-a000-7000-8000-000000000010",
    version_no: 1,
    source_type: "text",
    source_media_version_id: null,
    raw_text: "场景：控制室。\n甲：开始。",
    raw_hash: "a".repeat(64),
    normalized_text: "场景：控制室。\n甲：开始。",
    normalized_hash: "a".repeat(64),
    normalizer_version: "identity-v1",
    normalization_map: { type: "identity" },
    codepoint_count: 14,
    analysis_status: "ai_candidate_required",
    analyzer_version: "whole-script-lines-v1",
    created_by: "019fb2d0-a000-7000-8000-000000000004",
    created_at: "2026-08-13T03:00:00Z",
  },
  blocks: [
    {
      id: "019fb2d0-a000-7000-8000-000000000012",
      document_revision_id: "019fb2d0-a000-7000-8000-000000000011",
      position: 1,
      kind: "scene_heading",
      source_start: 0,
      source_end: 8,
      text_hash: "b".repeat(64),
      metadata: {},
    },
    {
      id: "019fb2d0-a000-7000-8000-000000000013",
      document_revision_id: "019fb2d0-a000-7000-8000-000000000011",
      position: 2,
      kind: "dialogue",
      source_start: 8,
      source_end: 14,
      text_hash: "c".repeat(64),
      metadata: {},
    },
  ],
  issues: [
    {
      id: "019fb2d0-a000-7000-8000-000000000014",
      document_revision_id: "019fb2d0-a000-7000-8000-000000000011",
      position: 1,
      code: "no_marker",
      severity: "warning",
      source_start: 0,
      source_end: 1,
      line_number: 1,
      column_number: 1,
      next_action: "generate_episode_plan",
      details: {},
    },
  ],
};

describe("真实项目生产入口", () => {
  beforeEach(() => {
    sessionStorage.clear();
    setAccessToken("test-access-token");
    vi.clearAllMocks();
    apiMocks.me.mockResolvedValue({
      data: {
        user: {
          id: "019fb2d0-a000-7000-8000-000000000004",
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
    apiMocks.getProject.mockResolvedValue({ data: project });
    apiMocks.listEpisodes.mockResolvedValue({ data: [episode, secondEpisode] });
    apiMocks.listScriptDocuments.mockResolvedValue({
      data: { items: [], total: 0, limit: 100, offset: 0 },
    });
    apiMocks.importScriptDocument.mockResolvedValue({ data: documentAnalysis });
    apiMocks.initializeUpload.mockResolvedValue({
      data: {
        upload_session: {
          id: "019fb2d0-a000-7000-8000-000000000021",
          workspace_id: workspaceId,
          media_object_id: null,
          status: "pending",
          kind: "document",
          filename: "whole-script.md",
          size_bytes: 32,
          mime_type: "text/markdown",
          sha256: "d".repeat(64),
          expires_at: "2026-08-13T03:05:00Z",
        },
        upload: {
          method: "PUT",
          url: "https://private-storage.test/upload/script",
          headers: { "content-type": "text/markdown" },
          expires_at: "2026-08-13T03:05:00Z",
        },
      },
    });
    apiMocks.completeUpload.mockResolvedValue({
      data: {
        media_object: {
          id: "019fb2d0-a000-7000-8000-000000000022",
          workspace_id: workspaceId,
          kind: "document",
          source_type: "upload",
          status: "active",
          current_version_id: mediaVersionId,
          revision: 1,
        },
        version: {
          id: mediaVersionId,
          workspace_id: workspaceId,
          media_object_id: "019fb2d0-a000-7000-8000-000000000022",
          media_object_kind: "document",
          media_object_source_type: "upload",
          media_object_status: "active",
          media_object_current_version_id: mediaVersionId,
          media_object_revision: 1,
          version_no: 1,
          filename: "whole-script.md",
          sha256: "d".repeat(64),
          size_bytes: 32,
          mime_type: "text/markdown",
          probe_status: "pending",
          probe_attempt: 1,
          probe_error_code: null,
          probe_error_summary: null,
          probe_next_action: null,
          width: null,
          height: null,
          duration_ms: null,
          codec: null,
          container: null,
          created_at: "2026-08-13T03:00:00Z",
        },
        probe_task: {
          id: "019fb2d0-a000-7000-8000-000000000023",
          workspace_id: workspaceId,
          task_type: "media_probe",
          request_type: "media_version",
          request_id: mediaVersionId,
          status: "queued",
          progress_stage: "queued",
          progress_percent: 0,
          revision: 1,
          attempt_count: 0,
          max_attempts: 3,
          retryable: false,
          next_action: null,
          error_code: null,
          error_summary: null,
          created_at: "2026-08-13T03:00:00Z",
          updated_at: "2026-08-13T03:00:00Z",
          started_at: null,
          finished_at: null,
        },
      },
    });
    apiMocks.getMedia.mockResolvedValue({
      data: {
        id: mediaVersionId,
        workspace_id: workspaceId,
        media_object_id: "019fb2d0-a000-7000-8000-000000000022",
        media_object_kind: "document",
        media_object_source_type: "upload",
        media_object_status: "active",
        media_object_current_version_id: mediaVersionId,
        media_object_revision: 1,
        version_no: 1,
        filename: "whole-script.md",
        sha256: "d".repeat(64),
        size_bytes: 32,
        mime_type: "text/markdown",
        probe_status: "ready",
        probe_attempt: 1,
        probe_error_code: null,
        probe_error_summary: null,
        probe_next_action: null,
        width: null,
        height: null,
        duration_ms: null,
        codec: null,
        container: "plain-text",
        created_at: "2026-08-13T03:00:00Z",
      },
    });
    apiMocks.getSnapshot.mockResolvedValue({
      data: {
        project_id: projectId,
        current_stage: "script_import",
        completion: 0,
        blocking_reasons: episodeSnapshot.blocking_reasons,
        next_actions: episodeSnapshot.next_actions,
        episodes: [
          episodeSnapshot,
          {
            ...episodeSnapshot,
            episode_id: secondEpisode.id,
            storyboard_summary: {
              status: "ready",
              total: 2,
              ready: 2,
              blocked: 0,
              unavailable: 0,
            },
          },
        ],
        partial_failures: [],
        computed_at: "2026-07-30T09:00:00Z",
      } satisfies API.ProjectProductionSnapshot,
    });
    apiMocks.createEpisode.mockResolvedValue({
      data: {
        ...episode,
        id: "019fb2d0-a000-7000-8000-000000000006",
        position: 3,
      },
    });
  });

  it("展示服务端生产事实并进入单集", async () => {
    render(
      <AppProviders>
        <ProjectWorkspace projectId={projectId} />
      </AppProviders>,
    );

    expect(await screen.findByRole("heading", { name: project.name })).toBeInTheDocument();
    expect(screen.getAllByText("单集尚未导入剧本").length).toBeGreaterThan(0);
    expect(screen.getByRole("link", { name: "进入第一集 · 雨巷相逢" })).toHaveAttribute(
      "href",
      `/studio/${episodeId}/script`,
    );
    const summary = screen.getByRole("region", { name: "项目生产摘要" });
    expect(within(summary).getByText("Ready 资产").nextElementSibling).toHaveTextContent(
      "3",
    );
    expect(within(summary).getByText("Ready 分镜").nextElementSibling).toHaveTextContent(
      "3",
    );
  });

  it("通过真实接口创建下一集", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <ProjectWorkspace projectId={projectId} />
      </AppProviders>,
    );

    await user.click(await screen.findByRole("button", { name: "创建单集" }));
    await user.type(screen.getByLabelText("单集名称"), "第二集 · 城门旧事");
    await user.click(screen.getByRole("button", { name: "确认创建" }));

    await waitFor(() => expect(apiMocks.createEpisode).toHaveBeenCalledTimes(1));
    expect(apiMocks.createEpisode).toHaveBeenCalledWith(
      { project_id: projectId },
      { name: "第二集 · 城门旧事", target_duration_ms: 90_000 },
    );
    expect(await screen.findByRole("status")).toHaveTextContent("第 3 集已创建");
  });

  it("粘贴整剧原稿并展示可执行的格式体检下一步", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <ProjectWorkspace projectId={projectId} />
      </AppProviders>,
    );

    await screen.findByRole("heading", { name: project.name });
    await user.type(screen.getByLabelText("整剧文本"), "场景：控制室。\n甲：开始。");
    await user.click(
      screen.getByRole("checkbox", {
        name: "我确认拥有该剧本用于本项目制作与分析的权利",
      }),
    );
    await user.click(screen.getByRole("button", { name: "导入并分析" }));

    await waitFor(() =>
      expect(apiMocks.importScriptDocument).toHaveBeenCalledTimes(1),
    );
    expect(apiMocks.importScriptDocument).toHaveBeenCalledWith(
      { project_id: projectId },
      expect.objectContaining({
        input_type: "text",
        title: `${project.name} · 整剧原稿`,
        text: "场景：控制室。\n甲：开始。",
        media_version_id: null,
        language: "zh-CN",
        idempotency_key: expect.stringMatching(/^script-document:[a-f0-9]{64}$/),
      }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent("格式体检已完成");
    expect(screen.getByText("需要 AI 分集候选")).toBeInTheDocument();
    expect(screen.getByText(/未找到独占一行的集标记/)).toBeInTheDocument();
    expect(screen.getByText("下一步：生成并人工确认分集建议")).toBeInTheDocument();
  });

  it("把 UTF-8 Markdown 固定为 document 媒体版本后再执行整剧分析", async () => {
    const user = userEvent.setup();
    const uploadFetch = vi
      .fn()
      .mockResolvedValue({ ok: true, status: 200 });
    vi.stubGlobal("fetch", uploadFetch);
    render(
      <AppProviders>
        <ProjectWorkspace projectId={projectId} />
      </AppProviders>,
    );

    await screen.findByRole("heading", { name: project.name });
    await user.selectOptions(
      screen.getByLabelText("导入方式"),
      "media",
    );
    const file = new File(
      ["第一集\n场景1：控制室，夜\n甲：开始。"],
      "whole-script.md",
      { type: "text/markdown" },
    );
    Object.defineProperty(file, "arrayBuffer", {
      value: async () =>
        new TextEncoder().encode("第一集\n场景1：控制室，夜\n甲：开始。").buffer,
    });
    await user.upload(screen.getByLabelText("剧本文档"), file);
    await user.click(
      screen.getByRole("checkbox", {
        name: "我确认拥有该剧本用于本项目制作与分析的权利",
      }),
    );
    await user.click(screen.getByRole("button", { name: "上传并分析" }));

    await waitFor(() => expect(apiMocks.initializeUpload).toHaveBeenCalled());
    await waitFor(() => expect(apiMocks.completeUpload).toHaveBeenCalled());
    await waitFor(() => expect(apiMocks.getMedia).toHaveBeenCalled());
    await waitFor(() => expect(apiMocks.importScriptDocument).toHaveBeenCalled());
    expect(apiMocks.initializeUpload).toHaveBeenCalledWith(
      expect.objectContaining({
        workspace_id: workspaceId,
        kind: "document",
        filename: "whole-script.md",
        mime_type: "text/markdown",
        idempotency_key: expect.stringMatching(
          /^script-document-upload:[a-f0-9]{64}$/,
        ),
      }),
    );
    expect(uploadFetch).toHaveBeenCalledWith(
      "https://private-storage.test/upload/script",
      expect.objectContaining({ method: "PUT", body: file }),
    );
    expect(apiMocks.getMedia).toHaveBeenCalledWith({
      version_id: mediaVersionId,
    });
    expect(apiMocks.importScriptDocument).toHaveBeenCalledWith(
      { project_id: projectId },
      expect.objectContaining({
        input_type: "media",
        text: null,
        media_version_id: mediaVersionId,
        idempotency_key: expect.stringMatching(/^script-document:[a-f0-9]{64}$/),
      }),
    );
    vi.unstubAllGlobals();
  });
});
