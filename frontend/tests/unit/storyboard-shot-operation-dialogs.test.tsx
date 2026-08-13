import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import {
  buildMergeTarget,
  buildSplitTargets,
  DeleteShotDialog,
  MergeShotsDialog,
  type ShotTransformSource,
  SplitShotDialog,
} from "@/app/studio/[episodeId]/storyboard-shot-operations";

const workspaceId = "019fb6d0-a000-7000-8000-000000000001";
const episodeId = "019fb6d0-a000-7000-8000-000000000002";
const scriptVersionId = "019fb6d0-a000-7000-8000-000000000003";
const sceneId = "019fb6d0-a000-7000-8000-000000000004";
const dialogueId = "019fb6d0-a000-7000-8000-000000000005";
const secondDialogueId = "019fb6d0-a000-7000-8000-000000000006";
const otherSceneId = "019fb6d0-a000-7000-8000-000000000007";
const now = "2026-07-31T12:00:00Z";

function transformSource(position: number): ShotTransformSource {
  const shotId = `019fb6d0-a000-7000-8000-00000000001${position}`;
  const versionId = `019fb6d0-a000-7000-8000-00000000002${position}`;
  const sourceDialogueId = position === 1 ? dialogueId : secondDialogueId;
  return {
    shot: {
      id: shotId,
      workspace_id: workspaceId,
      episode_id: episodeId,
      position,
      title: `镜头 ${position}`,
      source_script_version_id: scriptVersionId,
      source_scene_id: sceneId,
      source_candidate_id: null,
      source_draft_shot_id: null,
      status: "active",
      current_spec_version_id: versionId,
      revision: 2,
      created_at: now,
      updated_at: now,
    },
    version: {
      id: versionId,
      workspace_id: workspaceId,
      shot_id: shotId,
      version_no: 1,
      schema_version: 1,
      spec: {
        schema_version: 1,
        script_reference: {
          confirmed_script_version_id: scriptVersionId,
          scene_id: sceneId,
          dialogue_ids: [sourceDialogueId],
        },
        narrative: { purpose: `完成镜头 ${position}`, continuity_note: null },
        visual: {
          shot_size: "medium",
          camera_angle: "eye_level",
          camera_movement: "static",
          composition: "主体位于三分线",
          environment: "雨夜车站",
          subject_placements: [],
          mood_lighting: "冷蓝侧光",
        },
        action_beats: [
          { beat_key: "beat-1", order: 1, description: `动作 ${position}` },
          { beat_key: "beat-2", order: 2, description: `反应 ${position}` },
        ],
        dialogue_or_narration: [
          {
            source_dialogue_id: sourceDialogueId,
            beat_key: "beat-1",
            speaker_subject_key: null,
            render_as_audio: false,
            performance_note: null,
          },
        ],
        duration_ms: 4_000,
        audio_intent: { ambient: "雨声", sound_effects: [] },
        generation_intent: {
          mode: "text_to_video",
          first_frame: null,
          last_frame: null,
          keyframe_notes: null,
        },
      },
      content_hash: "a".repeat(64),
      input_hash: "b".repeat(64),
      asset_references: [],
      created_by: workspaceId,
      created_at: now,
    },
  };
}

function preflight(
  operation: "split" | "merge",
  sources: ShotTransformSource[],
): API.ShotTransformPreflightResponse {
  return {
    operation,
    source_shot_ids: sources.map((source) => source.shot.id),
    source_spec_version_ids: sources.map((source) => source.version.id),
    order_hash: "o".repeat(64),
    downstream_evidence: {
      generation_request_ids: [],
      candidate_ids: [],
      review_ids: [],
      issue_ids: [],
      timeline_source_ids: [],
    },
    impact_hash: "i".repeat(64),
  };
}

