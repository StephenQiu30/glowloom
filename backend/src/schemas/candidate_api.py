from __future__ import annotations

from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import Field

from schemas.candidates import CandidateSnapshot
from schemas.common import StrictContract
from schemas.media_registration import UsageType
from services.candidates import PreviewAuthorization


class CandidateTechnicalSummary(StrictContract):
    mime_type: str
    byte_size: int | None
    sha256: str | None
    width: int | None
    height: int | None
    duration_ticks: int | None
    timebase: int | None
    codec: str | None = None
    pixel_format: str | None = None
    frame_rate: str | None = None
    audio_stream_count: int | None = None
    sample_rate: int | None = None
    channels: int | None = None


class CandidateResponse(StrictContract):
    id: UUID
    episode_id: UUID
    task_id: UUID
    attempt_id: UUID
    output_slot: str
    usage_type: UsageType
    usage_id: UUID
    input_version_id: UUID
    input_hash: str
    media_version_id: UUID
    status: Literal["pending_media", "ready", "blocked"]
    blocked_reason: str | None
    model_profile_id: str
    provider_id: str
    model_id: str
    route_version: str
    schema_version: str
    active_adoption_id: UUID | None
    technical_summary: CandidateTechnicalSummary
    created_at: datetime
    finalized_at: datetime | None


class CandidateListResponse(StrictContract):
    items: tuple[CandidateResponse, ...]


class PreviewAuthorizationRequest(StrictContract):
    episode_id: UUID = Field(strict=False)


class PreviewAuthorizationResponse(StrictContract):
    media_version_id: UUID
    url: str = Field(pattern=r"^https?://")
    expires_in_seconds: Literal[900]
    expires_at: datetime


def candidate_response(value: CandidateSnapshot) -> CandidateResponse:
    probe = value.probe_summary
    technical = CandidateTechnicalSummary(
        mime_type=value.mime_type,
        byte_size=value.byte_size,
        sha256=value.sha256,
        width=value.width,
        height=value.height,
        duration_ticks=value.duration_ticks,
        timebase=value.timebase,
        codec=_text(probe, "codec"),
        pixel_format=_text(probe, "pixel_format"),
        frame_rate=_text(probe, "frame_rate"),
        audio_stream_count=_integer(probe, "audio_stream_count"),
        sample_rate=_integer(probe, "sample_rate"),
        channels=_integer(probe, "channels"),
    )
    return CandidateResponse(
        id=value.id,
        episode_id=value.episode_id,
        task_id=value.task_id,
        attempt_id=value.attempt_id,
        output_slot=value.output_slot,
        usage_type=value.usage_type,
        usage_id=value.usage_id,
        input_version_id=value.input_version_id,
        input_hash=value.input_hash,
        media_version_id=value.media_version_id,
        status=value.status,
        blocked_reason=value.blocked_reason,
        model_profile_id=value.model_profile_id,
        provider_id=value.provider_id,
        model_id=value.model_id,
        route_version=value.route_version,
        schema_version=value.schema_version,
        active_adoption_id=value.active_adoption_id,
        technical_summary=technical,
        created_at=value.created_at,
        finalized_at=value.finalized_at,
    )


def preview_response(value: PreviewAuthorization) -> PreviewAuthorizationResponse:
    return PreviewAuthorizationResponse(
        media_version_id=value.media_version_id,
        url=value.url,
        expires_in_seconds=900,
        expires_at=value.expires_at,
    )


def _text(value: dict[str, object], key: str) -> str | None:
    item = value.get(key)
    return item if isinstance(item, str) else None


def _integer(value: dict[str, object], key: str) -> int | None:
    item = value.get(key)
    return item if isinstance(item, int) else None
