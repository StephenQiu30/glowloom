import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const api = vi.hoisted(() => ({
  authLogin: vi.fn(),
  authLogout: vi.fn(),
  authRefresh: vi.fn(),
  authRegister: vi.fn(),
  operationGet: vi.fn(),
  projectAnalysisGet: vi.fn(),
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
vi.mock("@/api/project", () => ({ projectAnalysisGet: api.projectAnalysisGet, projectCreate: api.projectCreate }));
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
    window.history.replaceState(null, "", "/");
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

  it("restores an approved workflow from authorized URL object references after reload", async () => {
    const projectID = "33333333-3333-4333-8333-333333333333";
    const revisionID = "44444444-4444-4444-8444-444444444444";
    const operationID = "55555555-5555-4555-8555-555555555555";
    window.history.replaceState(null, "", `/?project=${projectID}&revision=${revisionID}&operation=${operationID}`);
    api.authRefresh.mockResolvedValue({
      data: {
        access_token: "refreshed-access-token",
        workspace: { id: "11111111-1111-4111-8111-111111111111", name: "恢复工作区" },
      },
    });
    api.operationGet.mockResolvedValue({ data: { id: operationID, project_id: projectID, type: "script_analysis", status: "succeeded", progress: 100 } });
    api.projectAnalysisGet.mockResolvedValue({
      data: {
        source_hash: "source-hash",
        parse_report: { status: "complete", format: "txt", parser_version: "deterministic-script-parser-v1", original_hash: "source-hash", text_hash: "text-hash", character_count: 42, paragraph_count: 3, failed_scopes: [] },
        episodes: [{ number: 1, title: "归途", content_unit_id: "66666666-6666-4666-8666-666666666666", anchor: { line: 1, start_offset: 0, end_offset: 5 }, scenes: [] }],
        characters: [],
        locations: [],
        props: [],
        costumes: [],
      },
    });

    render(<ScriptAnalysisWorkspace />);

    expect(await screen.findByTestId("phase-status")).toHaveTextContent("事实已批准");
    expect(screen.getByText(`Project：${projectID}`)).toBeInTheDocument();
    expect(screen.getByText("第 1 集 · 归途")).toBeInTheDocument();
    expect(api.operationGet).toHaveBeenCalledWith({ operationID });
    expect(api.projectAnalysisGet).toHaveBeenCalledWith({ projectID });
    expect(window.localStorage).toHaveLength(0);
  });

  it("uploads a DOCX original and exposes its deterministic ParseReport", async () => {
    api.authRefresh.mockResolvedValue({
      data: {
        access_token: "refreshed-access-token",
        workspace: { id: "11111111-1111-4111-8111-111111111111", name: "恢复工作区" },
      },
    });
    api.projectCreate.mockResolvedValue({ data: { id: "33333333-3333-4333-8333-333333333333" } });
    api.scriptRevisionCreate.mockResolvedValue({ data: { id: "44444444-4444-4444-8444-444444444444" } });
    api.scriptAnalysisQueue.mockResolvedValue({ data: { id: "55555555-5555-4555-8555-555555555555", status: "queued", progress: 0 } });
    api.operationGet.mockResolvedValue({ data: { id: "55555555-5555-4555-8555-555555555555", status: "succeeded", progress: 100 } });
    api.scriptAnalysisDraft.mockResolvedValue({
      data: {
        source_hash: "source-hash",
        parse_report: {
          status: "complete",
          format: "docx",
          parser_version: "deterministic-script-parser-v1",
          original_hash: "source-hash",
          text_hash: "text-hash",
          character_count: 42,
          paragraph_count: 3,
          failed_scopes: [],
        },
        episodes: [],
        characters: [],
        locations: [],
        props: [],
        costumes: [],
      },
    });
    const user = userEvent.setup();
    render(<ScriptAnalysisWorkspace />);

    const file = new File(["PK fixture"], "全剧本.docx", {
      type: "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
    });
    await user.upload(await screen.findByLabelText("剧本文件"), file);
    await user.click(screen.getByTestId("analyze-button"));

    await waitFor(() => expect(api.scriptRevisionCreate).toHaveBeenCalledWith(
      { projectID: "33333333-3333-4333-8333-333333333333" },
      {},
      file,
    ));
    expect(await screen.findByText("DOCX · 3 段 · 42 字符")).toBeInTheDocument();
    expect(screen.getByText("解析完整，无失败范围")).toBeInTheDocument();
  });

  it("shows the stable API recovery action for an import failure", async () => {
    api.authRefresh.mockResolvedValue({
      data: {
        access_token: "refreshed-access-token",
        workspace: { id: "11111111-1111-4111-8111-111111111111", name: "恢复工作区" },
      },
    });
    api.projectCreate.mockRejectedValue(new ApiClientError(
      "剧本文件格式无效或内容损坏",
      "script_invalid",
      422,
      undefined,
      [],
      undefined,
      "确认文件是未加密、可正常打开的 DOCX、Markdown 或 UTF-8 TXT 后重试",
    ));
    const user = userEvent.setup();
    render(<ScriptAnalysisWorkspace />);

    await user.click(await screen.findByTestId("analyze-button"));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "剧本文件格式无效或内容损坏。下一步：确认文件是未加密、可正常打开的 DOCX、Markdown 或 UTF-8 TXT 后重试",
    );
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
