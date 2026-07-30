import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  createEpisode: vi.fn(),
  getProject: vi.fn(),
  getSnapshot: vi.fn(),
  listEpisodes: vi.fn(),
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

import { AppProviders } from "@/app/providers";
import { ProjectWorkspace } from "@/app/projects/[projectId]/project-workspace";
import { setAccessToken } from "@/lib/auth-session";

const workspaceId = "019fb2d0-a000-7000-8000-000000000001";
const projectId = "019fb2d0-a000-7000-8000-000000000002";
const episodeId = "019fb2d0-a000-7000-8000-000000000003";

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
  computed_at: "2026-07-30T09:00:00Z",
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
    apiMocks.listEpisodes.mockResolvedValue({ data: [episode] });
    apiMocks.getSnapshot.mockResolvedValue({
      data: {
        project_id: projectId,
        current_stage: "script_import",
        completion: 0,
        blocking_reasons: episodeSnapshot.blocking_reasons,
        next_actions: episodeSnapshot.next_actions,
        episodes: [episodeSnapshot],
        partial_failures: [],
        computed_at: "2026-07-30T09:00:00Z",
      } satisfies API.ProjectProductionSnapshot,
    });
    apiMocks.createEpisode.mockResolvedValue({
      data: { ...episode, id: "019fb2d0-a000-7000-8000-000000000005", position: 2 },
    });
  });

  it("展示服务端生产事实并进入单集", async () => {
    render(
      <AppProviders>
        <ProjectWorkspace projectId={projectId} />
      </AppProviders>,
    );

    expect(await screen.findByRole("heading", { name: project.name })).toBeInTheDocument();
    expect(screen.getByText("单集尚未导入剧本")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "进入第一集 · 雨巷相逢" })).toHaveAttribute(
      "href",
      `/studio/${episodeId}/script`,
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
    expect(await screen.findByRole("status")).toHaveTextContent("第 2 集已创建");
  });
});
