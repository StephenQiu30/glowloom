import { describe, expect, it } from "vitest";

import {
  buildMergeTarget,
  buildSplitTargets,
} from "@/app/studio/[episodeId]/storyboard-shot-operations";

const scriptVersionId = "019fb6c0-a000-7000-8000-000000000001";
const firstSceneId = "019fb6c0-a000-7000-8000-000000000002";
const secondSceneId = "019fb6c0-a000-7000-8000-000000000003";
const firstDialogueId = "019fb6c0-a000-7000-8000-000000000004";
const secondDialogueId = "019fb6c0-a000-7000-8000-000000000005";
const locationVersionId = "019fb6c0-a000-7000-8000-000000000006";
const locationReference: API.AssetReferenceRequest = {
  slot_key: "location-main",
  role: "location",
  asset_version_id: locationVersionId,
  subject_key: null,
};

function version({
  id,
  sceneId,
  dialogueId,
  duration,
  purpose,
}: {
  id: string;
  sceneId: string;
  dialogueId: string;
  duration: number;
  purpose: string;
}): API.ShotSpecVersionResponse {
  return {
    id,
    workspace_id: "019fb6c0-a000-7000-8000-000000000010",
    shot_id: "019fb6c0-a000-7000-8000-000000000011",
    version_no: 1,
    schema_version: 1,
    spec: {
      schema_version: 1,
      script_reference: {
        confirmed_script_version_id: scriptVersionId,
        scene_id: sceneId,
        dialogue_ids: [dialogueId],
      },
      narrative: { purpose, continuity_note: "保持轴线连续" },
      visual: {
        shot_size: "medium",
        camera_angle: "eye_level",
        camera_movement: "static",
        composition: `${purpose}构图`,
        environment: `${purpose}环境`,
        subject_placements: [],
        mood_lighting: "冷蓝侧光",
      },
      action_beats: [
        { beat_key: "beat-1", order: 1, description: `${purpose}动作` },
      ],
      dialogue_or_narration: [
        {
          source_dialogue_id: dialogueId,
          beat_key: "beat-1",
          speaker_subject_key: null,
          render_as_audio: false,
          performance_note: null,
        },
      ],
      duration_ms: duration,
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
    asset_references: [
      {
        slot_key: "location-main",
        role: "location",
        asset_version_id: locationVersionId,
        asset_state_id: "019fb6c0-a000-7000-8000-000000000013",
        asset_id: "019fb6c0-a000-7000-8000-000000000014",
        binding_source: "manual",
        subject_key: null,
      },
    ],
    created_by: "019fb6c0-a000-7000-8000-000000000012",
    created_at: "2026-07-31T12:00:00Z",
  };
}

describe("分镜变换目标构造", () => {
  it("拆分保持总时长、固定资产，并把对白只分配到一个目标", () => {
    const source = version({
      id: "019fb6c0-a000-7000-8000-000000000020",
      sceneId: firstSceneId,
      dialogueId: firstDialogueId,
      duration: 4_000,
      purpose: "主角进入车站",
    });

    const targets = buildSplitTargets(source, {
      firstTitle: "主角进入",
      secondTitle: "车站空镜",
      firstDurationMs: 2_200,
    });

    expect(targets.map((target) => target.spec.duration_ms)).toEqual([
      2_200,
      1_800,
    ]);
    expect(targets[0].spec.script_reference.dialogue_ids).toEqual([
      firstDialogueId,
    ]);
    expect(targets[1].spec.script_reference.dialogue_ids).toEqual([]);
    expect(targets[1].spec.dialogue_or_narration).toEqual([]);
    expect(targets[0].asset_references).toEqual([locationReference]);
    expect(targets[1].asset_references).toEqual([locationReference]);
  });

  it("同场次合并连续动作与对白，并保持总时长", () => {
    const first = version({
      id: "019fb6c0-a000-7000-8000-000000000021",
      sceneId: firstSceneId,
      dialogueId: firstDialogueId,
      duration: 4_000,
      purpose: "主角进入车站",
    });
    const second = version({
      id: "019fb6c0-a000-7000-8000-000000000022",
      sceneId: firstSceneId,
      dialogueId: secondDialogueId,
      duration: 3_000,
      purpose: "灯箱突然闪烁",
    });

    const target = buildMergeTarget(first, second, {
      baseVersionId: first.id,
      title: "进入车站并发现灯箱",
    });

    expect(target.title).toBe("进入车站并发现灯箱");
    expect(target.spec.duration_ms).toBe(7_000);
    expect(target.spec.action_beats.map((beat) => beat.order)).toEqual([1, 2]);
    expect(target.spec.script_reference.dialogue_ids).toEqual([
      firstDialogueId,
      secondDialogueId,
    ]);
    expect(target.spec.dialogue_or_narration).toHaveLength(2);
  });

  it("跨场次合并时以用户选择的规格为场次与资产基础", () => {
    const first = version({
      id: "019fb6c0-a000-7000-8000-000000000023",
      sceneId: firstSceneId,
      dialogueId: firstDialogueId,
      duration: 4_000,
      purpose: "车站内景",
    });
    const second = version({
      id: "019fb6c0-a000-7000-8000-000000000024",
      sceneId: secondSceneId,
      dialogueId: secondDialogueId,
      duration: 3_000,
      purpose: "隧道出口",
    });

    const target = buildMergeTarget(first, second, {
      baseVersionId: second.id,
      title: "穿过隧道",
    });

    expect(target.spec.script_reference.scene_id).toBe(secondSceneId);
    expect(target.spec.script_reference.dialogue_ids).toEqual([
      secondDialogueId,
    ]);
    expect(target.asset_references).toEqual([locationReference]);
  });
});
