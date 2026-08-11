import { render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  me: vi.fn(),
}));

vi.mock("@/api/identity", async () => ({
  ...(await vi.importActual<typeof import("@/api/identity")>("@/api/identity")),
  meApiV1MeGet: apiMocks.me,
}));

import { AppProviders } from "@/app/providers";
import { ProtectedRoute } from "@/components/auth/protected-route";
import { StudioShell } from "@/components/studio/studio-shell";
import { setAccessToken } from "@/lib/auth-session";

const workspaceId = "019fb2e0-a000-7000-8000-000000000001";

function mockMe(role: API.WorkspaceResponse["role"]) {
  apiMocks.me.mockResolvedValue({
    data: {
      user: {
        id: "019fb2e0-a000-7000-8000-000000000002",
        email: `${role}@example.com`,
        display_name: role,
        avatar_url: null,
      },
      workspace: {
        id: workspaceId,
        name: "权限验收空间",
        status: "active",
        role,
        revision: 1,
      },
    } satisfies API.MeResponse,
  });
}

describe("role-aware global navigation", () => {
  beforeEach(() => {
    sessionStorage.clear();
    setAccessToken("test-access-token");
    vi.clearAllMocks();
  });

  it("keeps governance absent for viewers and renders a forbidden state", async () => {
    mockMe("viewer");
    const shell = render(
      <AppProviders>
        <StudioShell active="projects">
          <p>项目内容</p>
        </StudioShell>
      </AppProviders>,
    );

    const navigation = await screen.findByRole("navigation", { name: "主导航" });
    await waitFor(() => {
      expect(within(navigation).getByRole("link", { name: "首页" })).toBeInTheDocument();
      expect(within(navigation).getByRole("link", { name: "项目" })).toBeInTheDocument();
      expect(within(navigation).getByRole("link", { name: "资产" })).toBeInTheDocument();
    });
    expect(within(navigation).queryByRole("link", { name: "治理" })).not.toBeInTheDocument();
    shell.unmount();

    render(
      <AppProviders>
        <ProtectedRoute page="governance">
          <p>治理内容</p>
        </ProtectedRoute>
      </AppProviders>,
    );

    expect(await screen.findByRole("heading", { name: "无权访问此页面" })).toBeInTheDocument();
    expect(screen.queryByText("治理内容")).not.toBeInTheDocument();
  });

  it("shows the governance destination for owners", async () => {
    mockMe("owner");
    render(
      <AppProviders>
        <StudioShell active="governance">
          <p>治理内容</p>
        </StudioShell>
      </AppProviders>,
    );

    const navigation = await screen.findByRole("navigation", { name: "主导航" });
    await waitFor(() => {
      expect(within(navigation).getByRole("link", { name: "治理" })).toHaveAttribute(
        "href",
        "/governance",
      );
    });
    await waitFor(() => expect(apiMocks.me).toHaveBeenCalled());
  });
});
