import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import WorkspacesPage from "@/app/workspaces/page";
import { AppProviders } from "@/app/providers";
import { setAccessToken } from "@/lib/auth-session";

const replace = vi.fn();

vi.mock("next/navigation", () => ({
  useRouter: () => ({ replace }),
}));

vi.mock("@/api/identity", () => ({
  archiveWorkspaceApiV1WorkspacesWorkspaceIdArchivePost: vi.fn(),
  changePasswordApiV1AuthChangePasswordPost: vi.fn(),
  createWorkspaceApiV1WorkspacesPost: vi.fn(),
  deactivateMeApiV1MeDeactivatePost: vi.fn(),
  listWorkspacesApiV1WorkspacesGet: vi.fn().mockResolvedValue({
    data: [
      {
        id: "019c0000-0000-7000-8000-000000000002",
        name: "创作者的工作空间",
        status: "active",
        role: "owner",
        revision: 1,
      },
      {
        id: "019c0000-0000-7000-8000-000000000004",
        name: "已归档空间",
        status: "archived",
        role: "owner",
        revision: 3,
      },
    ],
  }),
  loginApiV1AuthLoginPost: vi.fn(),
  logoutApiV1AuthLogoutPost: vi.fn(),
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
  registerApiV1AuthRegisterPost: vi.fn(),
  restoreWorkspaceApiV1WorkspacesWorkspaceIdRestorePost: vi.fn(),
  updateMeApiV1MePatch: vi.fn(),
  updateWorkspaceApiV1WorkspacesWorkspaceIdPatch: vi.fn(),
}));

vi.mock("@/api/projects", () => ({}));

describe("account and workspace settings", () => {
  beforeEach(() => {
    setAccessToken("signed.jwt.token");
    replace.mockClear();
  });

  it("renders profile and active/archived workspace actions", async () => {
    render(
      <AppProviders>
        <WorkspacesPage />
      </AppProviders>,
    );

    expect(await screen.findByRole("heading", { name: "账户与工作空间" })).toBeInTheDocument();
    expect(screen.getByDisplayValue("创作者")).toBeInTheDocument();
    expect(await screen.findByText("创作者的工作空间")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "归档 创作者的工作空间" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "恢复 已归档空间" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "创建工作空间" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "修改密码" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "停用账户" })).toBeInTheDocument();
  });
});
