import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const createBible = vi.fn();
const confirmBible = vi.fn();
const decideReviewIssue = vi.fn();
const resumeBible = vi.fn();
let currentBible:
  | (API.ProductionBibleResponse & {
      review_decisions?: Record<string, "accepted" | "rejected">;
    })
  | undefined;
let currentError: { code: string; message: string } | undefined;

vi.mock("@/lib/server-state", () => ({
  appApiErrorMessage: (error: { message?: string }) => error?.message ?? "请求失败",
  useConfirmProductionBibleMutation: () => [confirmBible, { isLoading: false }],
  useCreateProductionBibleMutation: () => [createBible, { isLoading: false }],
  useCurrentProductionBibleQuery: () => ({ data: currentBible, error: currentError }),
  useDecideProductionBibleReviewIssueMutation: () => [decideReviewIssue, { isLoading: false }],
  useProductionBibleQuery: () => ({ data: undefined, error: undefined }),
  useResumeProductionBibleMutation: () => [resumeBible, { isLoading: false }],
}));

import { ProductionBibleWorkspace } from "@/app/projects/[projectId]/production-bible-workspace";

const analysis = {
  revision: {
    id: "revision-1",
    normalized_hash: "a".repeat(64),
  },
} as API.ScriptDocumentAnalysisResponse;

function bible(
  status: API.ProductionBibleResponse["status"],
): API.ProductionBibleResponse {
  return {
    checkpoint_revision: 2,
    checkpoint_stage: "synthesis",
    checkpoint_updated_at: "2026-08-25T00:00:00Z",
    confirmed_at: status === "confirmed" ? "2026-08-25T00:01:00Z" : null,
    confirmed_by: null,
    created_at: "2026-08-25T00:00:00Z",
    document_revision_id: "revision-1",
    engine_version: "1",
    entities: [
      {
        aliases: [],
        asset_id: null,
        canonical_name: "沈岚",
        created_at: "2026-08-25T00:00:00Z",
        entity_key: "character.shen-lan",
        episode_numbers: [1, 2],
        evidence: [],
        id: "entity-1",
        kind: "character",
        normalized_name: "沈岚",
        stable_spec: {},
        states: [],
        updated_at: "2026-08-25T00:00:00Z",
      },
    ],
    harness_version: "1",
    id: "bible-1",
    input_hash: "b".repeat(64),
    model_name: "codex-local",
    project_id: "project-1",
    prompt_version: "1",
    result_hash: "c".repeat(64),
    review_issues: [],
    review_decisions: {},
    revision: 3,
    schema_version: "1",
    status,
    task_id: "task-1",
    updated_at: "2026-08-25T00:00:00Z",
    workspace_id: "workspace-1",
    world_entries: [],
  };
}

function renderWorkspace() {
  return render(
    <ProductionBibleWorkspace
      analysis={analysis}
      canWrite
      projectId="project-1"
    />,
  );
}

describe("ProductionBibleWorkspace", () => {
  beforeEach(() => {
    currentBible = undefined;
    currentError = undefined;
    createBible.mockReset();
    confirmBible.mockReset();
    decideReviewIssue.mockReset();
    resumeBible.mockReset();
  });

  it("在没有制作圣经时提供真实生成入口", async () => {
    currentError = { code: "not_found", message: "Production Bible not found" };
    createBible.mockReturnValue({ unwrap: () => Promise.resolve(bible("queued")) });

    renderWorkspace();
    fireEvent.click(screen.getByRole("button", { name: "生成项目制作圣经" }));

    await waitFor(() => expect(createBible).toHaveBeenCalledTimes(1));
    expect(createBible.mock.calls[0][0]).toMatchObject({
      projectId: "project-1",
      revisionId: "revision-1",
    });
  });

  it("展示待审实体并使用版本与结果哈希确认", async () => {
    currentBible = bible("needs_review");
    confirmBible.mockReturnValue({ unwrap: () => Promise.resolve(bible("confirmed")) });

    renderWorkspace();
    expect(screen.getByRole("region", { name: "制作圣经实体" })).toHaveTextContent("沈岚");
    fireEvent.click(screen.getByRole("button", { name: "确认制作圣经" }));

    await waitFor(() => expect(confirmBible).toHaveBeenCalledTimes(1));
    expect(confirmBible.mock.calls[0][0]).toMatchObject({
      bibleId: "bible-1",
      body: {
        expected_result_hash: "c".repeat(64),
        expected_revision: 3,
      },
    });
  });

  it("确认后明确允许后续分集发布", () => {
    currentBible = bible("confirmed");
    renderWorkspace();
    expect(screen.getByRole("status")).toHaveTextContent("制作圣经已确认");
    expect(screen.getByText(/分集发布与后续场景、任务、分镜/)).toBeVisible();
  });

  it("阻断问题必须逐项记录人工接受决议", async () => {
    currentBible = {
      ...bible("needs_review"),
      review_decisions: {},
      review_issues: [
        {
          code: "episode_mapping_incomplete",
          evidence: [],
          issue_key: "issue.mapping",
          repair_hint: null,
          scope: "entity",
          severity: "blocking",
          subject_key: "character.shen-lan",
          summary: "集数范围与证据不一致",
        },
      ],
    };
    decideReviewIssue.mockReturnValue({
      unwrap: () =>
        Promise.resolve({
          ...currentBible,
          review_decisions: { "issue.mapping": "accepted" },
          revision: 4,
        }),
    });

    renderWorkspace();
    expect(screen.getByRole("button", { name: "确认制作圣经" })).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "接受风险并继续" }));

    await waitFor(() => expect(decideReviewIssue).toHaveBeenCalledTimes(1));
    expect(decideReviewIssue.mock.calls[0][0]).toMatchObject({
      bibleId: "bible-1",
      body: {
        action: "accepted",
        expected_revision: 3,
        issue_key: "issue.mapping",
      },
    });
  });

  it("不会把旧原稿的制作圣经误用于当前 Revision", () => {
    currentBible = {
      ...bible("confirmed"),
      document_revision_id: "revision-old",
    };
    renderWorkspace();
    expect(screen.getByText(/项目已有旧原稿的制作圣经/)).toBeVisible();
    expect(screen.getByRole("button", { name: "生成项目制作圣经" })).toBeEnabled();
    expect(screen.queryByText("制作圣经已确认", { exact: true })).not.toBeInTheDocument();
  });
});
