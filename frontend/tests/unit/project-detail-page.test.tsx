import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { AppProviders } from "@/app/providers";
import { ProjectWorkspace } from "@/app/(main)/projects/[projectId]/project-workspace";
import { setAccessToken } from "@/lib/auth-session";

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: vi.fn() }),
}));

vi.mock("@/api/identity", () => ({
  loginApiV1AuthLoginPost: vi.fn(),
  logoutApiV1AuthLogoutPost: vi.fn(),
  meApiV1MeGet: vi.fn(),
  registerApiV1AuthRegisterPost: vi.fn(),
}));

vi.mock("@/api/projects", () => ({
  createEpisodeApiV1ProjectsProjectIdEpisodesPost: vi.fn(),
  createProjectApiV1ProjectsPost: vi.fn(),
  getProjectApiV1ProjectsProjectIdGet: vi.fn().mockResolvedValue({
    data: {
      id: "019c0000-0000-7000-8000-000000000003",
      workspace_id: "019c0000-0000-7000-8000-000000000002",
      name: "海边来信",
      description: "竖屏悬疑短剧",
      aspect_ratio: "9:16",
      language: "zh-CN",
      visual_style: null,
      target_duration_ms: 90000,
      budget_limit: "0.000000",
      currency: "CNY",
      status: "active",
      revision: 2,
    },
  }),
  listEpisodesApiV1ProjectsProjectIdEpisodesGet: vi.fn().mockResolvedValue({
    data: [
      {
        id: "019c0000-0000-7000-8000-000000000004",
        workspace_id: "019c0000-0000-7000-8000-000000000002",
        project_id: "019c0000-0000-7000-8000-000000000003",
        name: "第一集",
        position: 1,
        target_duration_ms: 90000,
        status: "active",
        revision: 1,
        current_script_version_id: null,
        current_timeline_version_id: null,
      },
    ],
  }),
  listProjectsApiV1ProjectsGet: vi.fn(),
  projectProductionSnapshotApiV1ProjectsProjectIdProductionSnapshotGet: vi
    .fn()
    .mockResolvedValue({
      data: {
        project_id: "019c0000-0000-7000-8000-000000000003",
        current_stage: "script_import",
        completion: 10,
        blocking_reasons: [],
        next_actions: [
          {
            code: "import_script",
            label: "导入剧本",
            href: "/studio/019c0000-0000-7000-8000-000000000004/script",
          },
        ],
        episodes: [],
        partial_failures: [],
        computed_at: "2026-07-29T06:00:00Z",
      },
    }),
}));

describe("project production entry", () => {
  beforeEach(() => {
    setAccessToken("signed.jwt.token");
  });

  it("renders episode facts and the backend-provided next action", async () => {
    render(
      <AppProviders>
        <ProjectWorkspace projectId="019c0000-0000-7000-8000-000000000003" />
      </AppProviders>,
    );

    expect(await screen.findByRole("heading", { name: "海边来信" })).toBeInTheDocument();
    expect(await screen.findByText("第一集")).toBeInTheDocument();
    expect(await screen.findByText("导入剧本")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "创建单集" })).toBeInTheDocument();
  });
});
