from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, field_validator, model_validator

AnalysisStatus = Literal["deterministic", "ai_candidate_required", "rejected"]
DocumentSourceType = Literal["text", "media"]
NarrativeBlockKind = Literal[
    "preamble",
    "episode_marker",
    "scene_heading",
    "dialogue",
    "narration",
    "action",
    "separator",
]


class ScriptDocumentImportRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    input_type: DocumentSourceType
    title: str = Field(min_length=1, max_length=120)
    text: str | None = Field(default=None, max_length=100_000)
    media_version_id: UUID | None = None
    language: str = Field(min_length=1, max_length=35)
    rights_declaration: str = Field(min_length=1, max_length=1000)
    idempotency_key: str = Field(min_length=1, max_length=200)

    @field_validator(
        "title", "language", "rights_declaration", "idempotency_key"
    )
    @classmethod
    def strip_required_metadata(cls, value: str) -> str:
        normalized = value.strip()
        if not normalized:
            raise ValueError("metadata must contain text")
        return normalized

    @model_validator(mode="after")
    def validate_source(self) -> "ScriptDocumentImportRequest":
        if self.input_type == "text":
            if self.media_version_id is not None or self.text is None:
                raise ValueError("text input requires text and forbids media_version_id")
            if not self.text.strip():
                raise ValueError("text input must contain text")
        elif self.text is not None or self.media_version_id is None:
            raise ValueError("media input requires media_version_id and forbids text")
        return self


class ScriptDocumentResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    project_id: UUID
    title: str
    source_type: DocumentSourceType
    source_media_version_id: UUID | None
    language: str
    rights_declaration: str
    status: Literal["active", "archived"]
    revision: int
    created_by: UUID
    created_at: datetime


class DocumentRevisionResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    document_id: UUID
    version_no: int
    source_type: DocumentSourceType
    source_media_version_id: UUID | None
    raw_text: str
    raw_hash: str
    normalized_text: str
    normalized_hash: str
    normalizer_version: str
    normalization_map: dict[str, object]
    codepoint_count: int
    analysis_status: AnalysisStatus
    analyzer_version: str
    created_by: UUID
    created_at: datetime


class NarrativeBlockResponse(BaseModel):
    id: UUID
    document_revision_id: UUID
    position: int
    kind: NarrativeBlockKind
    source_start: int
    source_end: int
    text_hash: str
    metadata: dict[str, object]


class FormatIssueResponse(BaseModel):
    id: UUID
    document_revision_id: UUID
    position: int
    code: str
    severity: Literal["warning", "blocking"]
    source_start: int
    source_end: int
    line_number: int
    column_number: int
    next_action: str
    details: dict[str, object]


class ScriptDocumentAnalysisResponse(BaseModel):
    document: ScriptDocumentResponse
    revision: DocumentRevisionResponse
    blocks: list[NarrativeBlockResponse]
    issues: list[FormatIssueResponse]


class PaginatedScriptDocuments(BaseModel):
    items: list[ScriptDocumentResponse]
    total: int
    limit: int
    offset: int
