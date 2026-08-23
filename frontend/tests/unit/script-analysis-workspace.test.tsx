import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  authLogin: vi.fn(),
  authLogout: vi.fn(),
  authRefresh: vi.fn(),
  authRegister: vi.fn(),
  operationGet: vi.fn(),
  projectCreate: vi.fn(),
  scriptAnalysisApprove: vi.fn(),
  scriptAnalysisDraft: vi.fn(),
  scriptAnalysisQueue: vi.fn(),
  scriptRevisionCreate: vi.fn(),
}));

vi.mock("@/api/auth", () => ({
  authLogin: api.authLogin,
  authLogout: api.authLogout,
  authRefresh: api.authRefresh,
  authRegister: api.authRegister,
}));
vi.mock("@/api/operation", () => ({ operationGet: api.operationGet }));
vi.mock("@/api/project", () => ({ projectCreate: api.projectCreate }));
vi.mock("@/api/script", () => ({
  scriptAnalysisApprove: api.scriptAnalysisApprove,
  scriptAnalysisDraft: api.scriptAnalysisDraft,
  scriptAnalysisQueue: api.scriptAnalysisQueue,
  scriptRevisionCreate: api.scriptRevisionCreate,
}));

import { ScriptAnalysisWorkspace } from "@/features/script-analysis/views/script-analysis-workspace";
import { ApiClientError } from "@/lib/request";

describe("ScriptAnalysisWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.localStorage.clear();
    api.authRefresh.mockRejectedValue(new ApiClientError("刷新会话缺失", "unauthorized", 401));
    api.authLogout.mockResolvedValue(undefined);
  });

  it("requires the current email authentication contract before showing the fact line", async () => {
    render(<ScriptAnalysisWorkspace />);

    expect(await screen.findByRole("heading", { name: "登录后开始剧本事实分析" })).toBeInTheDocument();
    expect(screen.getByLabelText("邮箱"), "email field").toBeInTheDocument();
    expect(screen.getByLabelText("密码"), "password field").toBeInTheDocument();
    expect(screen.queryByTestId("analyze-button")).not.toBeInTheDocument();
  });

  it("registers through /api/auth/register and enters the script workflow", async () => {
    api.authRegister.mockResolvedValue({
      data: {
        access_token: "test-access-token",
        workspace: { id: "11111111-1111-4111-8111-111111111111", name: "试点工作区" },
        user: { id: "22222222-2222-4222-8222-222222222222", email: "director@example.test", display_name: "试点导演" },
        role: "admin",
        token_type: "Bearer",
        expires_at: "2026-08-24T00:00:00Z",
      },
    });
    const user = userEvent.setup();
    render(<ScriptAnalysisWorkspace />);

    await user.type(await screen.findByLabelText("邮箱"), "director@example.test");
    await user.type(screen.getByLabelText("密码"), "Lanverse-Test-Password-2026!");
    await user.type(screen.getByLabelText("显示名称"), "试点导演");
    await user.type(screen.getByLabelText("工作区名称"), "试点工作区");
    await user.click(screen.getByRole("button", { name: "注册并进入" }));

    await waitFor(() => expect(api.authRegister).toHaveBeenCalledWith({
      email: "director@example.test",
      password: "Lanverse-Test-Password-2026!",
      display_name: "试点导演",
      workspace_name: "试点工作区",
    }));
    expect(await screen.findByRole("heading", { name: "先把整本剧本，变成可核对的事实。" })).toBeInTheDocument();
    expect(screen.getByTestId("analyze-button")).toHaveTextContent("提交解析任务");
    expect(window.localStorage.getItem("lanverse.session_token")).toBeNull();
    expect(window.localStorage.getItem("lanverse.workspace_id")).toBeNull();
  });

  it("restores the in-memory access token through the HttpOnly refresh session", async () => {
    api.authRefresh.mockResolvedValue({
      data: {
        access_token: "refreshed-access-token",
        workspace: { id: "11111111-1111-4111-8111-111111111111", name: "恢复工作区" },
        user: { id: "22222222-2222-4222-8222-222222222222", email: "director@example.test", display_name: "试点导演" },
        role: "admin",
        token_type: "Bearer",
        expires_at: "2026-08-24T00:00:00Z",
      },
    });

    render(<ScriptAnalysisWorkspace />);

    expect(await screen.findByRole("heading", { name: "先把整本剧本，变成可核对的事实。" })).toBeInTheDocument();
    expect(api.authRefresh).toHaveBeenCalledOnce();
    expect(screen.getByText("恢复工作区")).toBeInTheDocument();
    expect(window.localStorage).toHaveLength(0);
  });

  it("returns to the identity gate when an authenticated request reports an expired session", async () => {
    api.authRegister.mockResolvedValue({
      data: {
        access_token: "expired-access-token",
        workspace: { id: "11111111-1111-4111-8111-111111111111", name: "试点工作区" },
        user: { id: "22222222-2222-4222-8222-222222222222", email: "director@example.test", display_name: "试点导演" },
        role: "admin",
        token_type: "Bearer",
        expires_at: "2026-08-24T00:00:00Z",
      },
    });
    api.projectCreate.mockRejectedValue(new ApiClientError("访问令牌无效", "unauthorized", 401));
    const user = userEvent.setup();
    render(<ScriptAnalysisWorkspace />);

    await user.type(await screen.findByLabelText("邮箱"), "director@example.test");
    await user.type(screen.getByLabelText("密码"), "Lanverse-Test-Password-2026!");
    await user.type(screen.getByLabelText("显示名称"), "试点导演");
    await user.type(screen.getByLabelText("工作区名称"), "试点工作区");
    await user.click(screen.getByRole("button", { name: "注册并进入" }));
    await user.click(await screen.findByTestId("analyze-button"));

    expect(await screen.findByRole("heading", { name: "登录后开始剧本事实分析" })).toBeInTheDocument();
    expect(screen.getByRole("alert")).toHaveTextContent("登录会话已失效，请重新登录");
    expect(window.localStorage.getItem("lanverse.session_token")).toBeNull();
    expect(window.localStorage.getItem("lanverse.workspace_id")).toBeNull();
  });
});
