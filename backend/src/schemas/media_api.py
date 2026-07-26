from __future__ import annotations

from uuid import UUID

from pydantic import Field

from schemas.common import StrictContract
from schemas.media_registration import UsageType


class GenerateMediaRequest(StrictContract):
    usage_type: UsageType
    usage_id: UUID = Field(strict=False)
    input_version_id: UUID = Field(strict=False)
    model_profile_id: str | None = Field(default=None, min_length=1, max_length=120)
