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
  projectList: vi.fn(),
  scriptAnalysisDraft: vi.fn(),
  scriptAnalysisDraftRevise: vi.fn(),
  scriptAnalysisQueue: vi.fn(),
  scriptEpisodeBreakdownApprove: vi.fn(),
  scriptNarrativeApprove: vi.fn(),
  scriptNarrativeDraftRevise: vi.fn(),
  scriptRevisionCreate: vi.fn(),
}));

vi.mock("@/api/auth", () => ({
  authLogin: api.authLogin,
  authLogout: api.authLogout,
  authRefresh: api.authRefresh,
  authRegister: api.authRegister,
}));
vi.mock("@/api/operation", () => ({ operationGet: api.operationGet }));
vi.mock("@/api/project", () => ({
  projectAnalysisGet: api.projectAnalysisGet,
  projectCreate: api.projectCreate,
  projectList: api.projectList,
}));
vi.mock("@/api/script", () => ({
  scriptAnalysisDraft: api.scriptAnalysisDraft,
  scriptAnalysisDraftRevise: api.scriptAnalysisDraftRevise,
  scriptAnalysisQueue: api.scriptAnalysisQueue,
  scriptEpisodeBreakdownApprove: api.scriptEpisodeBreakdownApprove,
  scriptNarrativeApprove: api.scriptNarrativeApprove,
  scriptNarrativeDraftRevise: api.scriptNarrativeDraftRevise,
  scriptRevisionCreate: api.scriptRevisionCreate,
}));

import { ScriptAnalysisWorkspace } from "@/features/script-analysis/views/script-analysis-workspace";
import { ApiClientError } from "@/lib/request";

const workflowIDs = {
  workspaceID: "11111111-1111-4111-8111-111111111111",
  projectID: "33333333-3333-4333-8333-333333333333",
  revisionID: "44444444-4444-4444-8444-444444444444",
  operationID: "55555555-5555-4555-8555-555555555555",
};

function breakdownAnalysis(status: "ready" | "blocked" = "ready"): API.Analysis {
  return {
    source_hash: "source-hash",
    parse_report: { status: "complete", format: "txt", parser_version: "deterministic-script-parser-v1", original_hash: "source-hash", text_hash: "text-hash", character_count: 300, paragraph_count: 9, failed_scopes: [] },
    breakdown: {
      revision_no: 2,
      status,
      coverage_hash: "coverage-hash",
      segmentation_hash: "segmentation-hash",
      issues: status === "blocked" ? [{ code: "duplicate_episode_number", message: "集号 1 重复，需要人工拆解或重排", candidate_keys: ["episode-a", "episode-b"], anchor: null }] : [],
    },
    episodes: [
      {
        temporary_key: "episode-a", ordinal: 1, number: 1, title: "归途", decision: "pending", boundary_rule: "explicit_episode_heading_v1", content_unit_id: null,
        anchor: { line: 1, start_offset: 0, end_offset: 100 },
        scenes: [
          { id: "scene-a1", heading: "码头", anchor: { line: 2, start_offset: 0, end_offset: 50 }, narratives: [] },
          { id: "scene-a2", heading: "仓库", anchor: { line: 3, start_offset: 50, end_offset: 100 }, narratives: [] },
        ],
      },
      {
        temporary_key: "episode-b", ordinal: 2, number: 2, title: "回声", decision: "pending", boundary_rule: "explicit_episode_heading_v1", content_unit_id: null,
        anchor: { line: 4, start_offset: 100, end_offset: 200 },
        scenes: [
          { id: "scene-b1", heading: "车站", anchor: { line: 5, start_offset: 100, end_offset: 150 }, narratives: [] },
          { id: "scene-b2", heading: "长街", anchor: { line: 6, start_offset: 150, end_offset: 200 }, narratives: [] },
        ],
      },
      {
        temporary_key: "episode-c", ordinal: 3, number: 3, title: "终局", decision: "pending", boundary_rule: "explicit_episode_heading_v1", content_unit_id: null,
        anchor: { line: 7, start_offset: 200, end_offset: 300 },
        scenes: [{ id: "scene-c1", heading: "山顶", anchor: { line: 8, start_offset: 200, end_offset: 300 }, narratives: [] }],
      },
    ],
    narrative: null,
    mentions: [],
    characters: [], locations: [], props: [], costumes: [],
  };
}

