from collections import defaultdict
from datetime import UTC, datetime
from typing import Literal, cast
from uuid import UUID

from pydantic import TypeAdapter, ValidationError
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.identity import Capability, actor_context
from app.modules.scripts import repository
from app.modules.scripts.authorization import (
    require_resource_access,
    resource_not_found,
)
from app.modules.scripts.extractions.schemas import (
    CandidateProposal,
    CandidateSourceRange,
    DialogueCandidateProposal,
    SceneCandidateProposal,
)
from app.modules.scripts.models import (
    CandidateDecision,
    Dialogue,
    ExtractionBatch,
    ExtractionCandidate,
    Scene,
    ScriptVersion,
)
from app.modules.scripts.structure.schemas import (
    DialogueResponse,
    SceneResponse,
    StructureConfirmationResponse,
)
from app.modules.scripts.versions.schemas import ScriptVersionResponse

_PROPOSAL_ADAPTER: TypeAdapter[CandidateProposal] = TypeAdapter(CandidateProposal)


def _version_response(version: ScriptVersion) -> ScriptVersionResponse:
    return ScriptVersionResponse(
        id=version.id,
        workspace_id=version.workspace_id,
        source_id=version.source_id,
        version_no=version.version_no,
        status=cast(Literal["draft", "published"], version.status),
        body=version.body,
        content_hash=version.content_hash,
        created_by=version.created_by,
        created_at=version.created_at,
    )


def _confirmation_response(
    batch: ExtractionBatch,
    confirmed_version: ScriptVersion,
    scenes: list[Scene],
    dialogues: list[Dialogue],
) -> StructureConfirmationResponse:
    dialogues_by_scene: dict[UUID, list[Dialogue]] = defaultdict(list)
    for dialogue in dialogues:
        dialogues_by_scene[dialogue.scene_id].append(dialogue)
    return StructureConfirmationResponse(
        batch_id=batch.id,
        source_script_version_id=batch.script_version_id,
        confirmed_version=_version_response(confirmed_version),
        scenes=[
            SceneResponse(
                id=scene.id,
                script_version_id=scene.script_version_id,
                position=scene.position,
                heading=scene.heading,
                location=scene.location,
                time_of_day=scene.time_of_day,
                summary=scene.summary,
                source_range=CandidateSourceRange(
                    start=scene.source_start,
                    end=scene.source_end,
                ),
                dialogues=[
                    DialogueResponse(
                        id=dialogue.id,
                        scene_id=dialogue.scene_id,
                        position=dialogue.position,
                        speaker_candidate=dialogue.speaker_candidate,
                        dialogue_kind=cast(
                            Literal[
                                "spoken",
                                "narration",
                                "internal",
                                "voice_over",
                            ],
                            dialogue.dialogue_kind,
                        ),
                        text=dialogue.text,
                        performance_note=dialogue.performance_note,
                        source_range=CandidateSourceRange(
                            start=dialogue.source_start,
                            end=dialogue.source_end,
                        ),
                        created_at=dialogue.created_at,
                    )
                    for dialogue in dialogues_by_scene[scene.id]
                ],
                created_at=scene.created_at,
            )
            for scene in scenes
        ],
    )


def _candidate_detail(candidate: ExtractionCandidate) -> dict[str, str]:
    return {
        "candidate_id": str(candidate.id),
        "candidate_key": candidate.candidate_key,
        "kind": candidate.kind,
        "status": candidate.status,
    }


def _latest_decisions(
    decisions: list[CandidateDecision],
) -> dict[UUID, CandidateDecision]:
    latest: dict[UUID, CandidateDecision] = {}
    for decision in decisions:
        previous = latest.get(decision.candidate_id)
        if previous is None or decision.sequence > previous.sequence:
            latest[decision.candidate_id] = decision
    return latest


def _proposal_for(
    candidate: ExtractionCandidate,
    decisions: dict[UUID, CandidateDecision],
) -> CandidateProposal:
    decision = decisions.get(candidate.id)
    if decision is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Structure decision evidence is unavailable",
            status_code=500,
        )
    raw_proposal = (
        decision.payload.get("proposal")
        if decision.action == "accept_with_changes"
        else candidate.proposal
    )
    try:
        return _PROPOSAL_ADAPTER.validate_python(raw_proposal)
    except ValidationError as error:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Structure proposal is invalid",
            status_code=500,
        ) from error


