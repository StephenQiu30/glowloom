import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const apiMocks = vi.hoisted(() => ({
  confirmPlan: vi.fn(),
  createPlan: vi.fn(),
  getPlan: vi.fn(),
  materializePlan: vi.fn(),
  mergeProposals: vi.fn(),
  moveBoundary: vi.fn(),
  publishCommit: vi.fn(),
  renameProposal: vi.fn(),
  splitProposal: vi.fn(),
}));

vi.mock("@/api/episodePlanning", async () => ({
  ...(await vi.importActual<typeof import("@/api/episodePlanning")>(
    "@/api/episodePlanning",
  )),
  confirmEpisodePlanApiV1EpisodePlansPlanIdConfirmPost: apiMocks.confirmPlan,
  createEpisodePlanApiV1DocumentRevisionsRevisionIdEpisodePlansPost:
    apiMocks.createPlan,
  getEpisodePlanApiV1EpisodePlansPlanIdGet: apiMocks.getPlan,
  materializeEpisodePlanApiV1EpisodePlansPlanIdMaterializationsPost:
    apiMocks.materializePlan,
  mergeEpisodeProposalsApiV1EpisodePlansPlanIdMergePost:
    apiMocks.mergeProposals,
  moveEpisodeBoundaryApiV1EpisodePlansPlanIdMoveBoundaryPost:
    apiMocks.moveBoundary,
  publishImportCommitApiV1ImportCommitsCommitIdPublishPost:
    apiMocks.publishCommit,
  renameEpisodeProposalApiV1EpisodePlansPlanIdRenamePost:
    apiMocks.renameProposal,
  splitEpisodeProposalApiV1EpisodePlansPlanIdSplitPost:
    apiMocks.splitProposal,
}));

import { AppProviders } from "@/app/providers";
import { EpisodePlanWorkspace } from "@/app/projects/[projectId]/episode-plan-workspace";
import { setAccessToken } from "@/lib/auth-session";

const revisionId = "019ff900-a000-7000-8000-000000000001";
const planId = "019ff900-a000-7000-8000-000000000002";
const proposalOneId = "019ff900-a000-7000-8000-000000000003";
const proposalTwoId = "019ff900-a000-7000-8000-000000000004";
const commitId = "019ff900-a000-7000-8000-000000000030";
const episodeOneId = "019ff900-a000-7000-8000-000000000031";
const episodeTwoId = "019ff900-a000-7000-8000-000000000032";
const source = "第一集\n场景1：控制室，夜\n甲：开始。\n第二集\n场景2：港口，雨\n乙：继续。";

const analysis = {
  document: {
    id: "019ff900-a000-7000-8000-000000000010",
    workspace_id: "019ff900-a000-7000-8000-000000000011",
    project_id: "019ff900-a000-7000-8000-000000000012",
    title: "雾港倒计时",
    source_type: "text" as const,
    source_media_version_id: null,
    language: "zh-CN",
    rights_declaration: "已授权",
    status: "active" as const,
    revision: 1,
    created_by: "019ff900-a000-7000-8000-000000000013",
    created_at: "2026-08-13T04:00:00Z",
  },
  revision: {
    id: revisionId,
    workspace_id: "019ff900-a000-7000-8000-000000000011",
    document_id: "019ff900-a000-7000-8000-000000000010",
    version_no: 1,
    source_type: "text" as const,
    source_media_version_id: null,
    raw_text: source,
    raw_hash: "a".repeat(64),
    normalized_text: source,
    normalized_hash: "a".repeat(64),
    normalizer_version: "identity-v1",
    normalization_map: { type: "identity" },
    codepoint_count: source.length,
    analysis_status: "deterministic" as const,
    analyzer_version: "whole-script-lines-v1",
    created_by: "019ff900-a000-7000-8000-000000000013",
    created_at: "2026-08-13T04:00:00Z",
  },
  blocks: [],
  issues: [],
} satisfies API.ScriptDocumentAnalysisResponse;

function planDetail(
  status: API.EpisodePlanResponse["status"],
  revision: number,
  firstTitle = "警报前夜",
): API.EpisodePlanDetailResponse {
  const midpoint = source.indexOf("第二集");
  return {
    plan: {
      id: planId,
      workspace_id: analysis.revision.workspace_id,
      project_id: analysis.document.project_id,
      document_revision_id: revisionId,
      strategy: "explicit_markers",
      status,
      target_duration_ms: 90_000,
      requested_episode_count: null,
      total_estimated_duration_ms: 118_000,
      input_hash: "b".repeat(64),
      planning_engine_version: "episode-planning-v1",
      model_name: null,
      prompt_version: null,
      schema_version: "episode-plan-schema-v1",
      planning_task_id: null,
      planning_error_code: null,
      revision,
      confirmed_by: status === "review_ready" ? null : analysis.document.created_by,
      confirmed_at: status === "review_ready" ? null : "2026-08-13T04:01:00Z",
      created_by: analysis.document.created_by,
      created_at: "2026-08-13T04:00:30Z",
      updated_at: "2026-08-13T04:01:00Z",
    },
    proposals: [
      {
        id: proposalOneId,
        plan_id: planId,
        position: 1,
        title: firstTitle,
        start_block_id: "019ff900-a000-7000-8000-000000000020",
        end_block_id: "019ff900-a000-7000-8000-000000000021",
        start_block_position: 1,
        end_block_position: 3,
        source_start: 0,
        source_end: midpoint,
        content_hash: "c".repeat(64),
        estimated_duration_ms: 58_000,
        reason: "冲突建立并以警报作为钩子",
        confidence: 1,
        boundary_evidence: { kind: "explicit_marker" },
        is_locked: false,
      },
      {
        id: proposalTwoId,
        plan_id: planId,
        position: 2,
        title: "追踪真相",
        start_block_id: "019ff900-a000-7000-8000-000000000022",
        end_block_id: "019ff900-a000-7000-8000-000000000023",
        start_block_position: 4,
        end_block_position: 6,
        source_start: midpoint,
        source_end: source.length,
        content_hash: "d".repeat(64),
        estimated_duration_ms: 60_000,
        reason: "完成追踪并留下下一集钩子",
        confidence: 0.91,
        boundary_evidence: { kind: "explicit_marker" },
        is_locked: false,
      },
    ],
    impact: {
      project_revision: 1,
      active_episode_count: 0,
      active_order_hash: "e".repeat(64),
      projected_episode_count: 2,
      allowed: true,
      blockers: [],
    },
    source: {
      document_revision_id: revisionId,
      normalized_text: source,
      normalized_hash: "a".repeat(64),
      codepoint_count: source.length,
      blocks: [],
    },
  };
}

