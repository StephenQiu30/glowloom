import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { ScriptAdaptationPanel } from "@/app/studio/[episodeId]/script-adaptation-panel";

const versionId = "019ff900-a000-7000-8000-000000000001";
const episode: API.EpisodeResponse = {
  id: "019ff900-a000-7000-8000-000000000002",
  workspace_id: "019ff900-a000-7000-8000-000000000003",
  project_id: "019ff900-a000-7000-8000-000000000004",
  name: "雾港倒计时 · 第一集",
  position: 1,
  target_duration_ms: 90_000,
  status: "active",
  revision: 2,
  current_script_version_id: versionId,
  current_timeline_version_id: null,
};
const currentVersion: API.ScriptVersionResponse = {
  id: versionId,
  workspace_id: episode.workspace_id,
  source_id: "019ff900-a000-7000-8000-000000000005",
  version_no: 1,
  status: "published",
  body: "第一集\n场景1：控制室，夜\n林澜：封锁港口。",
  content_hash: "a".repeat(64),
  created_by: "019ff900-a000-7000-8000-000000000006",
  created_at: "2026-08-13T05:00:00Z",
};

function run(status: API.AdaptationRunResponse["status"]): API.AdaptationRunResponse {
  return {
    id: "019ff900-a000-7000-8000-000000000007",
    workspace_id: episode.workspace_id,
    episode_id: episode.id,
    source_id: currentVersion.source_id,
    input_script_version_id: versionId,
    input_hash: currentVersion.content_hash,
    constraints: {
      target_duration_ms: 90_000,
      core_plot_points: ["林澜封锁港口", "结尾保留警报钩子"],
      pacing: "fast",
      colloquial_dialogue: true,
    },
    status,
    revision: 3,
    task_id: "019ff900-a000-7000-8000-000000000008",
    candidate_body: "第一集\n场景1：控制室，夜\n林澜：把港口封死。",
    candidate_hash: "b".repeat(64),
    draft_body: "第一集\n场景1：控制室，夜\n林澜：把港口封死。",
    draft_hash: "b".repeat(64),
    change_summary: "对白更口语化，保留核心冲突和结尾钩子。",
    estimated_duration_ms: 88_000,
    error_code: null,
    published_script_version_id: status === "published" ? versionId : null,
    created_at: "2026-08-13T05:01:00Z",
    updated_at: "2026-08-13T05:02:00Z",
  };
}

const defaultCallbacks = {
  onCancel: vi.fn(),
  onCompare: vi.fn(),
  onCreate: vi.fn(),
  onPublish: vi.fn(),
  onReset: vi.fn(),
  onSaveDraft: vi.fn(),
};

describe("ScriptAdaptationPanel", () => {
  it("只允许从当前已发布版本创建带四类约束的候选任务", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    const view = render(
      <ScriptAdaptationPanel
        {...defaultCallbacks}
        busy={false}
        currentVersion={{ ...currentVersion, status: "draft" }}
        difference={null}
        episode={episode}
        onCreate={onCreate}
      />,
    );

    expect(screen.getByText("先发布当前剧本")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "生成改写候选" })).toBeDisabled();

    view.rerender(
      <ScriptAdaptationPanel
        {...defaultCallbacks}
        busy={false}
        currentVersion={currentVersion}
        difference={null}
        episode={episode}
        onCreate={onCreate}
      />,
    );
    await user.type(
      screen.getByLabelText("必须保留的核心情节"),
      "林澜封锁港口\n结尾保留警报钩子",
    );
    await user.click(screen.getByRole("button", { name: "生成改写候选" }));

    expect(onCreate).toHaveBeenCalledWith(
      expect.objectContaining({
        input_script_version_id: versionId,
        target_duration_ms: 90_000,
        core_plot_points: ["林澜封锁港口", "结尾保留警报钩子"],
        pacing: "fast",
        colloquial_dialogue: true,
        idempotency_key: expect.stringMatching(/^studio-adaptation:/),
      }),
    );
  });

  it("成功候选支持编辑、服务端差异和显式发布", async () => {
    const user = userEvent.setup();
    const onSaveDraft = vi.fn().mockResolvedValue(undefined);
    const onCompare = vi.fn().mockResolvedValue(undefined);
    const onPublish = vi.fn().mockResolvedValue(undefined);
    render(
      <ScriptAdaptationPanel
        {...defaultCallbacks}
        busy={false}
        currentVersion={currentVersion}
        difference={{
          base_version_id: versionId,
          adaptation_run_id: run("succeeded").id,
          added_lines: 1,
          removed_lines: 1,
          diff_lines: ["-林澜：封锁港口。", "+林澜：把港口封死。"],
        }}
        episode={episode}
        run={run("succeeded")}
        onCompare={onCompare}
        onPublish={onPublish}
        onSaveDraft={onSaveDraft}
      />,
    );

    const editor = screen.getByLabelText("改写工作稿");
    await user.clear(editor);
    await user.type(editor, "林澜：马上封锁港口。\n警报骤响。", {
      skipClick: true,
    });
    await user.click(screen.getByRole("button", { name: "保存工作稿" }));
    await user.click(screen.getByRole("button", { name: "查看差异" }));
    await user.click(screen.getByRole("button", { name: "发布并设为当前" }));

    expect(onSaveDraft).toHaveBeenCalledWith("林澜：马上封锁港口。\n警报骤响。");
    expect(onCompare).toHaveBeenCalledOnce();
    expect(onPublish).toHaveBeenCalledOnce();
    expect(screen.getByText("新增 1 行 · 删除 1 行")).toBeInTheDocument();
  });

  it("仅排队状态可取消，unknown 明确要求人工重新发起", async () => {
    const user = userEvent.setup();
    const onCancel = vi.fn().mockResolvedValue(undefined);
    const view = render(
      <ScriptAdaptationPanel
        {...defaultCallbacks}
        busy={false}
        currentVersion={currentVersion}
        difference={null}
        episode={episode}
        run={run("queued")}
        onCancel={onCancel}
      />,
    );

    await user.click(screen.getByRole("button", { name: "取消排队" }));
    expect(onCancel).toHaveBeenCalledOnce();

    view.rerender(
      <ScriptAdaptationPanel
        {...defaultCallbacks}
        busy={false}
        currentVersion={currentVersion}
        difference={null}
        episode={episode}
        run={{ ...run("unknown"), error_code: "provider_outcome_unknown" }}
        onCancel={onCancel}
      />,
    );
    expect(screen.queryByRole("button", { name: "取消排队" })).not.toBeInTheDocument();
    expect(screen.getByText(/原稿与当前版本未改变/)).toBeInTheDocument();
    expect(screen.getByText(/provider_outcome_unknown/)).toBeInTheDocument();
  });
});
