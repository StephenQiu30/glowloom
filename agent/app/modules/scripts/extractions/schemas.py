from datetime import datetime
from typing import Annotated, Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.modules.production import TaskResponse, TaskStatus


class CommandModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class ScriptExtractionRequest(CommandModel):
    scope: Literal["full"]
    idempotency_key: str = Field(min_length=1, max_length=200)
    production_bible_id: UUID | None = None


class ExtractionBatchResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    script_version_id: UUID
    scope: Literal["full"]
    extractor_version: str
    input_hash: str
    production_bible_id: UUID | None
    production_bible_revision: int | None
    production_bible_result_hash: str | None
    status: TaskStatus
    confirmed_script_version_id: UUID | None
    candidate_count: int
    task: TaskResponse
    created_at: datetime


CandidateKind = Literal["scene", "dialogue", "asset", "asset_occurrence", "shot", "continuity"]
CandidateStatus = Literal["pending", "accepted", "linked", "merged", "ignored"]


class CandidateSourceRange(CommandModel):
    start: int = Field(ge=0)
    end: int = Field(ge=1)

    @model_validator(mode="after")
    def validate_order(self) -> "CandidateSourceRange":
        if self.end <= self.start:
            raise ValueError("source range end must be greater than start")
        return self


class SceneProductionTask(CommandModel):
    task_type: Literal[
        "asset_prepare",
        "shot_breakdown",
        "continuity_review",
        "voice_prepare",
    ]
    title: str = Field(min_length=1, max_length=200)
    objective: str = Field(min_length=1, max_length=1000)
    priority: Literal["low", "normal", "high", "blocking"] = "normal"


class SceneCandidateProposal(CommandModel):
    kind: Literal["scene"]
    heading: str = Field(min_length=1, max_length=200)
    location: str = Field(min_length=1, max_length=200)
    time_of_day: str = Field(min_length=1, max_length=100)
    summary: str = Field(min_length=1, max_length=2000)
    episode_number: int | None = Field(default=None, ge=1)
    scene_number: int | None = Field(default=None, ge=1, le=1000)
    story_beat: str | None = Field(default=None, max_length=1000)
    characters: list[str] = Field(default_factory=list, max_length=50)
    props: list[str] = Field(default_factory=list, max_length=100)
    environment_details: str | None = Field(default=None, max_length=2000)
    continuity_notes: list[str] = Field(default_factory=list, max_length=50)
    production_tasks: list[SceneProductionTask] = Field(
        default_factory=lambda: list[SceneProductionTask](), max_length=20
    )


class DialogueCandidateProposal(CommandModel):
    kind: Literal["dialogue"]
    scene_candidate_key: str = Field(min_length=1, max_length=100)
    speaker_candidate: str = Field(min_length=1, max_length=200)
    dialogue_kind: Literal["spoken", "narration", "internal", "voice_over"]
    text: str = Field(min_length=1, max_length=4000)
    performance_note: str | None = Field(default=None, max_length=1000)
    emotion: str | None = Field(default=None, max_length=200)
    action_before: str | None = Field(default=None, max_length=1000)
    subtext: str | None = Field(default=None, max_length=1000)


class AssetCandidateProposal(CommandModel):
    kind: Literal["asset"]
    asset_kind: Literal["character", "location", "prop", "costume", "visual_style", "voice"]
    name: str = Field(min_length=1, max_length=200)
    description: str = Field(min_length=1, max_length=2000)
    aliases: list[str] = Field(default_factory=list, max_length=20)
    role: str | None = Field(default=None, max_length=500)
    visual_identity: str | None = Field(default=None, max_length=2000)
    appearance: str | None = Field(default=None, max_length=2000)
    voice_profile: str | None = Field(default=None, max_length=1000)
    goals: list[str] = Field(default_factory=list, max_length=50)
    relationships: list[str] = Field(default_factory=list, max_length=100)
    arc_summary: str | None = Field(default=None, max_length=2000)
    continuity_notes: list[str] = Field(default_factory=list, max_length=50)
    first_seen_episode: int | None = Field(default=None, ge=1)
    episode_numbers: list[int] = Field(default_factory=lambda: list[int](), max_length=1000)


class AssetOccurrenceCandidateProposal(CommandModel):
    kind: Literal["asset_occurrence"]
    entity_key: str = Field(min_length=1, max_length=100)
    state_key: str = Field(min_length=1, max_length=80)
    scene_candidate_key: str = Field(min_length=1, max_length=100)
    role: Literal["location", "character", "prop", "costume", "visual_style", "voice"]


class ShotCandidateProposal(CommandModel):
    kind: Literal["shot"]
    scene_candidate_key: str = Field(min_length=1, max_length=100)
    title: str = Field(min_length=1, max_length=200)
    purpose: str = Field(min_length=1, max_length=2000)
    shot_number: int | None = Field(default=None, ge=1, le=1000)
    shot_type: str | None = Field(default=None, max_length=100)
    framing: str | None = Field(default=None, max_length=300)
    camera_movement: str | None = Field(default=None, max_length=300)
    action: str | None = Field(default=None, max_length=2000)
    visual_prompt: str | None = Field(default=None, max_length=3000)
    dialogue_excerpt: str | None = Field(default=None, max_length=1000)
    asset_names: list[str] = Field(default_factory=list, max_length=50)
    duration_ms: int | None = Field(default=None, ge=500, le=15_000)
    continuity_notes: list[str] = Field(default_factory=list, max_length=50)


