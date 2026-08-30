import { render, screen, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  listProjects: vi.fn(),
  me: vi.fn(),
  routerReplace: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace: apiMocks.routerReplace }),
}));

vi.mock("@/api/identity", async () => ({
  ...(await vi.importActual<typeof import("@/api/identity")>("@/api/identity")),
  meApiMeGet: apiMocks.me,
}));

vi.mock("@/api/projects", async () => ({
  ...(await vi.importActual<typeof import("@/api/projects")>("@/api/projects")),
  listProjectsApiProjectsGet: apiMocks.listProjects,
}));

import Home from "@/app/page";
import { AppProviders } from "@/app/providers";
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

describe("创作首页", () => {
  beforeEach(() => {
    sessionStorage.clear();
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
      } satisfies API.MeResponse,
    });
    apiMocks.listProjects.mockResolvedValue({
      data: { items: [project], total: 1, limit: 50, offset: 0 },
    });
  });

  it("为未登录用户展示可执行的注册和登录入口", async () => {
    render(
      <AppProviders>
        <Home />
      </AppProviders>,
    );

    expect(
      await screen.findByRole("heading", {
        name: /把剧本，变成.*可追踪的成片。/,
      }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "导入剧本" })).toHaveAttribute(
      "href",
      "/register",
    );
    expect(screen.getByRole("link", { name: "继续制作" })).toHaveAttribute(
      "href",
      "/login",
    );
    expect(screen.queryByText("项目草稿已生成")).not.toBeInTheDocument();
  });

  it("将产品文案与封面放在上层并把生产链独立到下层", async () => {
    render(
      <AppProviders>
        <Home />
      </AppProviders>,
    );

    const introduction = await screen.findByRole("region", { name: "产品欢迎" });
    const pipeline = screen.getByRole("region", { name: "可恢复的生产链" });

    expect(within(introduction).getByRole("heading", { name: /把剧本，变成.*可追踪的成片。/ })).toBeInTheDocument();
    expect(within(introduction).getByRole("img", { name: "长安夜航项目封面" })).toBeInTheDocument();
    expect(within(pipeline).getAllByRole("listitem")).toHaveLength(5);
    expect(introduction.compareDocumentPosition(pipeline) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it("已登录时不展示首页并直接进入项目工作区", async () => {
    setAccessToken("test-access-token");
    render(
      <AppProviders>
        <Home />
      </AppProviders>,
    );

    expect(await screen.findByLabelText("正在进入项目工作区")).toBeInTheDocument();
    expect(apiMocks.routerReplace).toHaveBeenCalledWith("/projects");
    expect(screen.queryByRole("heading", { name: /欢迎回来/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: /把剧本，变成/ })).not.toBeInTheDocument();
    expect(apiMocks.me).not.toHaveBeenCalled();
    expect(apiMocks.listProjects).not.toHaveBeenCalled();
  });
});
