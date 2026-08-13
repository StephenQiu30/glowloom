import { fireEvent, render, screen, waitFor } from "@testing-library/react";
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
const characterVersionId = "019fb2c0-a000-7000-8000-000000000015";
const voiceVersionId = "019fb2c0-a000-7000-8000-000000000016";
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
      source_draft_shot_id: null,
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
      source_draft_shot_id: null,
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
        asset_state_id: "019fb2c0-a000-7000-8000-000000000021",
        asset_id: "019fb2c0-a000-7000-8000-000000000011",
        binding_source: "manual",
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
      warnings: [
        {
          code: "STYLE_REFERENCE_MISSING",
          field_path: "asset_references",
          summary: "Optional visual style reference is missing",
          next_action: "acknowledge_warning",
        },
      ],
      next_actions: [],
      evaluated_dependencies: {
        shot_spec_version_id: specVersionId,
        confirmed_script_version_id: scriptVersionId,
        current_script_version_id: scriptVersionId,
        narrative_structure_id: sceneId,
        narrative_structure_revision: 1,
        narrative_dependency_hash: "1".repeat(64),
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
        current_script_version_id: scriptVersionId,
        narrative_structure_id: sceneId,
        narrative_structure_revision: 1,
        narrative_dependency_hash: "1".repeat(64),
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
    availability: "enabled",
    name_revision: 1,
    revision: 1,
    created_at: now,
    updated_at: now,
    warnings: [],
  },
  {
    id: "019fb2c0-a000-7000-8000-000000000017",
    workspace_id: workspaceId,
    project_id: "019fb2c0-a000-7000-8000-000000000012",
    kind: "character",
    name: "顾清禾",
    aliases: [],
    tags: [],
    status: "active",
    availability: "enabled",
    name_revision: 1,
    revision: 1,
    created_at: now,
    updated_at: now,
    warnings: [],
  },
  {
    id: "019fb2c0-a000-7000-8000-000000000018",
    workspace_id: workspaceId,
    project_id: "019fb2c0-a000-7000-8000-000000000012",
    kind: "voice",
    name: "顾清禾声线",
    aliases: [],
    tags: [],
    status: "active",
    availability: "enabled",
    name_revision: 1,
    revision: 1,
    created_at: now,
    updated_at: now,
    warnings: [],
  },
];

function assetBibleState(
  asset: API.AssetResponse,
  versionId: string,
  ordinal: number,
): API.AssetBibleState {
  const stateId = `019fb2c0-a000-7000-8000-00000000002${ordinal}`;
  const spec: API.AssetVersionResponse["spec"] =
    asset.kind === "location"
      ? {
          kind: "location",
          spatial_description: "雨夜旧车站",
          time_weather: "夜间暴雨",
          visual_elements: ["旧灯箱"],
          lighting: "冷蓝侧逆光",
        }
      : asset.kind === "voice"
        ? {
            kind: "voice",
            source_kind: "synthetic_recording",
            language: "zh-CN",
            performance_traits: ["克制"],
            allowed_usage: ["preview"],
          }
        : {
            kind: "character",
            identity: asset.name,
            appearance: "固定外观",
            age_impression: "青年",
            temperament: ["克制"],
          };
  const state: API.AssetStateResponse = {
    id: stateId,
    workspace_id: workspaceId,
    asset_id: asset.id,
    state_key: "base",
    label: "基础状态",
    description: "",
    status: "active",
    current_version_id: versionId,
    revision: 1,
    created_by: workspaceId,
    created_at: now,
    updated_at: now,
  };
  return {
    state,
    current_version: {
      id: versionId,
      workspace_id: workspaceId,
      asset_id: asset.id,
      asset_state_id: stateId,
      version_no: 1,
      schema_version: 1,
      spec,
      prompt_description: "固定生产版本",
      source_type: "manual",
      source_id: null,
      content_hash: `${ordinal}`.repeat(64),
      media_references: [],
      created_by: workspaceId,
      created_at: now,
    },
    occurrences: [],
    readiness: {
      status: "draft",
      blockers: [],
      warnings: [],
      next_actions: [],
      dependency_snapshot: {
        asset_state_id: stateId,
        asset_state_revision: 1,
        current_version_id: versionId,
        occurrence_decision_ids: [],
        media_version_ids: [],
        consent_ids: [],
        evaluated_at: now,
      },
    },
  };
}

