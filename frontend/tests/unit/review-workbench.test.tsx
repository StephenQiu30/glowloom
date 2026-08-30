import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  claimTask: vi.fn(),
  decideTask: vi.fn(),
  getTask: vi.fn(),
  getWorkflowRun: vi.fn(),
  listTasks: vi.fn(),
  me: vi.fn(),
  project: vi.fn(),
  releaseClaim: vi.fn(),
  renewClaim: vi.fn(),
  resumeDecision: vi.fn(),
}));

vi.mock("@/api/humanReviews", () => ({
  claimHumanTaskApiHumanTasksHumanTaskIdClaimsPost: apiMocks.claimTask,
  decideHumanTaskApiHumanTasksHumanTaskIdDecisionsPost: apiMocks.decideTask,
  getHumanTaskApiHumanTasksHumanTaskIdGet: apiMocks.getTask,
  listHumanTasksApiProjectsProjectIdHumanTasksGet: apiMocks.listTasks,
  releaseHumanTaskClaimApiHumanTasksHumanTaskIdClaimReleasesPost:
    apiMocks.releaseClaim,
  renewHumanTaskClaimApiHumanTasksHumanTaskIdClaimRenewalsPost:
    apiMocks.renewClaim,
  resumeHumanGateApiReviewDecisionsReviewDecisionIdResumePost:
    apiMocks.resumeDecision,
}));

vi.mock("@/api/workflows", () => ({
  getWorkflowRunApiWorkflowRunsWorkflowRunIdGet: apiMocks.getWorkflowRun,
}));

vi.mock("@/api/identity", async () => ({
  ...(await vi.importActual<typeof import("@/api/identity")>("@/api/identity")),
  meApiMeGet: apiMocks.me,
}));

vi.mock("@/api/projects", async () => ({
  ...(await vi.importActual<typeof import("@/api/projects")>("@/api/projects")),
  getProjectApiProjectsProjectIdGet: apiMocks.project,
}));

import { AppProviders } from "@/app/providers";
import { ReviewWorkbench } from "@/app/projects/[projectId]/reviews/review-workbench";
import { setAccessToken } from "@/lib/auth-session";
import { ApiClientError } from "@/lib/request";

const workspaceId = "019ffa00-a000-7000-8000-000000000001";
const projectId = "019ffa00-a000-7000-8000-000000000002";
const taskId = "019ffa00-a000-7000-8000-000000000003";
const runId = "019ffa00-a000-7000-8000-000000000004";
const nodeId = "019ffa00-a000-7000-8000-000000000005";
const userId = "019ffa00-a000-7000-8000-000000000006";
const decisionId = "019ffa00-a000-7000-8000-000000000007";
const claimToken = "019ffa00-a000-7000-8000-000000000008";
const candidateOne = "019ffa00-a000-7000-8000-000000000009";
const candidateTwo = "019ffa00-a000-7000-8000-000000000010";

const project: API.ProjectResponse = {
  id: projectId,
  workspace_id: workspaceId,
  name: "雾港倒计时",
  description: "审核候选并恢复制作工作流",
  aspect_ratio: "9:16",
  language: "zh-CN",
  visual_style: "电影写实",
  target_duration_ms: 90_000,
  status: "active",
  revision: 1,
};

function task(
  overrides: Partial<API.HumanTaskResponse> = {},
): API.HumanTaskResponse {
  return {
    id: taskId,
    workspace_id: workspaceId,
    project_id: projectId,
    workflow_run_id: runId,
    node_run_id: nodeId,
    subject_type: "workflow_node_output",
    subject_id: nodeId,
    subject_revision: 2,
    subject_hash: "a".repeat(64),
    candidate_ids: [candidateOne],
    rubric_version: "gate.production_bible_review@1.0.0",
    allowed_decisions: ["approved", "changes_requested", "rejected"],
    status: "OPEN",
    claim: null,
    revision: 1,
    created_at: "2026-08-27T02:00:00Z",
    updated_at: "2026-08-27T02:00:00Z",
    ...overrides,
  };
}

function listItem(value: API.HumanTaskResponse): API.HumanTaskListItemResponse {
  return { ...value, claim: value.claim ? {
    claimed_by: value.claim.claimed_by,
    expires_at: value.claim.expires_at,
  } : null };
}

