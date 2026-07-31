from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel

from app.modules.scripts.extractions.schemas import CandidateSourceRange
from app.modules.scripts.versions.schemas import ScriptVersionResponse


class DialogueResponse(BaseModel):
    id: UUID
    scene_id: UUID
    position: int
    speaker_candidate: str
    dialogue_kind: Literal["spoken", "narration", "internal", "voice_over"]
    text: str
    performance_note: str | None
    source_range: CandidateSourceRange
    created_at: datetime


class SceneResponse(BaseModel):
    id: UUID
    script_version_id: UUID
    position: int
    heading: str
    location: str
    time_of_day: str
    summary: str
    source_range: CandidateSourceRange
    dialogues: list[DialogueResponse]
    created_at: datetime


class StructureConfirmationResponse(BaseModel):
    batch_id: UUID
    source_script_version_id: UUID
    confirmed_version: ScriptVersionResponse
    scenes: list[SceneResponse]


class ConfirmedStructureResponse(BaseModel):
    script_version_id: UUID
    scenes: list[SceneResponse]
