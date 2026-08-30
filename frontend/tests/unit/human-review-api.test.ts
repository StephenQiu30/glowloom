import { beforeEach, describe, expect, it, vi } from "vitest";

const requestMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/request", () => ({ default: requestMock }));

import {
  claimHumanTaskApiHumanTasksHumanTaskIdClaimsPost,
  decideHumanTaskApiHumanTasksHumanTaskIdDecisionsPost,
  getHumanTaskApiHumanTasksHumanTaskIdGet,
  listHumanTasksApiProjectsProjectIdHumanTasksGet,
  releaseHumanTaskClaimApiHumanTasksHumanTaskIdClaimReleasesPost,
  renewHumanTaskClaimApiHumanTasksHumanTaskIdClaimRenewalsPost,
  resumeHumanGateApiReviewDecisionsReviewDecisionIdResumePost,
} from "@/api/humanReviews";
import { getWorkflowRunApiWorkflowRunsWorkflowRunIdGet } from "@/api/workflows";

const projectId = "019ffb00-a000-7000-8000-000000000001";
const taskId = "019ffb00-a000-7000-8000-000000000002";
const decisionId = "019ffb00-a000-7000-8000-000000000003";
const runId = "019ffb00-a000-7000-8000-000000000004";
const claimToken = "019ffb00-a000-7000-8000-000000000005";

describe("公共人工审核 API Client", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    requestMock.mockResolvedValue({ data: {} });
  });

  it("使用稳定列表/详情路由且列表查询不携带 Claim Token", async () => {
    await listHumanTasksApiProjectsProjectIdHumanTasksGet({
      project_id: projectId,
      status: "active",
      subject_type: null,
      limit: 50,
      after: null,
    });
    await getHumanTaskApiHumanTasksHumanTaskIdGet({ human_task_id: taskId });

    expect(requestMock).toHaveBeenNthCalledWith(
      1,
      `/api/projects/${projectId}/human-tasks`,
      {
        method: "GET",
        params: { status: "active", limit: 50 },
      },
    );
    expect(requestMock).toHaveBeenNthCalledWith(
      2,
      `/api/human-tasks/${taskId}`,
      { method: "GET" },
    );
    expect(JSON.stringify(requestMock.mock.calls)).not.toContain(claimToken);
  });

  it("Claim Token 只进入命令 Body，Resume 只使用已持久化 Decision ID", async () => {
    const claimBody: API.HumanTaskClaimRequest = {
      expected_revision: 1,
      idempotency_key: "human-task-claim:one",
    };
    const tokenBody: API.HumanTaskClaimTokenRequest = {
      claim_token: claimToken,
      expected_revision: 2,
      idempotency_key: "human-task-claim:two",
    };
    const decisionBody: API.HumanTaskDecisionRequest = {
      claim_token: claimToken,
      expected_task_revision: 2,
      expected_subject_revision: 1,
      expected_subject_hash: "a".repeat(64),
      decision: "approved",
      selected_candidate_id: null,
      idempotency_key: "human-task-decision:one",
    };

    await claimHumanTaskApiHumanTasksHumanTaskIdClaimsPost(
      { human_task_id: taskId },
      claimBody,
    );
    await renewHumanTaskClaimApiHumanTasksHumanTaskIdClaimRenewalsPost(
      { human_task_id: taskId },
      tokenBody,
    );
    await releaseHumanTaskClaimApiHumanTasksHumanTaskIdClaimReleasesPost(
      { human_task_id: taskId },
      tokenBody,
    );
    await decideHumanTaskApiHumanTasksHumanTaskIdDecisionsPost(
      { human_task_id: taskId },
      decisionBody,
    );
    await resumeHumanGateApiReviewDecisionsReviewDecisionIdResumePost({
      review_decision_id: decisionId,
    });
    await getWorkflowRunApiWorkflowRunsWorkflowRunIdGet({ workflow_run_id: runId });

    expect(requestMock.mock.calls.map(([url]) => url)).toEqual([
      `/api/human-tasks/${taskId}/claims`,
      `/api/human-tasks/${taskId}/claim-renewals`,
      `/api/human-tasks/${taskId}/claim-releases`,
      `/api/human-tasks/${taskId}/decisions`,
      `/api/review-decisions/${decisionId}/resume`,
      `/api/workflow-runs/${runId}`,
    ]);
    expect(requestMock).toHaveBeenNthCalledWith(5, expect.any(String), {
      method: "POST",
    });
    expect(requestMock.mock.calls[0]?.[1]).toMatchObject({ data: claimBody });
    expect(requestMock.mock.calls[1]?.[1]).toMatchObject({ data: tokenBody });
    expect(requestMock.mock.calls[2]?.[1]).toMatchObject({ data: tokenBody });
    expect(requestMock.mock.calls[3]?.[1]).toMatchObject({ data: decisionBody });
    expect(requestMock.mock.calls.map(([url]) => url).join(" ")).not.toContain(
      claimToken,
    );
  });
});
