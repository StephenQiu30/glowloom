from __future__ import annotations

from dataclasses import dataclass
from uuid import UUID

import asyncpg  # type: ignore[import-untyped]

from core.ids import new_id
from db.pool import DatabasePool
from repositories.adoptions import AdoptionRepository, CandidateForAdoptionRow
from repositories.idempotency import (
    IdempotencyRepository,
    canonical_request_hash,
)
from schemas.adoptions import AdoptionSnapshot
from schemas.media_registration import UsageType
from schemas.story_content import canonical_content_hash
from services.media_generation_inputs import (
    MediaInputFreezer,
    MediaInputNotFound,
    MediaInputOutdated,
    MediaInputRequest,
    UnsupportedMediaUsage,
)
from services.speech_generation_inputs import SpeechInputFreezer


class AdoptionCandidateNotFound(LookupError):
    pass


class CandidateNotAdoptable(ValueError):
    pass


class AdoptionInputOutdated(ValueError):
    pass


@dataclass(frozen=True, slots=True)
class AdoptCandidateCommand:
    usage_type: UsageType
    usage_id: UUID
    input_version_id: UUID
    input_hash: str
    candidate_id: UUID
    idempotency_key: str


class AdoptCandidateHandler:
    def __init__(self, database: DatabasePool) -> None:
        self._database = database
        self._adoptions = AdoptionRepository()
        self._idempotency = IdempotencyRepository()
        self._media_inputs = MediaInputFreezer()
        self._speech_inputs = SpeechInputFreezer()

    async def execute(self, command: AdoptCandidateCommand) -> AdoptionSnapshot:
        scope = (
            f"adoptCandidate/{command.usage_type}/{command.usage_id}/"
            f"{command.input_version_id}/{command.input_hash}"
        )
        request_hash = canonical_request_hash(
            method="POST",
            operation_id="adoptCandidate",
            path_parameters={},
            body={
                "usage_type": command.usage_type,
                "usage_id": str(command.usage_id),
                "input_version_id": str(command.input_version_id),
                "input_hash": command.input_hash,
                "candidate_id": str(command.candidate_id),
            },
        )
        async with self._database.transaction() as connection:
            stored = await self._idempotency.reserve(
                connection,
                owner_module="generation",
                operation_scope=scope,
                idempotency_key=command.idempotency_key,
                request_hash=request_hash,
                request_id=new_id(),
            )
            if stored is not None:
                adoption = await self._adoptions.get(
                    connection, UUID(str(stored.reference["adoption_id"]))
                )
                if adoption is None:
                    raise RuntimeError("idempotency response references a missing adoption")
                return adoption
            candidate = await self._adoptions.candidate_for_adoption(
                connection, command.candidate_id
            )
            if candidate is None:
                raise AdoptionCandidateNotFound
            self._validate_candidate(command, candidate)
            if not await self._adoptions.lock_episode(connection, candidate.episode_id):
                raise AdoptionCandidateNotFound
            await self._validate_current_input(connection, command, candidate)
            adoption = await self._adoptions.adopt(connection, candidate)
            await self._idempotency.complete(
                connection,
                operation_scope=scope,
                idempotency_key=command.idempotency_key,
                status=201,
                reference={"adoption_id": str(adoption.id)},
            )
            return adoption

    @staticmethod
    def _validate_candidate(
        command: AdoptCandidateCommand, candidate: CandidateForAdoptionRow
    ) -> None:
        slot = (
            candidate.usage_type,
            candidate.usage_id,
            candidate.input_version_id,
            candidate.input_hash,
        )
        requested = (
            command.usage_type,
            command.usage_id,
            command.input_version_id,
            command.input_hash,
        )
        if (
            slot != requested
            or candidate.output_slot != "primary"
            or candidate.candidate_status != "ready"
            or candidate.media_status != "ready"
        ):
            raise CandidateNotAdoptable("candidate is not ready for the requested slot")

    async def _validate_current_input(
        self,
        connection: asyncpg.Connection[asyncpg.Record],
        command: AdoptCandidateCommand,
        candidate: CandidateForAdoptionRow,
    ) -> None:
        request = MediaInputRequest(
            candidate.episode_id,
            command.usage_type,
            command.usage_id,
            command.input_version_id,
        )
        freezer = (
            self._speech_inputs if command.usage_type == "speech_audio" else self._media_inputs
        )
        try:
            frozen = await freezer.freeze(connection, request)
        except UnsupportedMediaUsage as error:
            raise CandidateNotAdoptable(str(error)) from error
        except (MediaInputNotFound, MediaInputOutdated) as error:
            raise AdoptionInputOutdated(str(error)) from error
        if canonical_content_hash(frozen.input_refs) != command.input_hash:
            raise AdoptionInputOutdated("candidate input is no longer current")