function decision(): API.ReviewDecisionResponse {
  return {
    id: decisionId,
    human_task_id: taskId,
    decision: "approved",
    subject_revision: 2,
    subject_hash: "a".repeat(64),
    selected_candidate_id: null,
    created_by: userId,
    created_at: "2026-08-27T02:05:00Z",
  };
}

function coordination(
  overrides: Partial<API.HumanGateCoordinationResponse> = {},
): API.HumanGateCoordinationResponse {
  return {
    review_decision_id: decisionId,
    decision_status: "recorded",
    owner_apply_status: "completed",
    owner_receipt_id: "019ffa00-a000-7000-8000-000000000011",
    workflow_resume_status: "unknown",
    workflow_signal_receipt_id: null,
    conflict_code: null,
    ...overrides,
  };
}

function workflowRun(
  nodeStatus: API.WorkflowNodeRunResponse["status"] = "SUCCEEDED",
  outputHash = "b".repeat(64),
): API.WorkflowRunViewResponse {
  return {
    run: {
      id: runId,
      workspace_id: workspaceId,
      project_id: projectId,
      authoring_revision_id: "019ffa00-a000-7000-8000-000000000012",
      definition_version_id: "019ffa00-a000-7000-8000-000000000013",
      run_input_snapshot_id: "019ffa00-a000-7000-8000-000000000014",
      temporal_workflow_id: `production-${runId}`,
      start_input_hash: "c".repeat(64),
      source_workflow_run_id: null,
      rerun_root_node_id: null,
      status: nodeStatus === "FAILED" ? "NEEDS_ATTENTION" : "SUCCEEDED",
      progress_stage: "human_gate:review",
      next_action: null,
      error: null,
      paused_from_status: null,
      paused_from_progress_stage: null,
      revision: 4,
      created_by: userId,
      created_at: "2026-08-27T02:00:00Z",
      updated_at: "2026-08-27T02:06:00Z",
    },
    nodes: [{
      id: nodeId,
      workspace_id: workspaceId,
      workflow_run_id: runId,
      node_id: "review",
      definition_key: "production.workflow",
      definition_version: "1.0.0",
      executor: "gate.production_bible_review",
      risk_level: "high",
      status: nodeStatus,
      attempt: 1,
      reused_from_node_run_id: null,
      input_hash: "a".repeat(64),
      cache_key: "review-node",
      output_hash: outputHash,
      revision: 3,
      created_at: "2026-08-27T02:00:00Z",
      updated_at: "2026-08-27T02:06:00Z",
    }],
  };
}

let currentDetail: API.HumanTaskDetailEnvelope["data"];

