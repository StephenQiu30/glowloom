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


class ExtractionBatchResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    script_version_id: UUID
    scope: Literal["full"]
    extractor_version: str
    input_hash: str
    status: TaskStatus
    confirmed_script_version_id: UUID | None
    candidate_count: int
    task: TaskResponse
    created_at: datetime


CandidateKind = Literal["scene", "dialogue", "asset", "shot", "continuity"]
CandidateStatus = Literal["pending", "accepted", "linked", "merged", "ignored"]


class CandidateSourceRange(CommandModel):
    start: int = Field(ge=0)
    end: int = Field(ge=1)

    @model_validator(mode="after")
    def validate_order(self) -> "CandidateSourceRange":
        if self.end <= self.start:
            raise ValueError("source range end must be greater than start")
        return self


class SceneCandidateProposal(CommandModel):
    kind: Literal["scene"]
    heading: str = Field(min_length=1, max_length=200)
    location: str = Field(min_length=1, max_length=200)
    time_of_day: str = Field(min_length=1, max_length=100)
    summary: str = Field(min_length=1, max_length=2000)


class DialogueCandidateProposal(CommandModel):
    kind: Literal["dialogue"]
    scene_candidate_key: str = Field(min_length=1, max_length=100)
    speaker_candidate: str = Field(min_length=1, max_length=200)
    dialogue_kind: Literal["spoken", "narration", "internal", "voice_over"]
    text: str = Field(min_length=1, max_length=4000)
    performance_note: str | None = Field(default=None, max_length=1000)


class AssetCandidateProposal(CommandModel):
    kind: Literal["asset"]
    asset_kind: Literal[
        "character", "location", "prop", "costume", "style", "voice"
    ]
    name: str = Field(min_length=1, max_length=200)
    description: str = Field(min_length=1, max_length=2000)


class ShotCandidateProposal(CommandModel):
    kind: Literal["shot"]
    scene_candidate_key: str = Field(min_length=1, max_length=100)
    title: str = Field(min_length=1, max_length=200)
    purpose: str = Field(min_length=1, max_length=2000)


class ContinuityCandidateProposal(CommandModel):
    kind: Literal["continuity"]
    severity: Literal["info", "warning", "blocking"]
    issue: str = Field(min_length=1, max_length=2000)
    suggestion: str = Field(min_length=1, max_length=2000)


CandidateProposal = Annotated[
    SceneCandidateProposal
    | DialogueCandidateProposal
    | AssetCandidateProposal
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


class IgnoreDecision(CommandModel):
    action: Literal["ignore"]


CandidateDecisionCommand = Annotated[
    AcceptNewDecision
    | AcceptWithChangesDecision
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
    candidates: list[ExtractionCandidateInput] = Field(max_length=500)

    @model_validator(mode="after")
    def validate_candidate_references(self) -> "ScriptExtractionResult":
        candidates_by_key = {
            candidate.candidate_key: candidate for candidate in self.candidates
        }
        if len(candidates_by_key) != len(self.candidates):
            raise ValueError("candidate keys must be unique")
        for candidate in self.candidates:
            proposal = candidate.proposal
            if isinstance(proposal, (DialogueCandidateProposal, ShotCandidateProposal)):
                scene = candidates_by_key.get(proposal.scene_candidate_key)
                if scene is None or scene.proposal.kind != "scene":
                    raise ValueError("scene candidate reference is invalid")
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
