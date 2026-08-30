import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  createWorkspace: vi.fn(),
  listWorkspaces: vi.fn(),
  me: vi.fn(),
  updateMe: vi.fn(),
}));

vi.mock("@/api/identity", async () => ({
  ...(await vi.importActual<typeof import("@/api/identity")>("@/api/identity")),
  createWorkspaceApiWorkspacesPost: apiMocks.createWorkspace,
  listWorkspacesApiWorkspacesGet: apiMocks.listWorkspaces,
  meApiMeGet: apiMocks.me,
  updateMeApiMePatch: apiMocks.updateMe,
}));

import { AppProviders } from "@/app/providers";
import WorkspacesPage from "@/app/workspaces/page";
import { setAccessToken } from "@/lib/auth-session";

const currentWorkspace: API.WorkspaceResponse = {
  id: "019fb2f0-a000-7000-8000-000000000001",
  name: "个人创作空间",
  status: "active",
  role: "owner",
  revision: 1,
};

const me: API.MeResponse = {
  user: {
    id: "019fb2f0-a000-7000-8000-000000000002",
    email: "creator@example.com",
    display_name: "创作者",
    avatar_url: null,
  },
  workspace: currentWorkspace,
};

describe("真实账户与工作空间设置", () => {
  beforeEach(() => {
    sessionStorage.clear();
    setAccessToken("test-access-token");
    vi.clearAllMocks();
    apiMocks.me.mockResolvedValue({ data: me });
    apiMocks.listWorkspaces.mockResolvedValue({ data: [currentWorkspace] });
    apiMocks.updateMe.mockResolvedValue({ data: me });
    apiMocks.createWorkspace.mockResolvedValue({
      data: {
        id: "019fb2f0-a000-7000-8000-000000000003",
        name: "青墨工作室",
        status: "active",
        role: "owner",
        revision: 1,
      } satisfies API.WorkspaceResponse,
    });
  });

  it("保存服务端个人资料并创建工作空间", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <WorkspacesPage />
      </AppProviders>,
    );

    const displayName = await screen.findByLabelText("显示名称");
    expect(displayName).toHaveValue("创作者");
    expect(screen.getByText(me.user.email)).toBeInTheDocument();
    await user.clear(displayName);
    await user.type(displayName, "总导演");
    await user.click(screen.getByRole("button", { name: "保存个人资料" }));
    await waitFor(() => expect(apiMocks.updateMe).toHaveBeenCalledWith({
      display_name: "总导演",
      avatar_url: null,
    }));

    await user.type(screen.getByRole("textbox", { name: "空间名称" }), "青墨工作室");
    await user.click(screen.getByRole("button", { name: "创建工作空间" }));
    await waitFor(() => expect(apiMocks.createWorkspace).toHaveBeenCalledWith({ name: "青墨工作室" }));
    expect(await screen.findByRole("status")).toHaveTextContent("青墨工作室");
  });
});
