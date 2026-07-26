from __future__ import annotations

import json
from typing import Literal
from uuid import UUID

from pydantic import Field

from schemas.common import StrictContract
from schemas.delivery_quality import DeliveryProbeSummaryV1
from schemas.rendering import (
    RenderInputRefsV1,
    RenderRecipeV1,
    RenderSegmentV1,
)
from schemas.subtitles import SubtitleInputRefsV1


class DeliveryMediaLineageV1(StrictContract):
    usage_type: Literal["shot_video", "speech_audio"]
    usage_id: UUID
    input_version_id: UUID
    input_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    adoption_id: UUID
    candidate_id: UUID
    media_version_id: UUID
    media_sha256: str = Field(pattern=r"^[0-9a-f]{64}$")
    media_kind: Literal["video", "audio"]
    source_kind: Literal["provider"]
    mime_type: str = Field(min_length=1, max_length=100)
    byte_size: int = Field(gt=0)
    duration_ticks: int = Field(gt=0)
    timebase: Literal[90000]
    probe_summary: dict[str, object]
    origin_attempt_id: UUID
    origin_task_id: UUID
    origin_submission_snapshot_id: UUID
    capability: Literal["video", "tts"]
    model_profile_id: str = Field(min_length=1, max_length=255)
    provider_id: str = Field(min_length=1, max_length=255)
    model_id: str = Field(min_length=1, max_length=255)
    route_version: str = Field(min_length=1, max_length=255)
    provider_schema_version: str = Field(min_length=1, max_length=255)


class DeliveryArtifactDigestV1(StrictContract):
    content_type: str = Field(min_length=1, max_length=100)
    byte_size: int = Field(gt=0)
    sha256: str = Field(pattern=r"^[0-9a-f]{64}$")


class DeliveryManifestArtifactsV1(StrictContract):
    mp4: DeliveryArtifactDigestV1
    srt: DeliveryArtifactDigestV1


class DeliveryManifestV1(StrictContract):
    schema_version: Literal["delivery-manifest-v1"] = "delivery-manifest-v1"
    episode_id: UUID
    render_snapshot_id: UUID
    render_task_id: UUID
    final_attempt_id: UUID
    snapshot_content_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    recipe_hash: str = Field(pattern=r"^[0-9a-f]{64}$")
    inputs: RenderInputRefsV1
    subtitle_input_refs: SubtitleInputRefsV1
    media_lineage: tuple[DeliveryMediaLineageV1, ...] = Field(min_length=7, max_length=110)
    segments: tuple[RenderSegmentV1, ...]
    recipe: RenderRecipeV1
    artifacts: DeliveryManifestArtifactsV1
    quality: DeliveryProbeSummaryV1

    def canonical_bytes(self) -> bytes:
        return json.dumps(
            self.model_dump(mode="json"),
            ensure_ascii=False,
            sort_keys=True,
            separators=(",", ":"),
        ).encode()


class DeliveryArtifactRefV1(DeliveryArtifactDigestV1):
    media_version_id: UUID
    bucket: str = Field(min_length=1, max_length=255)
    object_key: str = Field(min_length=1, max_length=1024)


class DeliveryArtifactSummaryV1(StrictContract):
    schema_version: Literal["delivery-artifacts-v1"] = "delivery-artifacts-v1"
    mp4: DeliveryArtifactRefV1
    srt: DeliveryArtifactRefV1
    manifest: DeliveryArtifactRefV1