class ContinuityCandidateProposal(CommandModel):
    kind: Literal["continuity"]
    scope: Literal["scene", "episode", "character", "world"] = "scene"
    severity: Literal["info", "warning", "blocking"]
    issue: str = Field(min_length=1, max_length=2000)
    suggestion: str = Field(min_length=1, max_length=2000)
    episode_number: int | None = Field(default=None, ge=1)
    entities: list[str] = Field(default_factory=list, max_length=50)
    evidence: str | None = Field(default=None, max_length=2000)
    topic: str | None = Field(default=None, max_length=200)
    title: str | None = Field(default=None, max_length=200)
    logline: str | None = Field(default=None, max_length=1000)
    summary: str | None = Field(default=None, max_length=3000)
    facts: list[str] = Field(default_factory=list, max_length=100)
    rules: list[str] = Field(default_factory=list, max_length=100)
    scene_candidate_key: str | None = Field(default=None, max_length=100)
    scene_candidate_keys: list[str] = Field(default_factory=list, max_length=1000)


CandidateProposal = Annotated[
    SceneCandidateProposal
    | DialogueCandidateProposal
    | AssetCandidateProposal
    | AssetOccurrenceCandidateProposal
    | ShotCandidateProposal
    | ContinuityCandidateProposal,
    Field(discriminator="kind"),
]


class AcceptNewDecision(CommandModel):
    action: Literal["accept_new"]


class AcceptWithChangesDecision(CommandModel):
    action: Literal["accept_with_changes"]
    proposal: CandidateProposal


class MergeIntoDecision(CommandModel):
    action: Literal["merge_into"]
    target_candidate_id: UUID


class LinkExistingDecision(CommandModel):
    action: Literal["link_existing"]
    downstream_id: UUID


class IgnoreDecision(CommandModel):
    action: Literal["ignore"]


CandidateDecisionCommand = Annotated[
    AcceptNewDecision
    | AcceptWithChangesDecision
    | LinkExistingDecision
    | MergeIntoDecision
    | IgnoreDecision,
    Field(discriminator="action"),
]


class CandidateDecisionRequest(CommandModel):
    decision_key: str = Field(min_length=1, max_length=200)
    expected_revision: int = Field(ge=1)
    decision: CandidateDecisionCommand


class ExtractionCandidateInput(CommandModel):
    candidate_key: str = Field(min_length=1, max_length=100)
    source_range: CandidateSourceRange
    proposal: CandidateProposal
    confidence_note: str | None = Field(default=None, max_length=1000)


class ScriptExtractionResult(CommandModel):
    candidates: list[ExtractionCandidateInput]

    @model_validator(mode="after")
    def validate_candidate_references(self) -> "ScriptExtractionResult":
        candidates_by_key = {candidate.candidate_key: candidate for candidate in self.candidates}
        if len(candidates_by_key) != len(self.candidates):
            raise ValueError("candidate keys must be unique")
        for candidate in self.candidates:
            proposal = candidate.proposal
            if isinstance(
                proposal,
                (
                    DialogueCandidateProposal,
                    AssetOccurrenceCandidateProposal,
                    ShotCandidateProposal,
                ),
            ):
                scene = candidates_by_key.get(proposal.scene_candidate_key)
                if scene is None or scene.proposal.kind != "scene":
                    raise ValueError("scene candidate reference is invalid")
            if isinstance(proposal, ContinuityCandidateProposal):
                scene_keys = set(proposal.scene_candidate_keys)
                if proposal.scene_candidate_key is not None:
                    scene_keys.add(proposal.scene_candidate_key)
                if any(
                    candidates_by_key.get(scene_key) is None
                    or candidates_by_key[scene_key].proposal.kind != "scene"
                    for scene_key in scene_keys
                ):
                    raise ValueError("continuity scene candidate reference is invalid")
        return self


class ExtractionCandidateResponse(BaseModel):
    id: UUID
    batch_id: UUID
    candidate_key: str
    kind: CandidateKind
    source_range: CandidateSourceRange
    proposal: CandidateProposal
    confidence_note: str | None
    required: bool
    status: CandidateStatus
    revision: int
    created_at: datetime


class CandidateDecisionEvidenceResponse(BaseModel):
    id: UUID
    candidate_id: UUID
    sequence: int
    decision_key: str
    decision: CandidateDecisionCommand
    downstream_type: Literal["ASSET"] | None
    downstream_id: UUID | None
    actor_id: UUID
    created_at: datetime


class CandidateDecisionResultResponse(BaseModel):
    candidate: ExtractionCandidateResponse
    evidence: CandidateDecisionEvidenceResponse


class PaginatedCandidateDecisions(BaseModel):
    items: list[CandidateDecisionEvidenceResponse]
    total: int
    limit: int
    offset: int


class PaginatedExtractionCandidates(BaseModel):
    items: list[ExtractionCandidateResponse]
    total: int
    limit: int
    offset: int
