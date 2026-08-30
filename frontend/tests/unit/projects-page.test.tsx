import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  createProject: vi.fn(),
  listProjects: vi.fn(),
  listWorkspaces: vi.fn(),
  me: vi.fn(),
}));

vi.mock("@/api/identity", async () => ({
  ...(await vi.importActual<typeof import("@/api/identity")>("@/api/identity")),
  listWorkspacesApiWorkspacesGet: apiMocks.listWorkspaces,
  meApiMeGet: apiMocks.me,
}));

vi.mock("@/api/projects", async () => ({
  ...(await vi.importActual<typeof import("@/api/projects")>("@/api/projects")),
  createProjectApiProjectsPost: apiMocks.createProject,
  listProjectsApiProjectsGet: apiMocks.listProjects,
}));

import { AppProviders } from "@/app/providers";
import { ProjectDashboard } from "@/app/projects/project-dashboard";
import { setAccessToken } from "@/lib/auth-session";

const workspaceId = "019fb2e0-a000-7000-8000-000000000001";
const project: API.ProjectResponse = {
  id: "019fb2e0-a000-7000-8000-000000000002",
  workspace_id: workspaceId,
  name: "镜中长安",
  description: "水墨幻想 AI 漫剧",
  aspect_ratio: "9:16",
  language: "zh-CN",
  visual_style: "水墨幻想",
  target_duration_ms: 90_000,
  status: "active",
  revision: 2,
};

describe("真实项目库", () => {
  beforeEach(() => {
    sessionStorage.clear();
    setAccessToken("test-access-token");
    vi.clearAllMocks();
    apiMocks.me.mockResolvedValue({
      data: {
        user: {
          id: "019fb2e0-a000-7000-8000-000000000003",
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
      data: { items: [project], total: 1, limit: 50, offset: 0 },
    });
    apiMocks.listWorkspaces.mockResolvedValue({
      data: [{
        id: workspaceId,
        name: "个人创作空间",
        status: "active",
        role: "owner",
        revision: 1,
      } satisfies API.WorkspaceResponse],
    });
    apiMocks.createProject.mockResolvedValue({ data: project });
  });

  it("读取服务端项目并支持本地搜索", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <ProjectDashboard />
      </AppProviders>,
    );

    expect(await screen.findByText("个人创作空间")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "项目管理" }).closest(".mx-auto")).toHaveClass("max-w-[1440px]");
    expect(await screen.findByRole("link", { name: `打开项目 ${project.name}` })).toHaveAttribute(
      "href",
      `/projects/${project.id}`,
    );
    await user.type(screen.getByRole("textbox", { name: "搜索项目" }), "不存在");
    expect(screen.queryByRole("link", { name: `打开项目 ${project.name}` })).not.toBeInTheDocument();
  });

  it("搜索无结果时可以一键恢复全部项目", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <ProjectDashboard />
      </AppProviders>,
    );

    expect(await screen.findByRole("link", { name: `打开项目 ${project.name}` })).toBeInTheDocument();
    await user.type(screen.getByRole("textbox", { name: "搜索项目" }), "不存在");

    expect(screen.getByRole("heading", { name: "没有匹配的项目" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "清除搜索和筛选" }));

    expect(screen.getByRole("link", { name: `打开项目 ${project.name}` })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: "搜索项目" })).toHaveValue("");
  });

  it("首次进入空项目库时提供就地创建入口", async () => {
    apiMocks.listProjects.mockResolvedValue({
      data: { items: [], total: 0, limit: 50, offset: 0 },
    });
    render(
      <AppProviders>
        <ProjectDashboard />
      </AppProviders>,
    );

    expect(await screen.findByRole("heading", { name: "还没有项目" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "创建第一个项目" })).toBeEnabled();
  });

  it("通过真实接口创建项目", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <ProjectDashboard />
      </AppProviders>,
    );

    const createButton = await screen.findByRole("button", { name: "创建项目" });
    await waitFor(() => expect(createButton).toBeEnabled());
    await user.click(createButton);
    await user.type(screen.getByLabelText("项目名称"), "镜中长安");
    await user.click(screen.getByRole("button", { name: "确认创建" }));

    await waitFor(() => expect(apiMocks.createProject).toHaveBeenCalledTimes(1));
    expect(apiMocks.createProject).toHaveBeenCalledWith(expect.objectContaining({
      workspace_id: workspaceId,
      name: "镜中长安",
      aspect_ratio: "9:16",
      language: "zh-CN",
    }));
    expect(await screen.findByRole("status")).toHaveTextContent("项目已创建");
  });
});