function narrativeAnalysis(status: "ready" | "blocked" | "approved" = "ready"): API.Analysis {
  const analysis = breakdownAnalysis();
  return {
    ...analysis,
    narrative: {
      id: "66666666-6666-4666-8666-666666666666",
      revision_no: 3,
      status,
      content_hash: "narrative-hash",
      completeness: status === "blocked" ? "partial" : "complete",
      issues: status === "blocked" ? [{ code: "unknown_speaker", message: "对白缺少明确说话人", scene_id: "77777777-7777-4777-8777-777777777777", node_id: "88888888-8888-4888-8888-888888888888", mention_id: null, anchor: null }] : [],
    },
    episodes: [{
      temporary_key: "episode-a", ordinal: 1, number: 1, title: "归途", decision: "accepted", boundary_rule: "explicit_episode_heading_v1",
      content_unit_id: "99999999-9999-4999-8999-999999999999",
      anchor: { line: 1, start_offset: 0, end_offset: 100 },
      scenes: [{
        id: "77777777-7777-4777-8777-777777777777", heading: "码头", anchor: { line: 2, start_offset: 10, end_offset: 100 },
        narratives: [
          { id: "88888888-8888-4888-8888-888888888888", kind: "dialogue", text: "林夏：我们走。", speaker: status === "blocked" ? "" : "林夏", status: "active", ignore_reason: null, anchor: { line: 3, start_offset: 20, end_offset: 35 } },
          { id: "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", kind: "action", text: "海风渐强。", speaker: "", status: "active", ignore_reason: null, anchor: { line: 4, start_offset: 36, end_offset: 50 } },
        ],
      }],
    }],
    mentions: [{
      id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", scene_id: "77777777-7777-4777-8777-777777777777",
      element_type: "character", surface_text: "林夏", status: "active", anchor: { line: 3, start_offset: 20, end_offset: 22 },
    }],
  };
}

function restoreDraft(analysis: API.Analysis = breakdownAnalysis()) {
  const { workspaceID, projectID, revisionID, operationID } = workflowIDs;
  window.history.replaceState(null, "", `/?project=${projectID}&revision=${revisionID}&operation=${operationID}`);
  api.authRefresh.mockResolvedValue({ data: { access_token: "refreshed-access-token", workspace: { id: workspaceID, name: "恢复工作区" } } });
  api.operationGet.mockResolvedValue({ data: { id: operationID, project_id: projectID, source_revision_id: revisionID, type: "script_analysis", status: "waiting_user", progress: 35 } });
  api.projectAnalysisGet.mockRejectedValue(new ApiClientError("正式分析不存在", "not_found", 404));
  api.scriptAnalysisDraft.mockResolvedValue({ data: analysis });
  api.scriptAnalysisDraftRevise.mockResolvedValue({ data: analysis });
  api.scriptEpisodeBreakdownApprove.mockResolvedValue({ data: narrativeAnalysis() });
  api.scriptNarrativeDraftRevise.mockResolvedValue({ data: analysis });
  api.scriptNarrativeApprove.mockResolvedValue({ data: narrativeAnalysis("approved") });
}