function commitSegments(published: boolean): API.EpisodeSegmentOriginResponse[] {
  return [episodeOneId, episodeTwoId].map((episodeId, index) => ({
    id: `019ff900-a000-7000-8000-00000000004${index}`,
    import_commit_id: commitId,
    proposal_id: index === 0 ? proposalOneId : proposalTwoId,
    document_revision_id: revisionId,
    episode_id: episodeId,
    source_id: `019ff900-a000-7000-8000-00000000005${index}`,
    draft_version_id: `019ff900-a000-7000-8000-00000000006${index}`,
    published_version_id: published
      ? `019ff900-a000-7000-8000-00000000007${index}`
      : null,
    position: index + 1,
    source_start: index === 0 ? 0 : source.indexOf("第二集"),
    source_end: index === 0 ? source.indexOf("第二集") : source.length,
    source_hash: (index === 0 ? "1" : "2").repeat(64),
  }));
}

describe("分集计划向导", () => {
  beforeEach(() => {
    sessionStorage.clear();
    setAccessToken("test-access-token");
    vi.clearAllMocks();
    apiMocks.createPlan.mockResolvedValue({ data: planDetail("review_ready", 1) });
    apiMocks.confirmPlan.mockResolvedValue({
      data: planDetail("confirmed", 2),
    });
    apiMocks.materializePlan.mockResolvedValue({
      data: {
        commit: {
          id: commitId,
          workspace_id: analysis.revision.workspace_id,
          project_id: analysis.document.project_id,
          plan_id: planId,
          mode: "append_new",
          status: "materialized",
          input_hash: "f".repeat(64),
          expected_project_revision: 1,
          expected_active_order_hash: "e".repeat(64),
          error_code: null,
          revision: 2,
          created_by: analysis.document.created_by,
          created_at: "2026-08-13T04:02:00Z",
          updated_at: "2026-08-13T04:02:00Z",
        },
        segments: commitSegments(false),
      } satisfies API.ImportCommitDetailResponse,
    });
    apiMocks.publishCommit.mockResolvedValue({
      data: {
        commit: {
          id: commitId,
          workspace_id: analysis.revision.workspace_id,
          project_id: analysis.document.project_id,
          plan_id: planId,
          mode: "append_new",
          status: "published",
          input_hash: "f".repeat(64),
          expected_project_revision: 1,
          expected_active_order_hash: "e".repeat(64),
          error_code: null,
          revision: 3,
          created_by: analysis.document.created_by,
          created_at: "2026-08-13T04:02:00Z",
          updated_at: "2026-08-13T04:03:00Z",
        },
        segments: commitSegments(true),
      } satisfies API.ImportCommitDetailResponse,
    });
  });

  it("从确定性候选审阅到批量发布始终使用服务端 revision", async () => {
    const user = userEvent.setup();
    render(
      <AppProviders>
        <EpisodePlanWorkspace
          analysis={analysis}
          canWrite
          targetDurationMs={90_000}
        />
      </AppProviders>,
    );

    await user.click(
      screen.getByRole("button", { name: "生成确定性分集计划" }),
    );
    expect(await screen.findByText("冲突建立并以警报作为钩子")).toBeInTheDocument();
    expect(screen.getByText(/置信度\s*100%/)).toBeInTheDocument();
    expect(screen.getByText(/场景1：控制室/)).toBeInTheDocument();

    expect(screen.getByLabelText("第 1 集标题")).toHaveAttribute("readonly");

    await user.click(screen.getByRole("button", { name: "确认分集计划" }));
    expect(apiMocks.confirmPlan).toHaveBeenCalledWith(
      { plan_id: planId },
      expect.objectContaining({ expected_revision: 1 }),
    );
    await user.click(screen.getByRole("button", { name: "原子创建 2 集" }));
    expect(apiMocks.materializePlan).toHaveBeenCalledWith(
      { plan_id: planId },
      expect.objectContaining({
        expected_plan_revision: 2,
        expected_project_revision: 1,
        expected_active_order_hash: "e".repeat(64),
      }),
    );
    await user.click(screen.getByRole("button", { name: "发布 2 集剧本" }));
    expect(apiMocks.publishCommit).toHaveBeenCalledWith(
      { commit_id: "019ff900-a000-7000-8000-000000000030" },
      expect.objectContaining({ expected_revision: 2 }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent(
      "2 集剧本已批量发布；每集均已生成待确认的场景与制作任务。",
    );
  });
});
