import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { StoryboardDrafts } from "@/app/studio/[episodeId]/storyboard-drafts";

const draft: API.DraftShotResponse = {
  id: "019ffb00-a000-7000-8000-000000000001",
  proposal_key: "opening",
  position: 1,
  title: "警报灯下的泵站",
  narrative_unit_version_ids: ["019ffb00-a000-7000-8000-000000000002"],
  spec: {
    schema_version: 1,
    script_reference: {
      confirmed_script_version_id: "019ffb00-a000-7000-8000-000000000003",
      scene_id: "019ffb00-a000-7000-8000-000000000004",
      dialogue_ids: [],
    },
    narrative: { purpose: "建立危机", continuity_note: null },
    visual: {
      shot_size: "wide",
      camera_angle: "eye_level",
      camera_movement: "dolly",
      composition: "倒计时与水位同框",
      environment: "夜间泵站",
      subject_placements: [],
      mood_lighting: "红色应急灯",
    },
    action_beats: [{ beat_key: "alarm", order: 1, description: "警报闪烁" }],
    dialogue_or_narration: [],
    duration_ms: 5_000,
    audio_intent: { ambient: "警报和水声", sound_effects: [] },
    generation_intent: {
      mode: "text_to_video",
      first_frame: null,
      last_frame: null,
      keyframe_notes: null,
    },
  },
  asset_references: [],
  risk_codes: ["continuity_review"],
  decision_history: [],
};

const batch: API.DraftBatchResponse = {
  id: "019ffb00-a000-7000-8000-000000000010",
  workspace_id: "019ffb00-a000-7000-8000-000000000011",
  project_id: "019ffb00-a000-7000-8000-000000000012",
  episode_id: "019ffb00-a000-7000-8000-000000000013",
  status: "needs_review",
  revision: 2,
  task_id: "019ffb00-a000-7000-8000-000000000014",
  input: {
    script_version_id: "019ffb00-a000-7000-8000-000000000003",
    narrative_structure_id: "019ffb00-a000-7000-8000-000000000015",
    narrative_revision: 1,
    narrative_dependency_hash: "a".repeat(64),
    narrative_unit_version_ids: ["019ffb00-a000-7000-8000-000000000002"],
    asset_state_ids: [],
    asset_version_ids: [],
    target_duration_ms: 5_000,
    aspect_ratio: "9:16",
    visual_style: null,
    input_hash: "b".repeat(64),
  },
  drafts: [draft],
  decision_summary: { pending: 1, accepted: 0, modified: 0, ignored: 0 },
  error_code: null,
  created_at: "2026-08-13T10:00:00Z",
  updated_at: "2026-08-13T10:00:00Z",
};

describe("StoryboardDrafts", () => {
  it("keeps AI drafts separate and records an explicit review decision", async () => {
    const user = userEvent.setup();
    const onDecide = vi.fn().mockResolvedValue(undefined);

    render(
      <StoryboardDrafts
        assetBible={undefined}
        batch={batch}
        busy={false}
        canCreate
        episodeId={batch.episode_id}
        onApply={vi.fn()}
        onApprove={vi.fn()}
        onCreate={vi.fn()}
        onDecide={onDecide}
        onPreflight={vi.fn()}
      />,
    );

    expect(screen.getByText("AI 分镜草案")).toBeInTheDocument();
    expect(screen.getByText(/正式镜头尚未写入/)).toBeInTheDocument();
    expect(screen.getByText(/警报灯下的泵站/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "接受此镜" }));

    expect(onDecide).toHaveBeenCalledWith(draft, "accepted", undefined);
  });

  it("selects ready asset states after the asset bible loads", async () => {
    const user = userEvent.setup();
    const onCreate = vi.fn().mockResolvedValue(undefined);
    const props = {
      batch: undefined,
      busy: false,
      canCreate: true,
      episodeId: batch.episode_id,
      onApply: vi.fn(),
      onApprove: vi.fn(),
      onCreate,
      onDecide: vi.fn(),
      onPreflight: vi.fn(),
    };
    const { rerender } = render(
      <StoryboardDrafts assetBible={undefined} {...props} />,
    );
    const stateId = "019ffb00-a000-7000-8000-000000000020";

    rerender(
      <StoryboardDrafts
        assetBible={{
          items: [
            {
              asset: {
                id: "019ffb00-a000-7000-8000-000000000021",
                workspace_id: batch.workspace_id,
                project_id: batch.project_id,
                kind: "character",
                name: "阿澜",
                aliases: [],
                tags: [],
                status: "active",
                availability: "enabled",
                name_revision: 1,
                revision: 1,
                created_at: batch.created_at,
                updated_at: batch.updated_at,
                warnings: [],
              },
              states: [
                {
                  state: {
                    id: stateId,
                    workspace_id: batch.workspace_id,
                    asset_id: "019ffb00-a000-7000-8000-000000000021",
                    state_key: "rainy_night",
                    label: "雨夜",
                    description: "雨夜造型",
                    status: "active",
                    current_version_id: "019ffb00-a000-7000-8000-000000000022",
                    revision: 1,
                    created_by: "019ffb00-a000-7000-8000-000000000024",
                    created_at: batch.created_at,
                    updated_at: batch.updated_at,
                  },
                  current_version: {
                    id: "019ffb00-a000-7000-8000-000000000022",
                    workspace_id: batch.workspace_id,
                    asset_id: "019ffb00-a000-7000-8000-000000000021",
                    asset_state_id: stateId,
                    version_no: 1,
                    schema_version: 1,
                    spec: {
                      kind: "character",
                      identity: "阿澜",
                      appearance: "雨夜造型",
                      age_impression: "",
                      temperament: [],
                      goals: [],
                      relationships: [],
                      arc_summary: "",
                      voice_profile: "",
                    },
                    prompt_description: "wet night costume",
                    source_type: "manual",
                    source_id: null,
                    content_hash: "d".repeat(64),
                    media_references: [],
                    created_by: "019ffb00-a000-7000-8000-000000000024",
                    created_at: batch.created_at,
                  },
                  occurrences: [
                    {
                      id: "019ffb00-a000-7000-8000-000000000023",
                      workspace_id: batch.workspace_id,
                      asset_state_id: stateId,
                      episode_id: batch.episode_id,
                      narrative_unit_id: "019ffb00-a000-7000-8000-000000000025",
                      narrative_unit_version_id: draft.narrative_unit_version_ids[0],
                      sequence: 1,
                      decision: "link",
                      origin: "manual",
                      evidence_hash: "e".repeat(64),
                      idempotency_key: "asset-occurrence-fixture",
                      freshness: "current",
                      created_by: "019ffb00-a000-7000-8000-000000000024",
                      created_at: batch.created_at,
                    },
                  ],
                  readiness: {
                    status: "ready",
                    blockers: [],
                    warnings: [],
                    next_actions: [],
                    dependency_snapshot: {
                      asset_state_id: stateId,
                      asset_state_revision: 1,
                      current_version_id: "019ffb00-a000-7000-8000-000000000022",
                      occurrence_decision_ids: [
                        "019ffb00-a000-7000-8000-000000000023",
                      ],
                      media_version_ids: [],
                      consent_ids: [],
                      evaluated_at: batch.updated_at,
                    },
                  },
                },
              ],
            },
          ],
          summary: {
            asset_count: 1,
            state_count: 1,
            ready: 1,
            draft: 0,
            blocked: 0,
            unavailable: 0,
          },
        }}
        {...props}
      />,
    );

    await user.click(screen.getByRole("button", { name: "生成待审核草案" }));

    expect(onCreate).toHaveBeenCalledWith([stateId]);
  });
});