const assetBible: API.AssetBibleResponse = {
  items: assets.map((asset, index) => ({
    asset,
    states: [
      assetBibleState(
        asset,
        [locationVersionId, characterVersionId, voiceVersionId][index],
        index + 1,
      ),
    ],
  })),
  summary: {
    asset_count: 3,
    state_count: 3,
    ready: 0,
    draft: 3,
    blocked: 0,
    unavailable: 0,
  },
};

const acceptedShotCandidate: API.ExtractionCandidateResponse = {
  id: "019fb2c0-a000-7000-8000-000000000013",
  batch_id: "019fb2c0-a000-7000-8000-000000000014",
  candidate_key: "shot-001",
  kind: "shot",
  source_range: { start: 36, end: 52 },
  proposal: {
    kind: "shot",
    scene_candidate_key: "scene-001",
    title: "雨中回望",
    purpose: "交代角色发现远处来客",
  },
  confidence_note: "镜头意图明确",
  required: false,
  status: "accepted",
  revision: 2,
  created_at: now,
};

describe("分镜工作台", () => {
  it("展示服务端准备度并执行建镜、规格版本、顺序和生命周期操作", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(true);
    const onCreateFromCandidate = vi.fn().mockResolvedValue(true);
    const onSaveSpec = vi.fn().mockResolvedValue(true);
    const onReorder = vi.fn().mockResolvedValue(undefined);
    const onSelectShot = vi.fn();
    const onCopy = vi.fn().mockResolvedValue(undefined);
    const onDelete = vi.fn().mockResolvedValue(false);
    const onDeletePreflight = vi.fn().mockResolvedValue({
      allowed: false,
      blockers: [],
    });
    const onMerge = vi.fn().mockResolvedValue(false);
    const onMergePrepare = vi.fn().mockResolvedValue(undefined);
    const onSetCurrentSpec = vi.fn().mockResolvedValue(undefined);
    const onSplit = vi.fn().mockResolvedValue(false);
    const onSplitPreflight = vi.fn().mockResolvedValue(undefined);
    const onToggleArchived = vi.fn().mockResolvedValue(undefined);
    const onUpdate = vi.fn().mockResolvedValue(true);

    render(
      <StoryboardWorkspace
        archivedShots={archivedShots}
        assetBible={assetBible}
        busy={false}
        confirmedShotCandidates={[acceptedShotCandidate]}
        order={shots}
        readiness={readiness}
        selectedShotId={firstShotId}
        structure={structure}
        versions={versions}
        onCopy={onCopy}
        onCreate={onCreate}
        onCreateFromCandidate={onCreateFromCandidate}
        onDelete={onDelete}
        onDeletePreflight={onDeletePreflight}
        onMerge={onMerge}
        onMergePrepare={onMergePrepare}
        onReorder={onReorder}
        onSaveSpec={onSaveSpec}
        onSelectShot={onSelectShot}
        onSetCurrentSpec={onSetCurrentSpec}
        onSplit={onSplit}
        onSplitPreflight={onSplitPreflight}
        onToggleArchived={onToggleArchived}
        onUpdate={onUpdate}
      />,
    );

    expect(screen.getByRole("heading", { name: "分镜设计" })).toBeInTheDocument();
    expect(screen.getByText("2 个镜头")).toBeInTheDocument();
    expect(screen.getByText("1 可生成")).toBeInTheDocument();
    expect(screen.getByText("尚未保存镜头规格")).toBeInTheDocument();
    expect(screen.getByText("尚未固定可选的视觉风格参考")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "拆分" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "合并" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "删除检查" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "v1 · 当前" })).toBeDisabled();

    await user.click(screen.getByRole("button", { name: "修改镜头标题" }));
    await user.clear(screen.getByLabelText("镜头标题"));
    await user.type(screen.getByLabelText("镜头标题"), "雨夜车站全景");
    await user.click(screen.getByRole("button", { name: "保存标题" }));
    await waitFor(() =>
      expect(onUpdate).toHaveBeenCalledWith(shots.items[0], "雨夜车站全景"),
    );

    await user.clear(screen.getByLabelText("镜头目的"));
    await user.type(screen.getByLabelText("镜头目的"), "强调人物进入未知空间");
    await user.click(
      screen.getByRole("button", { name: "顾清禾 · 基础状态" }),
    );
    await user.clear(screen.getByLabelText("顾清禾画面位置"));
    await user.type(screen.getByLabelText("顾清禾画面位置"), "画面左侧，面向站台深处");
    await user.click(
      screen.getByRole("button", { name: "顾清禾声线 · 基础状态" }),
    );
    await user.click(
      screen.getByRole("button", { name: "为对白 顾清禾 选择声音 顾清禾声线" }),
    );
    await user.clear(screen.getByLabelText("顾清禾表演提示"));
    await user.type(screen.getByLabelText("顾清禾表演提示"), "克制而警觉");
    await user.type(screen.getByLabelText("首帧意图"), "雨幕中的空站台");
    await user.type(screen.getByLabelText("尾帧意图"), "角色停在灯箱前");
    await user.click(screen.getByRole("button", { name: "保存为新版本" }));
    await waitFor(() =>
      expect(onSaveSpec).toHaveBeenCalledWith(
        firstShotId,
        expect.objectContaining({
          expected_current_spec_version_id: specVersionId,
          spec: expect.objectContaining({
            narrative: expect.objectContaining({ purpose: "强调人物进入未知空间" }),
            visual: expect.objectContaining({
              subject_placements: [
                expect.objectContaining({ placement: "画面左侧，面向站台深处" }),
              ],
            }),
            dialogue_or_narration: [
              expect.objectContaining({
                source_dialogue_id: dialogueId,
                render_as_audio: true,
                speaker_subject_key: "subject-00000018",
                performance_note: "克制而警觉",
              }),
            ],
            generation_intent: expect.objectContaining({
              first_frame: "雨幕中的空站台",
              last_frame: "角色停在灯箱前",
            }),
          }),
          asset_references: expect.arrayContaining([
            expect.objectContaining({
              role: "character",
              asset_version_id: characterVersionId,
              subject_key: "subject-00000017",
            }),
            expect.objectContaining({
              role: "voice",
              asset_version_id: voiceVersionId,
              subject_key: "subject-00000018",
            }),
          ]),
        }),
      ),
    );
    const savedRequest = onSaveSpec.mock.calls[0]?.[1] as
      | API.ShotSpecCreateRequest
      | undefined;
    expect(savedRequest?.asset_references).toEqual([
      {
        slot_key: "location-main",
        role: "location",
        asset_version_id: locationVersionId,
        subject_key: null,
      },
      {
        slot_key: "character-00000017",
        role: "character",
        asset_version_id: characterVersionId,
        subject_key: "subject-00000017",
      },
      {
        slot_key: "voice-00000018",
        role: "voice",
        asset_version_id: voiceVersionId,
        subject_key: "subject-00000018",
      },
    ]);

    await user.click(screen.getByRole("button", { name: "下移镜头" }));
    expect(onReorder).toHaveBeenCalledWith([secondShotId, firstShotId]);

    fireEvent.dragStart(
      screen.getByRole("button", { name: "拖动镜头 建立雨夜车站" }),
    );
    const secondShotRow = screen.getByRole("listitem", {
      name: "镜头 角色走入画面 顺序项",
    });
    fireEvent.dragOver(secondShotRow);
    fireEvent.drop(secondShotRow);
    await waitFor(() => {
      expect(onReorder).toHaveBeenLastCalledWith([secondShotId, firstShotId]);
      expect(onSelectShot).toHaveBeenCalledWith(firstShotId);
    });

    await user.click(screen.getByRole("button", { name: "复制镜头" }));
    expect(onCopy).toHaveBeenCalledWith(shots.items[0]);
    await user.click(screen.getByRole("button", { name: "归档镜头" }));
    expect(onToggleArchived).toHaveBeenCalledWith(shots.items[0]);
    await user.click(screen.getByRole("button", { name: "恢复旧版转场" }));
    expect(onToggleArchived).toHaveBeenCalledWith(archivedShots[0]);

    await user.click(screen.getByRole("button", { name: "新建镜头" }));
    await user.click(
      screen.getByRole("button", { name: "从候选建立 雨中回望" }),
    );
    await waitFor(() =>
      expect(onCreateFromCandidate).toHaveBeenCalledWith(acceptedShotCandidate),
    );

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
  }, 10_000);

  it("在剧本结构尚未确认时解释为什么不能新建镜头", () => {
    render(
      <StoryboardWorkspace
        archivedShots={[]}
        assetBible={undefined}
        busy={false}
        confirmedShotCandidates={[]}
        order={{ items: [], order_hash: "a".repeat(64) }}
        selectedShotId={null}
        versions={[]}
        onCopy={vi.fn()}
        onCreate={vi.fn()}
        onCreateFromCandidate={vi.fn()}
        onDelete={vi.fn()}
        onDeletePreflight={vi.fn()}
        onMerge={vi.fn()}
        onMergePrepare={vi.fn()}
        onReorder={vi.fn()}
        onSaveSpec={vi.fn()}
        onSelectShot={vi.fn()}
        onSetCurrentSpec={vi.fn()}
        onSplit={vi.fn()}
        onSplitPreflight={vi.fn()}
        onToggleArchived={vi.fn()}
        onUpdate={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "新建镜头" })).toBeDisabled();
    expect(
      screen.getByText("需先确认剧本结构并设为当前版本，才能建立镜头。"),
    ).toBeInTheDocument();
  });

  it("允许显式把历史规格设为当前版本", async () => {
    const user = userEvent.setup();
    const historicalVersion: API.ShotSpecVersionResponse = {
      ...versions[0],
      id: "019fb2c0-a000-7000-8000-000000000099",
      version_no: 2,
      content_hash: "9".repeat(64),
      input_hash: "8".repeat(64),
    };
    const onSetCurrentSpec = vi.fn().mockResolvedValue(undefined);
    render(
      <StoryboardWorkspace
        archivedShots={[]}
        assetBible={assetBible}
        busy={false}
        confirmedShotCandidates={[]}
        order={{ ...shots, items: [shots.items[0]] }}
        readiness={{ ...readiness, items: [readiness.items[0]] }}
        selectedShotId={firstShotId}
        structure={structure}
        versions={[historicalVersion, versions[0]]}
        onCopy={vi.fn()}
        onCreate={vi.fn()}
        onCreateFromCandidate={vi.fn()}
        onDelete={vi.fn()}
        onDeletePreflight={vi.fn()}
        onMerge={vi.fn()}
        onMergePrepare={vi.fn()}
        onReorder={vi.fn()}
        onSaveSpec={vi.fn()}
        onSelectShot={vi.fn()}
        onSetCurrentSpec={onSetCurrentSpec}
        onSplit={vi.fn()}
        onSplitPreflight={vi.fn()}
        onToggleArchived={vi.fn()}
        onUpdate={vi.fn()}
      />,
    );

    await user.click(screen.getByRole("button", { name: "v2 · 设为当前" }));
    expect(onSetCurrentSpec).toHaveBeenCalledWith(
      shots.items[0],
      historicalVersion,
    );
  });
});
