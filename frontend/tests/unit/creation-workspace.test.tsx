import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

const requestMock = vi.hoisted(() => vi.fn());
vi.mock("@/lib/request", async () => ({
  ...(await vi.importActual<typeof import("@/lib/request")>("@/lib/request")),
  default: requestMock,
}));

import { AppProviders } from "@/app/providers";
import { TextCreationWorkspace } from "@/features/creation/text-creation-workspace";
import { ProposalReview } from "@/features/creation/proposal-review";
import { ApiClientError } from "@/lib/request";

const projectId = "41000000-0000-4000-8000-000000000001";
const revisionId = "41000000-0000-4000-8000-000000000002";
const runId = "41000000-0000-4000-8000-000000000003";
const hash = "a".repeat(64);
const source = { revisionId, contentHash: hash, title: "门外来信", text: "甲😀：开门。" };
const run = {
  id: runId, project_id: projectId, workspace_id: projectId,
  source: { document_id: projectId, revision_id: revisionId, revision: 1, content_hash: hash, span_index_id: projectId },
  flow_type: "lanverse.creation.text-storyboard.production", workflow_id: `lanverse:creation:${runId}`,
  status: "accepted", revision: 2, last_error: "", acceptance: null,
  created_at: "2026-09-08T00:00:00Z", updated_at: "2026-09-08T00:00:00Z",
};
const proposal: API.CreationProposal = {
  id: revisionId, run_id: runId, step_id: projectId, stage: "direct_scene", step_key: "direct_scene/ep1/door", revision: 1,
  result_hash: hash, candidate_hash: hash, source_revision_id: revisionId, source_hash: hash,
  invocation_id: "scene-door", input_hash: hash, release_hash: hash, human_task_id: projectId,
  candidate: { episode_key: "ep1", scene_key: "door", dramatic_intent: "门外来信引发犹豫", audience_knows: ["甲在等一封信"], withhold: ["来信人的身份"], blocking: [], issues: [], shots: [{
    key: "shot1", purpose: "呈现迟疑", framing: "手部特写", camera_movement: "固定", action: "甲的手停在门把上", beat_keys: ["wait"], audio: [], visible_mentions: [], detail_evidence: [],
    duration_min_ms: 1000, duration_max_ms: 3000, timing_basis: "等待与停顿", screen_direction: "从左向右", entry_state: "手伸向门", exit_state: "停在门把上", panel_caption: "门把上的手，停顿。",
  }] },
  evidence: [{ revision_id: revisionId, source_hash: hash, block: 0, start: 0, end: 2, text_hash: hash, quote: "甲😀" }],
  issues: [{ code: "continuity_mapping_pending", scope: "ep1/door", severity: "blocker", summary: "跨场状态与披露需要人工核对" }],
  status: "needs_review", acceptance: null, created_at: "2026-09-08T00:00:00Z",
};

