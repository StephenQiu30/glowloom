from hashlib import sha256
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.modules.storyboards.drafts.provider_schema import (
    ProviderShot,
    StoryboardProviderResult,
)

AgentStage = Literal[
    "contexts_built",
    "source_analyzed",
    "scenes_planned",
    "shots_drafted",
    "repaired",
    "hard_gates_passed",
    "reviewed",
    "final_gate_passed",
    "failed",
]


class AgentModel(BaseModel):
    model_config = ConfigDict(extra="forbid", frozen=True)


class SceneContextUnit(AgentModel):
    unit_version_id: UUID
    position: int = Field(ge=1, le=10_000)
    kind: Literal["scene_heading", "action", "dialogue", "narration"]
    exact_text: str = Field(min_length=1)
    required_for_coverage: bool
    source_dialogue_id: UUID | None = None


class SceneContextAsset(AgentModel):
    asset_version_id: UUID
    position: int = Field(ge=1, le=10_000)
    kind: str = Field(min_length=1)
    name: str = Field(min_length=1)
    state_label: str = Field(min_length=1)


class SceneContext(AgentModel):
    scene_key: int = Field(ge=1)
    scene_id: UUID
    target_duration_ms: int = Field(ge=500)
    aspect_ratio: Literal["9:16", "16:9", "1:1"]
    visual_style: str | None = None
    units: tuple[SceneContextUnit, ...] = Field(min_length=1)
    assets: tuple[SceneContextAsset, ...] = ()
    world_facts: tuple[str, ...] = ()
    world_rules: tuple[str, ...] = ()

    @model_validator(mode="after")
    def validate_positions(self) -> "SceneContext":
        unit_positions = [unit.position for unit in self.units]
        if len(set(unit_positions)) != len(unit_positions):
            raise ValueError("scene context unit positions must be unique")
        asset_positions = [asset.position for asset in self.assets]
        if len(set(asset_positions)) != len(asset_positions):
            raise ValueError("scene context asset positions must be unique")
        return self


class SourceBeat(AgentModel):
    beat_key: str = Field(min_length=1, max_length=100)
    unit_positions: tuple[int, ...] = Field(min_length=1)
    dramatic_function: Literal[
        "setup",
        "conflict",
        "action",
        "dialogue",
        "reaction",
        "reveal",
        "payoff",
        "transition",
    ]
    summary: str = Field(min_length=1, max_length=1_000)

    @model_validator(mode="after")
    def validate_unit_positions(self) -> "SourceBeat":
        if len(set(self.unit_positions)) != len(self.unit_positions):
            raise ValueError("source beat unit positions must be unique")
        return self


class SceneAnalysis(AgentModel):
    scene_key: int = Field(ge=1)
    beats: tuple[SourceBeat, ...] = Field(min_length=1)
    conflict: str | None = Field(default=None, max_length=1_000)
    reveal: str | None = Field(default=None, max_length=1_000)
    reaction: str | None = Field(default=None, max_length=1_000)
    continuity_facts: tuple[str, ...] = Field(default=(), max_length=32)

    @model_validator(mode="after")
    def validate_beat_keys(self) -> "SceneAnalysis":
        beat_keys = [beat.beat_key for beat in self.beats]
        if len(set(beat_keys)) != len(beat_keys):
            raise ValueError("source beat keys must be unique")
        return self


class ShotSeed(AgentModel):
    seed_key: str = Field(min_length=1, max_length=100)
    beat_keys: tuple[str, ...] = Field(min_length=1)
    unit_positions: tuple[int, ...] = Field(min_length=1)
    purpose: str = Field(min_length=1, max_length=500)
    suggested_duration_ms: int = Field(ge=500, le=15_000)

    @model_validator(mode="after")
    def validate_references(self) -> "ShotSeed":
        if len(set(self.beat_keys)) != len(self.beat_keys):
            raise ValueError("shot seed beat keys must be unique")
        if len(set(self.unit_positions)) != len(self.unit_positions):
            raise ValueError("shot seed unit positions must be unique")
        return self


class ScenePlan(AgentModel):
    scene_key: int = Field(ge=1)
    spatial_axis: str = Field(min_length=1, max_length=1_000)
    movement_direction: str = Field(min_length=1, max_length=1_000)
    eyeline: str = Field(min_length=1, max_length=1_000)
    entrances_exits: tuple[str, ...] = Field(default=(), max_length=32)
    prop_states: tuple[str, ...] = Field(default=(), max_length=32)
    rhythm: str = Field(min_length=1, max_length=1_000)
    duration_budget_ms: int = Field(ge=500)
    shot_seeds: tuple[ShotSeed, ...] = Field(min_length=1, max_length=120)

    @model_validator(mode="after")
    def validate_seed_keys(self) -> "ScenePlan":
        seed_keys = [seed.seed_key for seed in self.shot_seeds]
        if len(set(seed_keys)) != len(seed_keys):
            raise ValueError("shot seed keys must be unique")
        return self


