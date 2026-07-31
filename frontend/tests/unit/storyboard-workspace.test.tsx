import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { StoryboardWorkspace } from "@/app/studio/[episodeId]/storyboard-workspace";

const workspaceId = "019fb2c0-a000-7000-8000-000000000001";
const episodeId = "019fb2c0-a000-7000-8000-000000000002";
const scriptVersionId = "019fb2c0-a000-7000-8000-000000000003";
const sceneId = "019fb2c0-a000-7000-8000-000000000004";
const dialogueId = "019fb2c0-a000-7000-8000-000000000005";
const firstShotId = "019fb2c0-a000-7000-8000-000000000006";
const secondShotId = "019fb2c0-a000-7000-8000-000000000007";
const archivedShotId = "019fb2c0-a000-7000-8000-000000000008";
const specVersionId = "019fb2c0-a000-7000-8000-000000000009";
const locationVersionId = "019fb2c0-a000-7000-8000-000000000010";
const now = "2026-07-31T10:00:00Z";

const shots: API.ShotOrderResponse = {
  order_hash: "a".repeat(64),
  items: [
    {
      id: firstShotId,
      workspace_id: workspaceId,
      episode_id: episodeId,
      position: 1,
      title: "建立雨夜车站",
      source_script_version_id: scriptVersionId,
      source_scene_id: sceneId,
      source_candidate_id: null,
      status: "active",
      current_spec_version_id: specVersionId,
      revision: 2,
      created_at: now,
      updated_at: now,
    },
    {
      id: secondShotId,
      workspace_id: workspaceId,
      episode_id: episodeId,
      position: 2,
      title: "角色走入画面",
      source_script_version_id: scriptVersionId,
      source_scene_id: sceneId,
      source_candidate_id: null,
      status: "active",
      current_spec_version_id: null,
      revision: 1,
      created_at: now,
      updated_at: now,
    },
  ],
};

const archivedShots: API.ShotResponse[] = [
  {
    ...shots.items[1],
    id: archivedShotId,
    position: 0,
    title: "旧版转场",
    status: "archived",
    revision: 3,
  },
];

const structure: API.ConfirmedStructureResponse = {
  script_version_id: scriptVersionId,
  scenes: [
    {
      id: sceneId,
      script_version_id: scriptVersionId,
      position: 1,
      heading: "第一场 · 雨夜车站",
      location: "旧车站月台",
      time_of_day: "夜",
      summary: "顾清禾走入空无一人的月台",
      source_range: { start: 0, end: 20 },
      dialogues: [
        {
          id: dialogueId,
          scene_id: sceneId,
          position: 1,
          speaker_candidate: "顾清禾",
          dialogue_kind: "spoken",
          text: "你终于来了。",
          performance_note: "压低声音",
          source_range: { start: 21, end: 35 },
          created_at: now,
        },
      ],
      created_at: now,
    },
  ],
};

const versions: API.ShotSpecVersionResponse[] = [
  {
    id: specVersionId,
    workspace_id: workspaceId,
    shot_id: firstShotId,
    version_no: 1,
    schema_version: 1,
    spec: {
      schema_version: 1,
      script_reference: {
        confirmed_script_version_id: scriptVersionId,
        scene_id: sceneId,
        dialogue_ids: [dialogueId],
      },
      narrative: { purpose: "建立空间关系", continuity_note: null },
      visual: {
        shot_size: "wide",
        camera_angle: "eye_level",
        camera_movement: "dolly",
        composition: "人物从画面右侧进入，站台延伸至远处",
        environment: "雨夜的旧车站月台",
        subject_placements: [],
        mood_lighting: "冷蓝色侧逆光",
      },
      action_beats: [
        { beat_key: "beat-1", order: 1, description: "人物走入月台" },
      ],
      dialogue_or_narration: [
        {
          source_dialogue_id: dialogueId,
          beat_key: "beat-1",
          speaker_subject_key: null,
          render_as_audio: false,
          performance_note: "压低声音",
        },
      ],
      duration_ms: 4_000,
      audio_intent: { ambient: "雨声与远处列车声", sound_effects: [] },
      generation_intent: {
        mode: "text_to_video",
        first_frame: null,
        last_frame: null,
        keyframe_notes: "保持人物方向连续",
      },
    },
    content_hash: "b".repeat(64),
    input_hash: "c".repeat(64),
    asset_references: [
      {
        slot_key: "location-main",
        role: "location",
        asset_version_id: locationVersionId,
        subject_key: null,
      },
    ],
    created_by: workspaceId,
    created_at: now,
  },
];