describe("文本核心闭环", () => {
  beforeEach(() => {
    requestMock.mockReset();
    requestMock.mockImplementation(async (url: string, options?: { method?: string }) => {
      if (url.endsWith("/current-script-source")) throw new ApiClientError("not_found", "not_found");
      if (url.endsWith("/creation-runs") && options?.method === "GET") return { data: [] };
      if (url.endsWith("/script-sources")) return { data: { identity: { version_id: revisionId, content_hash: hash } } };
      if (url.endsWith("/creation-runs") && options?.method === "POST") return { data: run };
      if (url.endsWith("/proposals")) return { data: [] };
      throw new Error(`unexpected request: ${url}`);
    });
  });

  it("先正式固定源，再用同一版本和哈希创建运行", async () => {
    const user = userEvent.setup();
    render(<AppProviders><TextCreationWorkspace projectId={projectId} source={source} canWrite /></AppProviders>);
    await user.click(await screen.findByRole("button", { name: "固定原稿并开始创作" }));
    await waitFor(() => expect(requestMock).toHaveBeenCalledWith(`/api/projects/${projectId}/creation-runs`, expect.objectContaining({
      method: "POST", data: expect.objectContaining({ document_revision_id: revisionId, source_hash: hash }),
    })));
    const acceptance = requestMock.mock.calls.findIndex(([url]) => url === `/api/projects/${projectId}/script-sources`);
    const creation = requestMock.mock.calls.findIndex(([url, options]) => url === `/api/projects/${projectId}/creation-runs` && options.method === "POST");
    expect(acceptance).toBeGreaterThan(-1);
    expect(creation).toBeGreaterThan(acceptance);
  });

  it("只读成员可查回运行但不能启动或同步提案", async () => {
    requestMock.mockImplementation(async (url: string) => {
      if (url.endsWith("/creation-runs")) return { data: [run] };
      if (url.endsWith("/proposals")) return { data: [] };
      throw new ApiClientError("not_found", "not_found");
    });
    render(<AppProviders><TextCreationWorkspace projectId={projectId} source={source} canWrite={false} /></AppProviders>);
    expect(await screen.findByText("Agent 已接受，等待执行状态")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "固定原稿并开始创作" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "同步执行与提案" })).not.toBeInTheDocument();
    expect(screen.queryByText("创作已完成")).not.toBeInTheDocument();
  });

  it("需要明确处置问题，批准后再单独正式采纳", async () => {
    const user = userEvent.setup();
    let decision: unknown = null;
    const task = { id: projectId, status: "CLAIMED", revision: 2, claim: { claim_token: runId }, subject_revision: 1, subject_hash: hash };
    requestMock.mockImplementation(async (url: string, options?: { method?: string; data?: unknown }) => {
      if (url.endsWith("/decisions")) { decision = { id: runId, decision: "approved" }; return { data: { task, decision, coordination: null } }; }
      if (url.endsWith("/adopt")) return { data: { submission_id: runId, formal_refs: [{ type: "text_intent_version", id: revisionId, revision: 1 }], risk_resolutions: [{ code: "continuity_mapping_pending", scope: "ep1/door", reason: "已核对，门外身份保持隐藏，手的位置连续。" }] } };
      if (url === `/api/human-tasks/${projectId}`) return { data: { task, decision, coordination: null } };
      throw new Error(`unexpected ${options?.method} ${url}`);
    });
    render(<AppProviders><ProposalReview proposal={proposal} proposals={[proposal]} projectId={projectId} canWrite /></AppProviders>);
    expect(await screen.findByRole("button", { name: "批准草案" })).toBeDisabled();
    expect(screen.getByRole("region", { name: "镜头 1" })).toHaveTextContent("门把上的手，停顿。");
    expect(screen.getByText("甲😀")).toBeInTheDocument();
    await user.type(screen.getByLabelText("处理说明：跨场状态与披露需要人工核对"), "已核对，门外身份保持隐藏，手的位置连续。");
    await user.click(screen.getByRole("button", { name: "批准草案" }));
    const adoptButton = await screen.findByRole("button", { name: "正式采纳并继续" });
    expect(requestMock.mock.calls.some(([url]) => String(url).endsWith("/adopt"))).toBe(false);
    await user.click(adoptButton);
    expect(await screen.findByRole("region", { name: "正式采纳回执" })).toHaveTextContent("text_intent_version");
    expect(requestMock).toHaveBeenCalledWith(`/api/creation-runs/${runId}/proposals/${revisionId}/adopt`, expect.objectContaining({ data: expect.objectContaining({ decision_id: runId, expected_revision: 1, risk_resolutions: [{ code: "continuity_mapping_pending", scope: "ep1/door", reason: "已核对，门外身份保持隐藏，手的位置连续。" }] }) }));
  });

  it("分析已经执行时，阶段进度展示已生成草案数", async () => {
    requestMock.mockImplementation(async (url: string) => {
      if (url.endsWith("/creation-runs")) return { data: [run] };
      if (url.endsWith("/proposals")) return { data: [] };
      if (url.endsWith("/execution")) return { data: { status: "running", stage: "analyze_episode", call_limit: 100, reserved_calls: 3, steps: [{ step_key: "analyze_episode/ep1", state: "needs_review" }, { step_key: "analyze_episode/ep2", state: "running" }] } };
      throw new ApiClientError("not_found", "not_found");
    });
    render(<AppProviders><TextCreationWorkspace projectId={projectId} source={source} canWrite /></AppProviders>);
    expect(await screen.findByText("已生成 1 份草案，正在分析")).toBeInTheDocument();
  });

  it("未知推理显示原运行受阻且不提供重复推理按钮", async () => {
    requestMock.mockImplementation(async (url: string) => {
      if (url.endsWith("/creation-runs")) return { data: [run] };
      if (url.endsWith("/proposals")) return { data: [] };
      if (url.endsWith("/execution")) return { data: { status: "blocked", stage: "direct_scene", call_limit: 20, reserved_calls: 4, last_error: "invocation_outcome_unknown", steps: [{ state: "unknown" }] } };
      throw new ApiClientError("not_found", "not_found");
    });
    render(<AppProviders><TextCreationWorkspace projectId={projectId} source={source} canWrite /></AppProviders>);
    expect(await screen.findByText("执行受阻")).toBeInTheDocument();
    expect(screen.getByText(/保留本次运行/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /重新生成|重试模型|恢复原运行交接/ })).not.toBeInTheDocument();
    expect(requestMock.mock.calls.some(([, options]) => options?.method === "POST")).toBe(false);
  });
});