def _merge_target_id(
    candidate: ExtractionCandidate,
    decisions: dict[UUID, CandidateDecision],
) -> UUID:
    decision = decisions.get(candidate.id)
    target = None if decision is None else decision.payload.get("target_candidate_id")
    try:
        return UUID(str(target))
    except (TypeError, ValueError) as error:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Scene merge evidence is invalid",
            status_code=500,
        ) from error


def _resolve_scene_candidate(
    candidate: ExtractionCandidate,
    scene_by_id: dict[UUID, ExtractionCandidate],
    decisions: dict[UUID, CandidateDecision],
) -> ExtractionCandidate:
    current = candidate
    visited: set[UUID] = set()
    while current.status == "merged":
        if current.id in visited:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Scene candidate merge cycle prevents confirmation",
                status_code=409,
                next_action="resolve_structure_candidates",
                details={"candidate_key": candidate.candidate_key},
            )
        visited.add(current.id)
        target_id = _merge_target_id(current, decisions)
        target = scene_by_id.get(target_id)
        if target is None:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Scene candidate reference prevents confirmation",
                status_code=409,
                next_action="resolve_structure_candidates",
                details={"candidate_key": candidate.candidate_key},
            )
        current = target
    if current.status != "accepted":
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Scene candidate reference is not accepted",
            status_code=409,
            next_action="resolve_structure_candidates",
            details={"candidate_key": candidate.candidate_key},
        )
    return current


async def _existing_confirmation(
    session: AsyncSession,
    batch: ExtractionBatch,
) -> StructureConfirmationResponse:
    confirmed_version_id = batch.confirmed_script_version_id
    if confirmed_version_id is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Confirmed structure state is unavailable",
            status_code=500,
        )
    version = await repository.find_version(session, confirmed_version_id)
    if version is None or version.workspace_id != batch.workspace_id:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Confirmed script version is unavailable",
            status_code=500,
        )
    scenes = await repository.list_scenes(session, version.id)
    dialogues = await repository.list_dialogues(
        session,
        [scene.id for scene in scenes],
    )
    return _confirmation_response(batch, version, scenes, dialogues)