const readiness: API.ShotReadinessBatchResponse = {
  episode_id: episodeId,
  summary: { total: 2, ready: 1, blocked: 1, unavailable: 0 },
  evaluation_hash: "d".repeat(64),
  items: [
    {
      shot_id: firstShotId,
      status: "ready",
      ready: true,
      blocking_reasons: [],
      warnings: [],
      next_actions: [],
      evaluated_dependencies: {
        shot_spec_version_id: specVersionId,
        confirmed_script_version_id: scriptVersionId,
        scene_id: sceneId,
        dialogue_ids: [dialogueId],
        asset_version_ids: [locationVersionId],
        media_version_ids: [],
        consent_ids: [],
        asset_evaluation_hashes: {},
      },
      evaluation_hash: "e".repeat(64),
    },
    {
      shot_id: secondShotId,
      status: "blocked",
      ready: false,
      blocking_reasons: [
        {
          code: "CURRENT_SPEC_MISSING",
          field_path: null,
          dependency_type: null,
          dependency_id: null,
          summary: "Current shot spec is missing",
          next_action: "save_shot_spec",
        },
      ],
      warnings: [],
      next_actions: ["save_shot_spec"],
      evaluated_dependencies: {
        shot_spec_version_id: null,
        confirmed_script_version_id: scriptVersionId,
        scene_id: sceneId,
        dialogue_ids: [],
        asset_version_ids: [],
        media_version_ids: [],
        consent_ids: [],
        asset_evaluation_hashes: {},
      },
      evaluation_hash: "f".repeat(64),
    },
  ],
};

const assets: API.AssetResponse[] = [
  {
    id: "019fb2c0-a000-7000-8000-000000000011",
    workspace_id: workspaceId,
    project_id: "019fb2c0-a000-7000-8000-000000000012",
    kind: "location",
    name: "雨夜旧车站",
    aliases: [],
    tags: [],
    status: "active",
    current_version_id: locationVersionId,
    revision: 1,
    created_at: now,
    updated_at: now,
    warnings: [],
  },
];

describe("分镜工作台", () => {
  it("展示服务端准备度并执行建镜、规格版本、顺序和生命周期操作", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(true);
    const onSaveSpec = vi.fn().mockResolvedValue(true);
    const onReorder = vi.fn().mockResolvedValue(undefined);
    const onCopy = vi.fn().mockResolvedValue(undefined);
    const onToggleArchived = vi.fn().mockResolvedValue(undefined);

    render(
      <StoryboardWorkspace
        archivedShots={archivedShots}
        assets={assets}
        busy={false}
        order={shots}
        readiness={readiness}
        selectedShotId={firstShotId}
        structure={structure}
        versions={versions}
        onCopy={onCopy}
        onCreate={onCreate}
        onReorder={onReorder}
        onSaveSpec={onSaveSpec}
        onSelectShot={vi.fn()}
        onToggleArchived={onToggleArchived}
      />,
    );

    expect(screen.getByRole("heading", { name: "分镜设计" })).toBeInTheDocument();
    expect(screen.getByText("2 个镜头")).toBeInTheDocument();
    expect(screen.getByText("1 可生成")).toBeInTheDocument();
    expect(screen.getByText("尚未保存镜头规格")).toBeInTheDocument();

    await user.clear(screen.getByLabelText("镜头目的"));
    await user.type(screen.getByLabelText("镜头目的"), "强调人物进入未知空间");
    await user.click(screen.getByRole("button", { name: "保存为新版本" }));
    await waitFor(() =>
      expect(onSaveSpec).toHaveBeenCalledWith(
        firstShotId,
        expect.objectContaining({
          expected_current_spec_version_id: specVersionId,
          spec: expect.objectContaining({
            narrative: expect.objectContaining({ purpose: "强调人物进入未知空间" }),
          }),
        }),
      ),
    );

    await user.click(screen.getByRole("button", { name: "下移镜头" }));
    expect(onReorder).toHaveBeenCalledWith([secondShotId, firstShotId]);
    await user.click(screen.getByRole("button", { name: "复制镜头" }));
    expect(onCopy).toHaveBeenCalledWith(shots.items[0]);
    await user.click(screen.getByRole("button", { name: "归档镜头" }));
    expect(onToggleArchived).toHaveBeenCalledWith(shots.items[0]);
    await user.click(screen.getByRole("button", { name: "恢复旧版转场" }));
    expect(onToggleArchived).toHaveBeenCalledWith(archivedShots[0]);

    await user.click(screen.getByRole("button", { name: "新建镜头" }));
    await user.type(screen.getByLabelText("新镜头标题"), "远处灯箱闪烁");
    await user.click(screen.getByRole("button", { name: "创建空镜头" }));
    await waitFor(() =>
      expect(onCreate).toHaveBeenCalledWith(
        expect.objectContaining({
          title: "远处灯箱闪烁",
          source_script_version_id: scriptVersionId,
          source_scene_id: sceneId,
        }),
      ),
    );
  });

  it("在剧本结构尚未确认时解释为什么不能新建镜头", () => {
    render(
      <StoryboardWorkspace
        archivedShots={[]}
        assets={[]}
        busy={false}
        order={{ items: [], order_hash: "a".repeat(64) }}
        selectedShotId={null}
        versions={[]}
        onCopy={vi.fn()}
        onCreate={vi.fn()}
        onReorder={vi.fn()}
        onSaveSpec={vi.fn()}
        onSelectShot={vi.fn()}
        onToggleArchived={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "新建镜头" })).toBeDisabled();
    expect(
      screen.getByText("需先确认剧本结构并设为当前版本，才能建立镜头。"),
    ).toBeInTheDocument();
  });
});
