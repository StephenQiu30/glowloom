from __future__ import annotations

import re
from dataclasses import dataclass
from datetime import datetime, timedelta
from uuid import UUID

from core.clock import Clock
from db.pool import DatabasePool
from integrations.object_storage import ObjectStore
from repositories.candidates import CandidateRepository
from schemas.candidates import CandidateSnapshot
from schemas.media_registration import UsageType

INPUT_HASH = re.compile(r"^[0-9a-f]{64}$")
PREVIEW_TTL_SECONDS = 900


class CandidateNotFound(LookupError):
    pass


class CandidateQueryInvalid(ValueError):
    pass


@dataclass(frozen=True, slots=True)
class PreviewAuthorization:
    media_version_id: UUID
    url: str
    expires_in_seconds: int
    expires_at: datetime


class CandidateQueryService:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._repository = CandidateRepository()

    async def list(
        self,
        *,
        episode_id: UUID,
        usage_type: UsageType,
        usage_id: UUID,
        input_version_id: UUID,
        input_hash: str,
    ) -> tuple[CandidateSnapshot, ...]:
        if INPUT_HASH.fullmatch(input_hash) is None:
            raise CandidateQueryInvalid("input hash is invalid")
        async with self._database.transaction() as connection:
            return await self._repository.list_for_slot(
                connection,
                episode_id=episode_id,
                usage_type=usage_type,
                usage_id=usage_id,
                input_version_id=input_version_id,
                input_hash=input_hash,
            )


class PreviewAuthorizationService:
    def __init__(
        self, database: DatabasePool, object_store: ObjectStore, clock: Clock
    ) -> None:
        self._database = database
        self._object_store = object_store
        self._clock = clock
        self._repository = CandidateRepository()

    async def authorize(
        self, *, episode_id: UUID, media_version_id: UUID
    ) -> PreviewAuthorization:
        async with self._database.transaction() as connection:
            media = await self._repository.preview_media(
                connection,
                episode_id=episode_id,
                media_version_id=media_version_id,
            )
        if media is None:
            raise CandidateNotFound("ready candidate media was not found")
        url = await self._object_store.authorize_read(
            bucket=media.bucket,
            object_key=media.object_key,
            expires_seconds=PREVIEW_TTL_SECONDS,
        )
        return PreviewAuthorization(
            media_version_id,
            url,
            PREVIEW_TTL_SECONDS,
            self._clock.now() + timedelta(seconds=PREVIEW_TTL_SECONDS),
        )