describe("ScriptAnalysisWorkspace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    window.localStorage.clear();
    window.history.replaceState(null, "", "/");
    api.authRefresh.mockRejectedValue(new ApiClientError("刷新会话缺失", "unauthorized", 401));
    api.authLogout.mockResolvedValue(undefined);
    api.projectList.mockResolvedValue({ data: { items: [], page: 1, page_size: 20, total: 0 } });
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
    api.operationGet.mockResolvedValue({ data: { id: operationID, project_id: projectID, source_revision_id: revisionID, type: "script_analysis", status: "succeeded", progress: 100 } });
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

    await waitFor(() => expect(screen.getByTestId("phase-status")).toHaveTextContent("叙事已批准 · 知识待决议"));
    expect(screen.getByText(`Project：${projectID}`)).toBeInTheDocument();
    expect(screen.getByText("第 1 集 · 归途")).toBeInTheDocument();
    expect(api.operationGet).toHaveBeenCalledWith({ operationID });
    expect(api.projectAnalysisGet).toHaveBeenCalledWith({ projectID });
    expect(screen.getByText("叙事已批准，ProductionElementMention 已冻结；实体与生产需求仍待知识决议。")).toBeInTheDocument();
    expect(window.localStorage).toHaveLength(0);
  });

  it("shows breakdown blockers and prevents approval until source coverage is valid", async () => {
    restoreDraft(breakdownAnalysis("blocked"));
    render(<ScriptAnalysisWorkspace />);

    expect(await screen.findByRole("heading", { name: "3. 校对剧集边界" })).toBeInTheDocument();
    expect(screen.getByText("集号 1 重复，需要人工拆解或重排")).toBeInTheDocument();
    expect(screen.getByTestId("approve-button")).toBeDisabled();
  });

  it("revises an episode title against the current source hash", async () => {
    restoreDraft();
    const user = userEvent.setup();
    render(<ScriptAnalysisWorkspace />);

    const title = await screen.findByLabelText("范围 1 标题");
    await user.clear(title);
    await user.type(title, "归途·人工修订");
    await user.click(screen.getByRole("button", { name: "保存范围 1 标题" }));

    await waitFor(() => expect(api.scriptAnalysisDraftRevise).toHaveBeenCalledWith(
      { revisionID: workflowIDs.revisionID },
      {
        expected_source_hash: "source-hash",
        operations: [expect.objectContaining({ type: "rename", candidate_key: "episode-a", title: "归途·人工修订" })],
      },
    ));
  });

  it("splits only at a complete scene boundary", async () => {
    restoreDraft();
    const user = userEvent.setup();
    render(<ScriptAnalysisWorkspace />);

    await user.selectOptions(await screen.findByLabelText("范围 1 拆分边界"), "50");
    await user.clear(screen.getByLabelText("范围 1 拆分后左侧标题"));
    await user.type(screen.getByLabelText("范围 1 拆分后左侧标题"), "归途上");
    await user.clear(screen.getByLabelText("范围 1 拆分后右侧标题"));
    await user.type(screen.getByLabelText("范围 1 拆分后右侧标题"), "归途下");
    await user.click(screen.getByRole("button", { name: "拆分范围 1" }));

    await waitFor(() => expect(api.scriptAnalysisDraftRevise).toHaveBeenCalledWith(
      { revisionID: workflowIDs.revisionID },
      {
        expected_source_hash: "source-hash",
        operations: [expect.objectContaining({
          type: "split", candidate_key: "episode-a", boundary_offset: 50,
          left_title: "归途上", right_title: "归途下",
        })],
      },
    ));
  });

  it("supports merge, boundary move, reorder, and named ignore commands", async () => {
    const analysis = breakdownAnalysis();
    restoreDraft(analysis);
    const user = userEvent.setup();
    render(<ScriptAnalysisWorkspace />);

    await user.click(await screen.findByRole("button", { name: "合并范围 1 与 2" }));
    await waitFor(() => expect(api.scriptAnalysisDraftRevise).toHaveBeenLastCalledWith(
      { revisionID: workflowIDs.revisionID },
      { expected_source_hash: "source-hash", operations: [expect.objectContaining({ type: "merge", candidate_keys: ["episode-a", "episode-b"] })] },
    ));

    await user.selectOptions(screen.getByLabelText("范围 1 与 2 边界"), "150");
    await user.click(screen.getByRole("button", { name: "移动范围 1 与 2 边界" }));
    await waitFor(() => expect(api.scriptAnalysisDraftRevise).toHaveBeenLastCalledWith(
      { revisionID: workflowIDs.revisionID },
      { expected_source_hash: "source-hash", operations: [expect.objectContaining({ type: "move_boundary", left_key: "episode-a", right_key: "episode-b", boundary_offset: 150 })] },
    ));

    await user.click(screen.getByRole("button", { name: "下移范围 1" }));
    await waitFor(() => expect(api.scriptAnalysisDraftRevise).toHaveBeenLastCalledWith(
      { revisionID: workflowIDs.revisionID },
      { expected_source_hash: "source-hash", operations: [expect.objectContaining({ type: "reorder", ordered_candidate_keys: ["episode-b", "episode-a", "episode-c"] })] },
    ));

    await user.clear(screen.getByLabelText("范围 1 忽略理由"));
    await user.type(screen.getByLabelText("范围 1 忽略理由"), "片头说明，不属于正片");
    await user.click(screen.getByRole("button", { name: "具名忽略范围 1" }));
    await waitFor(() => expect(api.scriptAnalysisDraftRevise).toHaveBeenLastCalledWith(
      { revisionID: workflowIDs.revisionID },
      { expected_source_hash: "source-hash", operations: [expect.objectContaining({ type: "ignore", candidate_key: "episode-a", title: "片头说明，不属于正片" })] },
    ));
  });

  it("approves only the episode breakdown before opening narrative review", async () => {
    restoreDraft();
    const user = userEvent.setup();
    render(<ScriptAnalysisWorkspace />);

    await user.click(await screen.findByRole("button", { name: "批准剧集拆解并创建叙事草稿" }));

    await waitFor(() => expect(api.scriptEpisodeBreakdownApprove).toHaveBeenCalledWith({ revisionID: workflowIDs.revisionID }));
    expect(await screen.findByRole("heading", { name: "4. 校对叙事与 Mention" })).toBeInTheDocument();
    expect(screen.getByTestId("phase-status")).toHaveTextContent("叙事结构待校对");
  });

  it("revises a scene and typed narrative node against the current narrative hash", async () => {
    const analysis = narrativeAnalysis();
    restoreDraft(analysis);
    api.scriptNarrativeDraftRevise.mockResolvedValue({ data: analysis });
    const user = userEvent.setup();
    render(<ScriptAnalysisWorkspace />);

    const sceneHeading = await screen.findByLabelText("第 1 集场景 1 标题");
    await user.clear(sceneHeading);
    await user.type(sceneHeading, "海边码头");
    await user.click(screen.getByRole("button", { name: "保存第 1 集场景 1 标题" }));
    await waitFor(() => expect(api.scriptNarrativeDraftRevise).toHaveBeenLastCalledWith(
      { revisionID: workflowIDs.revisionID },
      { expected_narrative_hash: "narrative-hash", operations: [expect.objectContaining({ type: "update_scene", scene_id: "77777777-7777-4777-8777-777777777777", heading: "海边码头" })] },
    ));

    await user.selectOptions(screen.getByLabelText("第 1 集场景 1节点 1 类型"), "narration");
    await user.clear(screen.getByLabelText("第 1 集场景 1节点 1 正文"));
    await user.type(screen.getByLabelText("第 1 集场景 1节点 1 正文"), "远处传来汽笛声。");
    await user.click(screen.getByRole("button", { name: "保存第 1 集场景 1节点 1" }));
    await waitFor(() => expect(api.scriptNarrativeDraftRevise).toHaveBeenLastCalledWith(
      { revisionID: workflowIDs.revisionID },
      { expected_narrative_hash: "narrative-hash", operations: [expect.objectContaining({ type: "update_node", node_id: "88888888-8888-4888-8888-888888888888", node_kind: "narration", text: "远处传来汽笛声。" })] },
    ));
  });

  it("creates and deletes an anchored Mention without publishing an M04 entity", async () => {
    const analysis = narrativeAnalysis();
    restoreDraft(analysis);
    api.scriptNarrativeDraftRevise.mockResolvedValue({ data: analysis });
    const user = userEvent.setup();
    render(<ScriptAnalysisWorkspace />);

    await user.selectOptions(await screen.findByLabelText("场景 1 新 Mention 类型"), "prop");
    await user.type(screen.getByLabelText("场景 1 新 Mention 来源文本"), "旧怀表");
    await user.clear(screen.getByLabelText("场景 1 新 Mention 起始 Offset"));
    await user.type(screen.getByLabelText("场景 1 新 Mention 起始 Offset"), "36");
    await user.clear(screen.getByLabelText("场景 1 新 Mention 结束 Offset"));
    await user.type(screen.getByLabelText("场景 1 新 Mention 结束 Offset"), "40");
    await user.click(screen.getByRole("button", { name: "在场景 1 创建 Mention" }));
    await waitFor(() => expect(api.scriptNarrativeDraftRevise).toHaveBeenLastCalledWith(
      { revisionID: workflowIDs.revisionID },
      { expected_narrative_hash: "narrative-hash", operations: [expect.objectContaining({ type: "create_mention", scene_id: "77777777-7777-4777-8777-777777777777", element_type: "prop", surface_text: "旧怀表", anchor: expect.objectContaining({ start_offset: 36, end_offset: 40 }) })] },
    ));

    await user.click(screen.getByRole("button", { name: "删除 Mention 林夏" }));
    await waitFor(() => expect(api.scriptNarrativeDraftRevise).toHaveBeenLastCalledWith(
      { revisionID: workflowIDs.revisionID },
      { expected_narrative_hash: "narrative-hash", operations: [expect.objectContaining({ type: "delete_mention", mention_id: "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb" })] },
    ));
  });

  it("blocks narrative approval on named validation issues and approves a clean revision only", async () => {
    restoreDraft(narrativeAnalysis("blocked"));
    const user = userEvent.setup();
    const { unmount } = render(<ScriptAnalysisWorkspace />);

    expect(await screen.findByText("对白缺少明确说话人")).toBeInTheDocument();
    expect(screen.getByTestId("approve-narrative-button")).toBeDisabled();

    unmount();
    restoreDraft(narrativeAnalysis());
    render(<ScriptAnalysisWorkspace />);
    await user.click(await screen.findByTestId("approve-narrative-button"));
    await waitFor(() => expect(api.scriptNarrativeApprove).toHaveBeenCalledWith(
      { revisionID: workflowIDs.revisionID },
      { expected_narrative_hash: "narrative-hash" },
    ));
    expect(await screen.findByTestId("phase-status")).toHaveTextContent("叙事已批准 · 知识待决议");
  });

  it("lists authorized projects and resumes a workflow without a pre-existing URL", async () => {
    const workspaceID = "11111111-1111-4111-8111-111111111111";
    const projectID = "33333333-3333-4333-8333-333333333333";
    const revisionID = "44444444-4444-4444-8444-444444444444";
    const operationID = "55555555-5555-4555-8555-555555555555";
    api.authRefresh.mockResolvedValue({
      data: {
        access_token: "refreshed-access-token",
        workspace: { id: workspaceID, name: "恢复工作区" },
      },
    });
    api.projectList.mockResolvedValue({
      data: {
        items: [{
          id: projectID,
          workspace_id: workspaceID,
          name: "跨设备项目",
          created_at: "2026-08-23T10:00:00Z",
          latest_workflow: {
            project_id: projectID,
            source_revision_id: revisionID,
            source_status: "approved",
            operation_id: operationID,
            operation_status: "succeeded",
            progress: 100,
          },
        }],
        page: 1,
        page_size: 20,
        total: 1,
      },
    });
    api.operationGet.mockResolvedValue({
      data: { id: operationID, project_id: projectID, source_revision_id: revisionID, type: "script_analysis", status: "succeeded", progress: 100 },
    });
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
    const user = userEvent.setup();
    render(<ScriptAnalysisWorkspace />);

    expect(await screen.findByRole("heading", { name: "继续已有项目" })).toBeInTheDocument();
    expect(screen.getByText("共 1 个项目 · 第 1 页")).toBeInTheDocument();
    expect(screen.getByRole("navigation", { name: "项目列表分页" })).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "继续解析 跨设备项目" }));

    await waitFor(() => expect(screen.getByTestId("phase-status")).toHaveTextContent("叙事已批准 · 知识待决议"));
    expect(api.projectList).toHaveBeenCalledWith({ workspaceID, page: 1, page_size: 20 });
    expect(api.operationGet).toHaveBeenCalledWith({ operationID });
    expect(screen.getByLabelText("项目名称")).toHaveValue("跨设备项目");
    expect(window.location.search).toBe(`?project=${projectID}&revision=${revisionID}&operation=${operationID}`);
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