describe("分镜结构操作对话框", () => {
  it("拆分构造器按明确边界完整分配动作与对白", () => {
    const source = transformSource(1);
    const targets = buildSplitTargets(source.version, {
      firstTitle: "前段",
      secondTitle: "后段",
      firstDurationMs: 2_000,
      firstActionCount: 1,
      firstDialogueCount: 1,
    });

    expect(
      targets.flatMap((target) =>
        target.spec.action_beats.map((beat) => beat.description),
      ),
    ).toEqual(["动作 1", "反应 1"]);
    expect(
      targets.flatMap(
        (target) => target.spec.script_reference.dialogue_ids,
      ),
    ).toEqual([dialogueId]);
    expect(targets.map((target) => target.spec.action_beats.length)).toEqual([
      1, 1,
    ]);
  });

  it("合并构造器拒绝跨场与不可表示的动作数量", () => {
    const first = transformSource(1);
    const crossScene = structuredClone(transformSource(2));
    crossScene.shot.source_scene_id = otherSceneId;
    crossScene.version.spec.script_reference.scene_id = otherSceneId;
    expect(() =>
      buildMergeTarget(first.version, crossScene.version, {
        baseVersionId: first.version.id,
        title: "跨场合并",
      }),
    ).toThrow(/同一场次/);

    const second = transformSource(2);
    first.version.spec.action_beats = Array.from({ length: 5 }, (_, index) => ({
      beat_key: `first-${index + 1}`,
      order: index + 1,
      description: `前镜动作 ${index + 1}`,
    }));
    second.version.spec.action_beats = Array.from({ length: 4 }, (_, index) => ({
      beat_key: `second-${index + 1}`,
      order: index + 1,
      description: `后镜动作 ${index + 1}`,
    }));
    expect(() =>
      buildMergeTarget(first.version, second.version, {
        baseVersionId: first.version.id,
        title: "动作超限",
      }),
    ).toThrow(/8 个动作/);
  });

  it("同场合并完整保留动作、对白和重编号后的关联", () => {
    const first = transformSource(1);
    const second = transformSource(2);
    const target = buildMergeTarget(first.version, second.version, {
      baseVersionId: first.version.id,
      title: "完整合并",
    });

    expect(target.spec.action_beats.map((beat) => beat.description)).toEqual([
      "动作 1",
      "反应 1",
      "动作 2",
      "反应 2",
    ]);
    expect(target.spec.script_reference.dialogue_ids).toEqual([
      dialogueId,
      secondDialogueId,
    ]);
    expect(
      (target.spec.dialogue_or_narration ?? []).map((item) => item.beat_key),
    ).toEqual(["beat-1", "beat-3"]);
  });

  it("先预检再提交完整拆分目标", async () => {
    const user = userEvent.setup();
    const source = transformSource(1);
    const onPreflight = vi
      .fn()
      .mockResolvedValue(preflight("split", [source]));
    const onApply = vi.fn().mockResolvedValue(true);
    render(
      <SplitShotDialog
        busy={false}
        orderHash={"a".repeat(64)}
        source={source}
        onApply={onApply}
        onPreflight={onPreflight}
      />,
    );

    await user.click(screen.getByRole("button", { name: "拆分" }));
    await user.click(screen.getByRole("button", { name: "检查拆分影响" }));
    expect(onPreflight).toHaveBeenCalledWith(source.shot.id, {
      expected_source_spec_version_id: source.version.id,
      expected_order_hash: "a".repeat(64),
    });
    await user.clear(screen.getByLabelText("前段时长（毫秒）"));
    await user.type(screen.getByLabelText("前段时长（毫秒）"), "2300");
    await user.click(screen.getByRole("button", { name: "确认拆分" }));

    await waitFor(() => expect(onApply).toHaveBeenCalledOnce());
    const request = onApply.mock.calls[0][1] as API.SplitShotRequest;
    expect(request.impact_hash).toBe("i".repeat(64));
    expect(request.targets.map((target) => target.spec.duration_ms)).toEqual([
      2_300,
      1_700,
    ]);
    expect(
      request.targets.map((target) => target.spec.action_beats.length),
    ).toEqual([1, 1]);
    expect(
      request.targets.map(
        (target) =>
          target.spec.script_reference.dialogue_ids?.length ?? 0,
      ),
    ).toEqual([1, 0]);
  });

  it("合并相邻镜头时固定预检结果并提交目标规格", async () => {
    const user = userEvent.setup();
    const first = transformSource(1);
    const second = transformSource(2);
    const onPrepare = vi.fn().mockResolvedValue({
      preflight: preflight("merge", [first, second]),
      sources: [first, second],
    });
    const onApply = vi.fn().mockResolvedValue(true);
    render(
      <MergeShotsDialog
        busy={false}
        candidates={[second.shot]}
        source={first}
        onApply={onApply}
        onPrepare={onPrepare}
      />,
    );

    await user.click(screen.getByRole("button", { name: "合并" }));
    await user.click(screen.getByRole("button", { name: "检查合并影响" }));
    expect(onPrepare).toHaveBeenCalledWith(first.shot, second.shot);
    await user.click(screen.getByRole("button", { name: "确认合并" }));

    await waitFor(() => expect(onApply).toHaveBeenCalledOnce());
    expect(onApply).toHaveBeenCalledWith(
      expect.objectContaining({
        shot_ids: [first.shot.id, second.shot.id],
        expected_spec_version_ids: [first.version.id, second.version.id],
        impact_hash: "i".repeat(64),
        target: expect.objectContaining({
          spec: expect.objectContaining({ duration_ms: 8_000 }),
        }),
      }),
    );
  });

  it("删除预检会把服务端证据 blocker 转成用户文案", async () => {
    const user = userEvent.setup();
    const source = transformSource(1);
    const onPreflight = vi.fn().mockResolvedValue({
      allowed: false,
      blockers: [
        {
          code: "SPEC_VERSION_EVIDENCE",
          summary: "Shot has immutable spec evidence",
        },
      ],
    });
    render(
      <DeleteShotDialog
        busy={false}
        shot={source.shot}
        onDelete={vi.fn()}
        onPreflight={onPreflight}
      />,
    );

    await user.click(screen.getByRole("button", { name: "删除检查" }));
    await user.click(screen.getByRole("button", { name: "检查删除条件" }));

    expect(
      await screen.findByText("镜头已有规格历史，只能归档"),
    ).toBeInTheDocument();
  });
});
