import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { ProjectDashboard } from "@/app/projects/project-dashboard";
import { AppProviders } from "@/app/providers";
import { setAccessToken } from "@/lib/auth-session";

const replace = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace }),
}));

vi.mock("@/api/identity", () => ({
  meApiV1MeGet: vi.fn().mockResolvedValue({
    data: {
      user: {
        id: "019c0000-0000-7000-8000-000000000001",
        email: "maker@example.com",
        display_name: "创作者",
        avatar_url: null,
      },
      workspace: {
        id: "019c0000-0000-7000-8000-000000000002",
        name: "创作者的工作空间",
        status: "active",
        role: "owner",
        revision: 1,
      },
    },
  }),
  loginApiV1AuthLoginPost: vi.fn(),
  listWorkspacesApiV1WorkspacesGet: vi.fn().mockResolvedValue({
    data: [
      {
        id: "019c0000-0000-7000-8000-000000000002",
        name: "创作者的工作空间",
        status: "active",
        role: "owner",
        revision: 1,
      },
    ],
  }),
  logoutApiV1AuthLogoutPost: vi.fn(),
  registerApiV1AuthRegisterPost: vi.fn(),
}));

vi.mock("@/api/projects", () => ({
  createProjectApiV1ProjectsPost: vi.fn(),
  listProjectsApiV1ProjectsGet: vi.fn().mockResolvedValue({
    data: {
      items: [
        {
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
          revision: 1,
        },
      ],
      total: 1,
      limit: 50,
      offset: 0,
    },
  }),
}));

describe("projects workspace", () => {
  beforeEach(() => {
    setAccessToken("signed.jwt.token");
    replace.mockClear();
  });

  it("renders server-owned workspace and project facts", async () => {
    render(
      <AppProviders>
        <ProjectDashboard />
      </AppProviders>,
    );

    expect(await screen.findByRole("heading", { name: "项目" })).toBeInTheDocument();
    expect(screen.getByText("创作者的工作空间")).toBeInTheDocument();
    expect(await screen.findByRole("link", { name: /海边来信/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "创建项目" })).toBeInTheDocument();
  });
});
