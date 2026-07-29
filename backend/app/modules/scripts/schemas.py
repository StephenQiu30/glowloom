from datetime import datetime
from typing import Annotated, Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

from app.modules.production.schemas import TaskResponse, TaskStatus


class CommandModel(BaseModel):
    model_config = ConfigDict(extra="forbid")


class ScriptImportRequest(CommandModel):
    input_type: Literal["text"]
    title: str = Field(min_length=1, max_length=120)
    body: str = Field(min_length=1, max_length=20_000)
    rights_declaration: str = Field(min_length=1, max_length=1000)
    idempotency_key: str = Field(min_length=1, max_length=200)

    @field_validator("body", mode="before")
    @classmethod
    def normalize_newlines(cls, value: object) -> object:
        if isinstance(value, str):
            return value.replace("\r\n", "\n").replace("\r", "\n")
        return value

    @field_validator("body")
    @classmethod
    def reject_blank_body(cls, value: str) -> str:
        if not value.strip():
            raise ValueError("script body must contain text")
        return value


class ScriptVersionPublishRequest(CommandModel):
    body: str = Field(min_length=1, max_length=20_000)
    expected_current_version_id: UUID | None

    @field_validator("body", mode="before")
    @classmethod
    def normalize_newlines(cls, value: object) -> object:
        if isinstance(value, str):
            return value.replace("\r\n", "\n").replace("\r", "\n")
        return value

    @field_validator("body")
    @classmethod
    def reject_blank_body(cls, value: str) -> str:
        if not value.strip():
            raise ValueError("script body must contain text")
        return value


class ScriptSourceResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    episode_id: UUID
    input_type: Literal["text", "media"]
    title: str
    source_media_version_id: UUID | None
    rights_declaration: str
    status: Literal["active", "archived"]
    revision: int
    created_at: datetime


class ScriptVersionResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    source_id: UUID
    version_no: int
    status: Literal["draft", "published"]
    body: str
    content_hash: str
    created_by: UUID
    created_at: datetime


class ScriptImportResponse(BaseModel):
    source: ScriptSourceResponse
    version: ScriptVersionResponse


class CurrentScriptVersionRequest(CommandModel):
    version_id: UUID
    expected_current_version_id: UUID | None


class CurrentScriptVersionResponse(BaseModel):
    episode_id: UUID
    current_script_version_id: UUID
    episode_revision: int


class ScriptVersionPublishResponse(BaseModel):
    version: ScriptVersionResponse
    current: CurrentScriptVersionResponse


class ScriptSourceStateRequest(CommandModel):
    expected_revision: int = Field(ge=1)


class ScriptVersionDiffResponse(BaseModel):
    base_version_id: UUID
    target_version_id: UUID
    added_lines: int
    removed_lines: int
    diff_lines: list[str]


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


class PaginatedExtractionCandidates(BaseModel):
    items: list[ExtractionCandidateResponse]
    total: int
    limit: int
    offset: int


class PaginatedScriptVersions(BaseModel):
    items: list[ScriptVersionResponse]
    total: int
    limit: int
    offset: int