class ReviewIssue(AgentModel):
    issue_id: str = Field(min_length=1, max_length=160)
    code: str = Field(min_length=1, max_length=120)
    severity: Literal["blocker", "warning"]
    scope: Literal["global", "scene", "shot"]
    scene_key: int | None = Field(default=None, ge=1)
    shot_positions: tuple[int, ...] = Field(default=())
    evidence: str = Field(min_length=1, max_length=2_000)
    repair_hint: str | None = Field(default=None, max_length=2_000)
    source: Literal["tool", "reviewer"]

    @model_validator(mode="after")
    def validate_scope(self) -> "ReviewIssue":
        if self.scope == "global":
            if self.scene_key is not None or self.shot_positions:
                raise ValueError("global issue cannot include a scene or shot positions")
        elif self.scope == "scene":
            if self.scene_key is None or self.shot_positions:
                raise ValueError("scene issue requires scene_key and no shot positions")
        elif self.scene_key is None or not self.shot_positions:
            raise ValueError("shot issue requires scene_key and shot positions")
        if len(set(self.shot_positions)) != len(self.shot_positions):
            raise ValueError("review issue shot positions must be unique")
        return self


class StoryboardReview(AgentModel):
    issues: tuple[ReviewIssue, ...] = Field(default=(), max_length=500)


class SceneDraft(AgentModel):
    scene_key: int = Field(ge=1)
    result: StoryboardProviderResult


class StoryboardTimelineShot(AgentModel):
    scene_key: int = Field(ge=1)
    scene_id: UUID
    local_position: int = Field(ge=1)
    global_position: int = Field(ge=1)
    timecode_in_ms: int = Field(ge=0)
    timecode_out_ms: int = Field(ge=500)
    original_proposal_key: str = Field(min_length=1, max_length=120)
    shot: ProviderShot

    @model_validator(mode="after")
    def validate_timecodes(self) -> "StoryboardTimelineShot":
        if self.timecode_out_ms - self.timecode_in_ms != self.shot.duration_ms:
            raise ValueError("timeline duration must match shot duration")
        return self


class AssembledStoryboard(AgentModel):
    candidate: StoryboardProviderResult
    timeline: tuple[StoryboardTimelineShot, ...] = Field(min_length=1)
    total_duration_ms: int = Field(ge=500)
    result_hash: str = Field(pattern=r"^[0-9a-f]{64}$")

    def has_consistent_payload(self) -> bool:
        if len(self.candidate.shots) != len(self.timeline):
            return False
        expected_timecode_ms = 0
        for position, (candidate_shot, row) in enumerate(
            zip(self.candidate.shots, self.timeline, strict=True),
            start=1,
        ):
            if (
                row.global_position != position
                or row.shot != candidate_shot
                or row.timecode_in_ms != expected_timecode_ms
            ):
                return False
            expected_timecode_ms = row.timecode_out_ms
        expected_hash = sha256(self.candidate.model_dump_json().encode("utf-8")).hexdigest()
        return expected_timecode_ms == self.total_duration_ms and self.result_hash == expected_hash

    @model_validator(mode="after")
    def validate_payload_consistency(self) -> "AssembledStoryboard":
        if not self.has_consistent_payload():
            raise ValueError(
                "assembled storyboard candidate, timeline, duration and hash must agree"
            )
        return self


class StoryboardCheckpoint(AgentModel):
    batch_id: UUID
    task_id: UUID
    run_token: UUID | None = None
    harness_version: str = Field(min_length=1)
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    stage: AgentStage
    stage_attempt: int = Field(ge=1)
    status: Literal["running", "completed", "failed"]
    repair_round: int = Field(ge=0, le=2)
    scene_contexts: tuple[SceneContext, ...] = ()
    analyses: tuple[SceneAnalysis, ...] = ()
    plans: tuple[ScenePlan, ...] = ()
    scene_drafts: tuple[SceneDraft, ...] = ()
    issues: tuple[ReviewIssue, ...] = ()
    assembled: AssembledStoryboard | None = None

    @model_validator(mode="after")
    def validate_terminal_status(self) -> "StoryboardCheckpoint":
        if self.stage == "final_gate_passed" and self.status != "completed":
            raise ValueError("final checkpoint must be completed")
        return self


class StoryboardAgentRunResult(AgentModel):
    status: Literal["needs_review", "failed"]
    candidate_only: Literal[True] = True
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    result_hash: str | None = Field(default=None, pattern=r"^[0-9a-f]{64}$")
    candidate: StoryboardProviderResult | None = None
    timeline: tuple[StoryboardTimelineShot, ...] = ()
    issues: tuple[ReviewIssue, ...] = ()
    repair_rounds: int = Field(ge=0, le=2)
    skill_versions: dict[str, str]
    checkpoints_saved: int = Field(ge=0)

    @model_validator(mode="after")
    def validate_outcome(self) -> "StoryboardAgentRunResult":
        if self.status == "needs_review":
            if self.candidate is None or self.result_hash is None or not self.timeline:
                raise ValueError("successful harness result requires a candidate and timeline")
        elif self.candidate is not None or self.result_hash is not None or self.timeline:
            raise ValueError("failed harness result cannot expose a candidate")
        return self