describe("公共审核工作台", () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
    setAccessToken("test-access-token");
    vi.clearAllMocks();
    currentDetail = { task: task(), decision: null, coordination: null };
    apiMocks.me.mockResolvedValue({
      data: {
        user: {
          id: userId,
          email: "reviewer@example.com",
          display_name: "审核者",
          avatar_url: null,
        },
        workspace: {
          id: workspaceId,
          name: "制作空间",
          status: "active",
          role: "editor",
          revision: 1,
        },
      },
    });
    apiMocks.project.mockResolvedValue({ data: project });
    apiMocks.listTasks.mockImplementation(async () => ({
      data: { items: [listItem(currentDetail.task)], next_after: null },
    }));
    apiMocks.getTask.mockImplementation(async () => ({ data: currentDetail }));
    apiMocks.getWorkflowRun.mockResolvedValue({ data: workflowRun() });
    apiMocks.resumeDecision.mockImplementation(async () => {
      const resumed = coordination({
        workflow_resume_status: "completed",
        workflow_signal_receipt_id: "019ffa00-a000-7000-8000-000000000015",
      });
      currentDetail = { ...currentDetail, coordination: resumed };
      return { data: { coordination: resumed } };
    });
  });

  it("把决议、业务应用和工作流恢复分开显示，unknown 只允许按原决议恢复", async () => {
    currentDetail = {
      task: task({ status: "COMPLETED", revision: 3 }),
      decision: decision(),
      coordination: coordination(),
    };
    const user = userEvent.setup();
    render(<AppProviders><ReviewWorkbench initialTaskId={taskId} projectId={projectId} /></AppProviders>);

    const statusRegion = await screen.findByRole("region", { name: "审核状态" });
    expect(within(statusRegion).getByText("决议已记录")).toBeInTheDocument();
    expect(within(statusRegion).getByText("业务应用完成，正在恢复工作流")).toBeInTheDocument();
    expect(within(statusRegion).getByText("结果未知，可安全恢复")).toBeInTheDocument();
    expect(screen.queryByText("工作流已继续")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "按原决议恢复工作流" }));
    expect(apiMocks.resumeDecision).toHaveBeenCalledWith({
      review_decision_id: decisionId,
    });
  });

  it("从详情恢复 Claim Token，并只提交冻结候选与服务端 revision", async () => {
    currentDetail = {
      task: task({
        subject_type: "generation_candidate_selection",
        candidate_ids: [candidateOne, candidateTwo],
        allowed_decisions: ["changes_requested", "rejected", "selected"],
      }),
      decision: null,
      coordination: null,
    };
    apiMocks.claimTask.mockImplementation(async () => {
      const claimed = task({
        subject_type: "generation_candidate_selection",
        candidate_ids: [candidateOne, candidateTwo],
        allowed_decisions: ["changes_requested", "rejected", "selected"],
        status: "CLAIMED",
        revision: 2,
        claim: {
          claimed_by: userId,
          expires_at: "2099-08-27T02:10:00Z",
          claim_token: claimToken,
        },
      });
      currentDetail = { task: claimed, decision: null, coordination: null };
      return { data: { task: claimed } };
    });
    apiMocks.decideTask.mockImplementation(async (...args) => {
      const body = args[1] as API.HumanTaskDecisionRequest;
      const recorded = {
        ...decision(),
        decision: "selected" as const,
        selected_candidate_id: body.selected_candidate_id,
      };
      const state = coordination({ owner_apply_status: "pending" });
      currentDetail = {
        task: task({ ...currentDetail.task, status: "COMPLETED", revision: 3, claim: null }),
        decision: recorded,
        coordination: state,
      };
      return { data: { ...currentDetail, decision: recorded, coordination: state } };
    });
    const user = userEvent.setup();
    render(<AppProviders><ReviewWorkbench projectId={projectId} /></AppProviders>);

    const claimButton = await screen.findByRole("button", { name: "领取审核" });
    claimButton.focus();
    await user.keyboard("{Enter}");
    expect(apiMocks.claimTask).toHaveBeenCalledWith(
      { human_task_id: taskId },
      expect.objectContaining({
        expected_revision: 1,
        idempotency_key: expect.stringMatching(/^human-task-claim:/),
      }),
    );

    const candidate = await screen.findByRole("radio", { name: candidateTwo });
    await user.click(candidate);
    await user.click(screen.getByRole("button", { name: "确认选择" }));
    expect(apiMocks.decideTask).toHaveBeenCalledWith(
      { human_task_id: taskId },
      {
        claim_token: claimToken,
        expected_task_revision: 2,
        expected_subject_revision: 2,
        expected_subject_hash: "a".repeat(64),
        decision: "selected",
        selected_candidate_id: candidateTwo,
        idempotency_key: expect.stringMatching(/^human-task-decision:/),
      },
    );
    expect(window.location.href).not.toContain(claimToken);
    expect(JSON.stringify({
      local: { ...localStorage },
      session: { ...sessionStorage },
    })).not.toContain(claimToken);
  });

  it("刷新后从详情恢复租约，并用同一 Token 续期或释放", async () => {
    const claimed = task({
      status: "CLAIMED",
      revision: 2,
      claim: {
        claimed_by: userId,
        expires_at: "2099-08-27T02:10:00Z",
        claim_token: claimToken,
      },
    });
    currentDetail = { task: claimed, decision: null, coordination: null };
    apiMocks.renewClaim.mockImplementation(async () => {
      const renewed = task({ ...claimed, revision: 3 });
      currentDetail = { task: renewed, decision: null, coordination: null };
      return { data: { task: renewed } };
    });
    apiMocks.releaseClaim.mockImplementation(async () => {
      const released = task({ status: "OPEN", revision: 4, claim: null });
      currentDetail = { task: released, decision: null, coordination: null };
      return { data: { task: released } };
    });
    const user = userEvent.setup();
    render(<AppProviders><ReviewWorkbench initialTaskId={taskId} projectId={projectId} /></AppProviders>);

    await user.click(await screen.findByRole("button", { name: "续期租约" }));
    expect(apiMocks.renewClaim).toHaveBeenCalledWith(
      { human_task_id: taskId },
      expect.objectContaining({
        claim_token: claimToken,
        expected_revision: 2,
        idempotency_key: expect.stringMatching(/^human-task-renew:/),
      }),
    );
    expect(await screen.findByText("审核租约已续期。")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "释放审核" }));
    expect(apiMocks.releaseClaim).toHaveBeenCalledWith(
      { human_task_id: taskId },
      expect.objectContaining({
        claim_token: claimToken,
        expected_revision: 3,
        idempotency_key: expect.stringMatching(/^human-task-release:/),
      }),
    );
  });

  it("详情没有 Token 时只尝试由服务端判定过期接管", async () => {
    currentDetail = {
      task: task({
        status: "CLAIMED",
        revision: 2,
        claim: {
          claimed_by: "019ffa00-a000-7000-8000-000000000099",
          expires_at: "2020-08-27T02:10:00Z",
        },
      }),
      decision: null,
      coordination: null,
    };
    apiMocks.claimTask.mockResolvedValue({ data: { task: currentDetail.task } });
    const user = userEvent.setup();
    render(<AppProviders><ReviewWorkbench initialTaskId={taskId} projectId={projectId} /></AppProviders>);

    expect(await screen.findByLabelText("审核租约")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "尝试接管审核" }));
    expect(apiMocks.claimTask).toHaveBeenCalledWith(
      { human_task_id: taskId },
      expect.objectContaining({ expected_revision: 2 }),
    );
  });

  it("命令返回冲突时重取已记录决议，不把错误伪装成本地成功", async () => {
    currentDetail = {
      task: task({
        status: "CLAIMED",
        revision: 2,
        claim: {
          claimed_by: userId,
          expires_at: "2099-08-27T02:10:00Z",
          claim_token: claimToken,
        },
      }),
      decision: null,
      coordination: null,
    };
    apiMocks.decideTask.mockImplementation(async () => {
      currentDetail = {
        task: task({ status: "COMPLETED", revision: 3, claim: null }),
        decision: decision(),
        coordination: coordination({
          owner_apply_status: "conflict",
          owner_receipt_id: null,
          workflow_resume_status: "pending",
          conflict_code: "owner_baseline_changed",
        }),
      };
      throw new ApiClientError(
        "Owner baseline changed",
        "resource_conflict",
        undefined,
        currentDetail.coordination,
      );
    });
    const user = userEvent.setup();
    render(<AppProviders><ReviewWorkbench initialTaskId={taskId} projectId={projectId} /></AppProviders>);

    await user.click(await screen.findByRole("button", { name: "接受" }));
    expect(await screen.findByText("决议已记录")).toBeInTheDocument();
    expect(screen.getByText("业务应用冲突")).toBeInTheDocument();
    expect(screen.queryByText("工作流已继续")).not.toBeInTheDocument();
    expect(screen.getByText("请求所依据的服务端版本已经变化，请刷新后重试。")).toBeInTheDocument();
    expect(apiMocks.getTask.mock.calls.length).toBeGreaterThan(1);
  });

  it("未知 Subject 和 Viewer 都保持只读，不猜测可执行动作", async () => {
    apiMocks.me.mockResolvedValue({
      data: {
        user: {
          id: userId,
          email: "viewer@example.com",
          display_name: "查看者",
          avatar_url: null,
        },
        workspace: {
          id: workspaceId,
          name: "制作空间",
          status: "active",
          role: "viewer",
          revision: 1,
        },
      },
    });
    currentDetail = {
      task: task({ subject_type: "future_storygraph_subject" }),
      decision: null,
      coordination: null,
    };
    render(<AppProviders><ReviewWorkbench initialTaskId={taskId} projectId={projectId} /></AppProviders>);

    expect(await screen.findByText("当前 Subject 类型仅支持只读查看")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "领取审核" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "接受" })).not.toBeInTheDocument();
  });

  it("仅在 Resume 完成且 NodeRun 输出已重取后显示工作流已继续", async () => {
    currentDetail = {
      task: task({ status: "COMPLETED", revision: 3 }),
      decision: decision(),
      coordination: coordination({
        workflow_resume_status: "completed",
        workflow_signal_receipt_id: "019ffa00-a000-7000-8000-000000000015",
      }),
    };
    render(<AppProviders><ReviewWorkbench initialTaskId={taskId} projectId={projectId} /></AppProviders>);

    expect(await screen.findAllByText("工作流已继续")).toHaveLength(2);
    expect(apiMocks.getWorkflowRun).toHaveBeenCalledWith({ workflow_run_id: runId });
  });
});