async def confirm_structure(
    session: AsyncSession,
    claims: AccessTokenClaims,
    batch_id: UUID,
) -> StructureConfirmationResponse:
    async with session.begin():
        batch = await repository.find_extraction_batch(
            session,
            batch_id,
            for_update=True,
        )
        if batch is None:
            raise resource_not_found("Extraction batch")
        await require_resource_access(
            session,
            claims,
            batch.workspace_id,
            "Extraction batch",
        )
        actor = await actor_context(
            session,
            claims,
            batch.workspace_id,
            Capability.CONTENT_WRITE,
        )
        if batch.confirmed_script_version_id is not None:
            return await _existing_confirmation(session, batch)
        if batch.status != "succeeded":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Extraction batch is not ready for structure confirmation",
                status_code=409,
            )

        candidates = await repository.list_structure_candidates(
            session,
            batch.id,
            for_update=True,
        )
        unresolved = [
            candidate
            for candidate in candidates
            if candidate.required and candidate.status == "pending"
        ]
        blocking_continuity = [
            candidate
            for candidate in candidates
            if candidate.kind == "continuity"
            and candidate.status == "pending"
            and candidate.proposal.get("severity") == "blocking"
        ]
        if unresolved or blocking_continuity:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Structure candidates must be resolved before confirmation",
                status_code=409,
                next_action="resolve_structure_candidates",
                details={
                    "unresolved_candidates": [
                        _candidate_detail(candidate) for candidate in unresolved
                    ],
                    "blocking_continuity_candidates": [
                        _candidate_detail(candidate)
                        for candidate in blocking_continuity
                    ],
                },
            )

        decisions = _latest_decisions(
            await repository.list_candidate_decisions_for_candidates(
                session,
                [candidate.id for candidate in candidates],
            )
        )
        scene_candidates = [
            candidate for candidate in candidates if candidate.kind == "scene"
        ]
        accepted_scene_candidates = [
            candidate for candidate in scene_candidates if candidate.status == "accepted"
        ]
        scene_by_id = {candidate.id: candidate for candidate in scene_candidates}
        scene_by_key = {
            candidate.candidate_key: candidate for candidate in scene_candidates
        }

        input_version = await repository.find_version(
            session,
            batch.script_version_id,
        )
        if input_version is None or input_version.workspace_id != batch.workspace_id:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Extraction input version is unavailable",
                status_code=500,
            )
        source = await repository.find_source(
            session,
            input_version.source_id,
            for_update=True,
        )
        if source is None or source.workspace_id != batch.workspace_id:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Script source is unavailable",
                status_code=500,
            )
        if source.status != "active":
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Script source is archived",
                status_code=409,
            )

        now = datetime.now(UTC)
        confirmed_version = ScriptVersion(
            id=uuid7(),
            workspace_id=batch.workspace_id,
            source_id=source.id,
            version_no=await repository.latest_version_number(session, source.id) + 1,
            status="published",
            body=input_version.body,
            content_hash=input_version.content_hash,
            structure_summary={
                "confirmation_batch_id": str(batch.id),
                "source_script_version_id": str(input_version.id),
                "scene_count": len(accepted_scene_candidates),
            },
            created_by=actor.user_id,
            created_at=now,
        )
        scenes: list[Scene] = []
        scene_by_candidate_id: dict[UUID, Scene] = {}
        for position, candidate in enumerate(accepted_scene_candidates, start=1):
            proposal = _proposal_for(candidate, decisions)
            if not isinstance(proposal, SceneCandidateProposal):
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR,
                    "Scene proposal type is invalid",
                    status_code=500,
                )
            scene = Scene(
                id=uuid7(),
                workspace_id=batch.workspace_id,
                script_version_id=confirmed_version.id,
                position=position,
                heading=proposal.heading,
                location=proposal.location,
                time_of_day=proposal.time_of_day,
                summary=proposal.summary,
                source_start=candidate.source_start,
                source_end=candidate.source_end,
                created_at=now,
            )
            scenes.append(scene)
            scene_by_candidate_id[candidate.id] = scene

        dialogue_candidates_by_scene: dict[UUID, list[ExtractionCandidate]] = defaultdict(
            list
        )
        dialogue_proposals: dict[UUID, DialogueCandidateProposal] = {}
        for candidate in candidates:
            if candidate.kind != "dialogue" or candidate.status != "accepted":
                continue
            proposal = _proposal_for(candidate, decisions)
            if not isinstance(proposal, DialogueCandidateProposal):
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR,
                    "Dialogue proposal type is invalid",
                    status_code=500,
                )
            referenced_scene = scene_by_key.get(proposal.scene_candidate_key)
            if referenced_scene is None:
                raise ApiError(
                    ErrorCode.STATE_CONFLICT,
                    "Dialogue scene reference prevents confirmation",
                    status_code=409,
                    next_action="resolve_structure_candidates",
                    details={"candidate_key": candidate.candidate_key},
                )
            resolved_scene = _resolve_scene_candidate(
                referenced_scene,
                scene_by_id,
                decisions,
            )
            dialogue_candidates_by_scene[resolved_scene.id].append(candidate)
            dialogue_proposals[candidate.id] = proposal

        dialogues: list[Dialogue] = []
        for scene_candidate in accepted_scene_candidates:
            scene = scene_by_candidate_id[scene_candidate.id]
            for position, candidate in enumerate(
                dialogue_candidates_by_scene[scene_candidate.id],
                start=1,
            ):
                proposal = dialogue_proposals[candidate.id]
                dialogues.append(
                    Dialogue(
                        id=uuid7(),
                        workspace_id=batch.workspace_id,
                        scene_id=scene.id,
                        position=position,
                        speaker_candidate=proposal.speaker_candidate,
                        dialogue_kind=proposal.dialogue_kind,
                        text=proposal.text,
                        performance_note=proposal.performance_note,
                        source_start=candidate.source_start,
                        source_end=candidate.source_end,
                        created_at=now,
                    )
                )

        confirmed_version.structure_summary["dialogue_count"] = len(dialogues)
        session.add(confirmed_version)
        await session.flush()
        session.add_all(scenes)
        await session.flush()
        session.add_all(dialogues)
        batch.confirmed_script_version_id = confirmed_version.id
        batch.updated_at = now
        await session.flush()
    return _confirmation_response(batch, confirmed_version, scenes, dialogues)
