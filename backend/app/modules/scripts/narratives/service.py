from datetime import UTC, datetime
from hashlib import sha256
from typing import Literal, cast
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.identity import Capability, actor_context
from app.modules.projects import (
    EpisodeContentContext,
    episode_for_content_read,
    resolve_episode_content_context,
    resolve_episode_content_contexts,
)
from app.modules.scripts import repository as scripts_repository
from app.modules.scripts.authorization import require_resource_access, resource_not_found
from app.modules.scripts.contracts import (
    NarrativeDependencySnapshot,
    NarrativeImpactSnapshot,
    NarrativeUnitVersionReference,
)
from app.modules.scripts.models import Dialogue, Scene, ScriptSource, ScriptVersion
from app.modules.scripts.narratives import repository
from app.modules.scripts.narratives.hashing import (
    canonical_hash,
    dependency_hash,
    structure_hash,
)
from app.modules.scripts.narratives.models import (
    NarrativeImpactAssessment,
    NarrativeStructure,
    NarrativeUnit,
    NarrativeUnitVersion,
)
from app.modules.scripts.narratives.parser import ParsedUnit, parse_narrative_units
from app.modules.scripts.narratives.schemas import (
    NarrativeDependencyResponse,
    NarrativeImpactResponse,
    NarrativeRevisionResponse,
    NarrativeStructureResponse,
    NarrativeStructureRevisionRequest,
    NarrativeUnitResponse,
    SourceRange,
)

PARSER_VERSION = "deterministic-lines-v1"
INVALIDATED_SCOPES = ["shot_readiness", "coverage", "export"]


async def resolve_narrative_unit_versions(
    session: AsyncSession,
    workspace_id: UUID,
    unit_version_ids: list[UUID],
) -> dict[UUID, NarrativeUnitVersionReference]:
    rows = await repository.list_unit_version_scopes(
        session,
        list(dict.fromkeys(unit_version_ids)),
    )
    contexts = await resolve_episode_content_contexts(
        session,
        workspace_id,
        [version.episode_id for version, _unit in rows],
    )
    return {
        version.id: NarrativeUnitVersionReference(
            workspace_id=version.workspace_id,
            project_id=context.project_id,
            episode_id=version.episode_id,
            script_version_id=version.script_version_id,
            narrative_unit_id=version.unit_id,
            narrative_unit_version_id=version.id,
            current_script_version_id=context.current_script_version_id,
            current_unit_version_id=unit.current_version_id,
            text_hash=version.text_hash,
        )
        for version, unit in rows
        if version.workspace_id == workspace_id
        and (context := contexts.get(version.episode_id)) is not None
    }


def _unit_hash_payload(
    *,
    unit_id: UUID,
    kind: str,
    position: int,
    source_start: int,
    source_end: int,
    exact_text: str,
    required_for_coverage: bool,
) -> dict[str, object]:
    return {
        "unit_id": str(unit_id),
        "kind": kind,
        "position": position,
        "source_start": source_start,
        "source_end": source_end,
        "exact_text_hash": sha256(exact_text.encode("utf-8")).hexdigest(),
        "required_for_coverage": required_for_coverage,
    }


def _context(body: str, start: int, end: int) -> tuple[str, str]:
    return body[max(0, start - 60) : start], body[end : min(len(body), end + 60)]


def _source_links(
    parsed: ParsedUnit,
    scenes: list[Scene],
    dialogues: list[Dialogue],
) -> tuple[UUID | None, UUID | None]:
    dialogue = next(
        (
            item
            for item in dialogues
            if item.source_start == parsed.source_start and item.source_end == parsed.source_end
        ),
        None,
    )
    if dialogue is not None:
        return dialogue.scene_id, dialogue.id
    scene = next(
        (item for item in scenes if item.source_start <= parsed.source_start < item.source_end),
        None,
    )
    return (scene.id if scene is not None else None), None


async def _script_context(
    session: AsyncSession,
    version_id: UUID,
) -> tuple[ScriptVersion, ScriptSource, EpisodeContentContext]:
    version = await scripts_repository.find_version(session, version_id)
    if version is None:
        raise resource_not_found("Script version")
    source = await scripts_repository.find_source(session, version.source_id)
    if source is None:
        raise resource_not_found("Script source")
    episode = await resolve_episode_content_context(
        session,
        version.workspace_id,
        source.episode_id,
    )
    if episode is None:
        raise resource_not_found("Episode")
    return version, source, episode


async def ensure_structure(
    session: AsyncSession,
    *,
    version: ScriptVersion,
    source: ScriptSource,
    episode: EpisodeContentContext,
    actor_id: UUID,
) -> NarrativeStructure:
    existing = await repository.find_structure_by_script(session, version.id)
    if existing is not None:
        return existing
    if version.status != "published":
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Only published script versions can have narrative structure",
            status_code=409,
        )
    if (
        source.workspace_id != version.workspace_id
        or episode.workspace_id != version.workspace_id
        or source.episode_id != episode.episode_id
    ):
        raise ApiError(
            ErrorCode.RESOURCE_CONFLICT,
            "Script version narrative scope is invalid",
            status_code=409,
        )
    parsed_units = parse_narrative_units(version.body)
    if not parsed_units:
        raise ApiError(
            ErrorCode.INVALID_REQUEST,
            "Script version contains no narrative units",
            status_code=422,
            next_action="edit_script_version",
        )
    scenes = await scripts_repository.list_scenes(session, version.id)
    dialogues = await scripts_repository.list_dialogues(session, [scene.id for scene in scenes])
    now = datetime.now(UTC)
    structure = NarrativeStructure(
        id=uuid7(),
        workspace_id=version.workspace_id,
        episode_id=episode.episode_id,
        script_version_id=version.id,
        input_hash=version.content_hash,
        parser_version=PARSER_VERSION,
        structure_hash="0" * 64,
        dependency_hash="0" * 64,
        revision=1,
        command_receipts={},
        created_by=actor_id,
        created_at=now,
        updated_at=now,
    )
    session.add(structure)
    await session.flush()
    unit_rows: list[NarrativeUnit] = []
    version_rows: list[NarrativeUnitVersion] = []
    hash_units: list[dict[str, object]] = []
    for position, parsed in enumerate(parsed_units, start=1):
        unit = NarrativeUnit(
            id=uuid7(),
            workspace_id=version.workspace_id,
            episode_id=episode.episode_id,
            kind=parsed.kind,
            status="active",
            current_version_id=None,
            revision=1,
            created_by=actor_id,
        )
        prefix, suffix = _context(version.body, parsed.source_start, parsed.source_end)
        source_scene_id, source_dialogue_id = _source_links(parsed, scenes, dialogues)
        version_row = NarrativeUnitVersion(
            id=uuid7(),
            workspace_id=version.workspace_id,
            episode_id=episode.episode_id,
            structure_id=structure.id,
            script_version_id=version.id,
            unit_id=unit.id,
            version_no=1,
            structure_revision=1,
            position=position,
            source_start=parsed.source_start,
            source_end=parsed.source_end,
            exact_text=parsed.exact_text,
            text_hash=sha256(parsed.exact_text.encode("utf-8")).hexdigest(),
            prefix_text=prefix,
            suffix_text=suffix,
            required_for_coverage=True,
            payload={},
            source_scene_id=source_scene_id,
            source_dialogue_id=source_dialogue_id,
            origin="deterministic",
            created_by=actor_id,
        )
        unit.current_version_id = version_row.id
        unit_rows.append(unit)
        version_rows.append(version_row)
        hash_units.append(
            _unit_hash_payload(
                unit_id=unit.id,
                kind=unit.kind,
                position=position,
                source_start=parsed.source_start,
                source_end=parsed.source_end,
                exact_text=parsed.exact_text,
                required_for_coverage=True,
            )
        )
    session.add_all(unit_rows)
    await session.flush()
    session.add_all(version_rows)
    await session.flush()
    structure.structure_hash = structure_hash(
        script_version_id=version.id,
        revision=1,
        units=hash_units,
    )
    structure.dependency_hash = dependency_hash(
        script_version_id=version.id,
        structure_hash_value=structure.structure_hash,
        unit_version_ids=[item.id for item in version_rows],
    )
    await session.flush()
    return structure


async def ensure_structure_for_version(
    session: AsyncSession,
    version_id: UUID,
    *,
    actor_id: UUID,
) -> NarrativeStructure:
    version, source, episode = await _script_context(session, version_id)
    return await ensure_structure(
        session,
        version=version,
        source=source,
        episode=episode,
        actor_id=actor_id,
    )


def _unit_response(item: NarrativeUnitVersion, kind: str) -> NarrativeUnitResponse:
    return NarrativeUnitResponse(
        id=item.id,
        unit_id=item.unit_id,
        kind=cast(
            Literal["scene_heading", "action", "dialogue", "narration"],
            kind,
        ),
        position=item.position,
        version_no=item.version_no,
        source_range=SourceRange(start=item.source_start, end=item.source_end),
        exact_text=item.exact_text,
        text_hash=item.text_hash,
        prefix_text=item.prefix_text,
        suffix_text=item.suffix_text,
        required_for_coverage=item.required_for_coverage,
        source_scene_id=item.source_scene_id,
        source_dialogue_id=item.source_dialogue_id,
        origin=cast(Literal["deterministic", "manual"], item.origin),
        created_at=item.created_at,
    )


async def _structure_response(
    session: AsyncSession,
    structure: NarrativeStructure,
    *,
    revision: int | None = None,
) -> NarrativeStructureResponse:
    selected_revision = revision or structure.revision
    if selected_revision < 1 or selected_revision > structure.revision:
        raise resource_not_found("Narrative structure revision")
    versions = await repository.list_versions(session, structure.id, selected_revision)
    if not versions:
        raise resource_not_found("Narrative structure revision")
    units = await repository.find_units(session, [item.unit_id for item in versions])
    kind_by_id = {item.id: item.kind for item in units}
    selected_structure_hash = structure_hash(
        script_version_id=structure.script_version_id,
        revision=selected_revision,
        units=[
            _unit_hash_payload(
                unit_id=item.unit_id,
                kind=kind_by_id[item.unit_id],
                position=item.position,
                source_start=item.source_start,
                source_end=item.source_end,
                exact_text=item.exact_text,
                required_for_coverage=item.required_for_coverage,
            )
            for item in versions
        ],
    )
    selected_dependency_hash = dependency_hash(
        script_version_id=structure.script_version_id,
        structure_hash_value=selected_structure_hash,
        unit_version_ids=[item.id for item in versions],
    )
    revision_updated_at = max(item.created_at for item in versions)
    return NarrativeStructureResponse(
        id=structure.id,
        workspace_id=structure.workspace_id,
        episode_id=structure.episode_id,
        script_version_id=structure.script_version_id,
        input_hash=structure.input_hash,
        parser_version=structure.parser_version,
        structure_hash=selected_structure_hash,
        dependency_hash=selected_dependency_hash,
        revision=selected_revision,
        units=[_unit_response(item, kind_by_id[item.unit_id]) for item in versions],
        created_at=structure.created_at,
        updated_at=revision_updated_at,
    )


async def resolve_narrative_dependencies(
    session: AsyncSession,
    workspace_id: UUID,
    script_version_ids: list[UUID],
) -> dict[UUID, NarrativeDependencySnapshot]:
    unique_ids = list(dict.fromkeys(script_version_ids))
    structures = await repository.list_structures_by_scripts(
        session,
        workspace_id,
        unique_ids,
    )
    return {
        structure.script_version_id: NarrativeDependencySnapshot(
            episode_id=structure.episode_id,
            script_version_id=structure.script_version_id,
            structure_id=structure.id,
            structure_revision=structure.revision,
            dependency_hash=structure.dependency_hash,
        )
        for structure in structures
    }


async def get_structure(
    session: AsyncSession,
    claims: AccessTokenClaims,
    version_id: UUID,
    *,
    revision: int | None,
) -> NarrativeStructureResponse:
    async with session.begin():
        version, _source, _episode = await _script_context(session, version_id)
        await require_resource_access(
            session,
            claims,
            version.workspace_id,
            "Script version",
        )
        structure = await repository.find_structure_by_script(session, version.id)
        if structure is None:
            raise ApiError(
                ErrorCode.DEPENDENCY_UNAVAILABLE,
                "Narrative structure is unavailable",
                status_code=503,
                next_action="review_script_publication",
            )
        response = await _structure_response(session, structure, revision=revision)
    return response


def _impact_response(impact: NarrativeImpactAssessment) -> NarrativeImpactResponse:
    return NarrativeImpactResponse(
        id=impact.id,
        episode_id=impact.episode_id,
        sequence=impact.sequence,
        trigger=cast(Literal["current_changed", "structure_corrected"], impact.trigger),
        episode_revision=impact.episode_revision,
        previous_script_version_id=impact.previous_script_version_id,
        current_script_version_id=impact.current_script_version_id,
        previous_structure_hash=impact.previous_structure_hash,
        current_structure_hash=impact.current_structure_hash,
        previous_dependency_hash=impact.previous_dependency_hash,
        current_dependency_hash=impact.current_dependency_hash,
        previous_unit_count=impact.previous_unit_count,
        current_unit_count=impact.current_unit_count,
        affected_shot_ids=list(impact.affected_shot_ids),
        invalidated_scopes=cast(
            list[Literal["shot_readiness", "coverage", "export"]],
            list(impact.invalidated_scopes),
        ),
        impact_hash=impact.impact_hash,
        created_at=impact.created_at,
    )


async def _append_impact(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    episode_id: UUID,
    episode_revision: int,
    trigger: Literal["current_changed", "structure_corrected"],
    previous_script_version_id: UUID | None,
    current_script_version_id: UUID,
    previous_structure_hash: str | None,
    current_structure_hash: str,
    previous_dependency_hash: str | None,
    current_dependency_hash: str,
    previous_unit_count: int,
    current_unit_count: int,
    affected_shot_ids: list[UUID],
    actor_id: UUID,
) -> NarrativeImpactAssessment:
    sequence = await repository.next_impact_sequence(session, episode_id)
    payload = {
        "episode_id": str(episode_id),
        "sequence": sequence,
        "trigger": trigger,
        "episode_revision": episode_revision,
        "previous_script_version_id": (
            str(previous_script_version_id) if previous_script_version_id else None
        ),
        "current_script_version_id": str(current_script_version_id),
        "previous_structure_hash": previous_structure_hash,
        "current_structure_hash": current_structure_hash,
        "previous_dependency_hash": previous_dependency_hash,
        "current_dependency_hash": current_dependency_hash,
        "previous_unit_count": previous_unit_count,
        "current_unit_count": current_unit_count,
        "affected_shot_ids": [str(item) for item in affected_shot_ids],
        "invalidated_scopes": INVALIDATED_SCOPES,
    }
    impact = NarrativeImpactAssessment(
        id=uuid7(),
        workspace_id=workspace_id,
        episode_id=episode_id,
        sequence=sequence,
        trigger=trigger,
        episode_revision=episode_revision,
        previous_script_version_id=previous_script_version_id,
        current_script_version_id=current_script_version_id,
        previous_structure_hash=previous_structure_hash,
        current_structure_hash=current_structure_hash,
        previous_dependency_hash=previous_dependency_hash,
        current_dependency_hash=current_dependency_hash,
        previous_unit_count=previous_unit_count,
        current_unit_count=current_unit_count,
        affected_shot_ids=affected_shot_ids,
        invalidated_scopes=INVALIDATED_SCOPES,
        impact_hash=canonical_hash(payload),
        created_by=actor_id,
        created_at=datetime.now(UTC),
    )
    session.add(impact)
    await session.flush()
    return impact


async def record_current_impact(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    episode_id: UUID,
    episode_revision: int,
    previous_script_version_id: UUID | None,
    current_script_version_id: UUID,
    affected_shot_ids: list[UUID],
    actor_id: UUID,
) -> NarrativeImpactAssessment:
    previous = (
        await repository.find_structure_by_script(session, previous_script_version_id)
        if previous_script_version_id is not None
        else None
    )
    current = await ensure_structure_for_version(
        session,
        current_script_version_id,
        actor_id=actor_id,
    )
    previous_count = (
        len(await repository.list_versions(session, previous.id, previous.revision))
        if previous is not None
        else 0
    )
    current_count = len(await repository.list_versions(session, current.id, current.revision))
    return await _append_impact(
        session,
        workspace_id=workspace_id,
        episode_id=episode_id,
        episode_revision=episode_revision,
        trigger="current_changed",
        previous_script_version_id=previous_script_version_id,
        current_script_version_id=current_script_version_id,
        previous_structure_hash=previous.structure_hash if previous is not None else None,
        current_structure_hash=current.structure_hash,
        previous_dependency_hash=(previous.dependency_hash if previous is not None else None),
        current_dependency_hash=current.dependency_hash,
        previous_unit_count=previous_count,
        current_unit_count=current_count,
        affected_shot_ids=affected_shot_ids,
        actor_id=actor_id,
    )


async def record_current_impact_snapshot(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    episode_id: UUID,
    episode_revision: int,
    previous_script_version_id: UUID | None,
    current_script_version_id: UUID,
    affected_shot_ids: list[UUID],
    actor_id: UUID,
) -> NarrativeImpactSnapshot:
    impact = await record_current_impact(
        session,
        workspace_id=workspace_id,
        episode_id=episode_id,
        episode_revision=episode_revision,
        previous_script_version_id=previous_script_version_id,
        current_script_version_id=current_script_version_id,
        affected_shot_ids=affected_shot_ids,
        actor_id=actor_id,
    )
    return NarrativeImpactSnapshot(
        impact_id=impact.id,
        previous_dependency_hash=impact.previous_dependency_hash,
        current_dependency_hash=impact.current_dependency_hash,
        invalidated_scopes=tuple(impact.invalidated_scopes),
    )


def _validate_ranges(
    body: str,
    request: NarrativeStructureRevisionRequest,
) -> None:
    previous_end = -1
    for position, item in enumerate(request.units, start=1):
        if item.source_end > len(body) or item.source_end <= item.source_start:
            raise ApiError(
                ErrorCode.INVALID_REQUEST,
                "Narrative unit source range is invalid",
                status_code=422,
                details={"position": position},
            )
        if item.source_start < previous_end:
            raise ApiError(
                ErrorCode.INVALID_REQUEST,
                "Narrative unit source ranges overlap or are out of order",
                status_code=422,
                details={"position": position},
            )
        if not body[item.source_start : item.source_end].strip():
            raise ApiError(
                ErrorCode.INVALID_REQUEST,
                "Narrative unit must reference non-empty source text",
                status_code=422,
                details={"position": position},
            )
        previous_end = item.source_end
    expected_positions = {
        index
        for unit in parse_narrative_units(body)
        for index in range(unit.source_start, unit.source_end)
        if not body[index].isspace()
    }
    requested_positions = {
        index
        for item in request.units
        for index in range(item.source_start, item.source_end)
        if not body[index].isspace()
    }
    if requested_positions != expected_positions:
        raise ApiError(
            ErrorCode.INVALID_REQUEST,
            "Narrative correction must conserve all narrative source content",
            status_code=422,
            details={
                "missing_codepoints": len(expected_positions - requested_positions),
                "unexpected_codepoints": len(requested_positions - expected_positions),
            },
        )


async def revise_structure(
    session: AsyncSession,
    claims: AccessTokenClaims,
    structure_id: UUID,
    request: NarrativeStructureRevisionRequest,
    *,
    trace_id: str,
) -> NarrativeRevisionResponse:
    async with session.begin():
        structure = await repository.find_structure(session, structure_id, for_update=True)
        if structure is None:
            raise resource_not_found("Narrative structure")
        await actor_context(session, claims, structure.workspace_id, Capability.CONTENT_WRITE)
        receipt = structure.command_receipts.get(request.idempotency_key)
        if receipt is not None:
            if receipt.get("command_hash") != canonical_hash(
                request.model_dump(mode="json") | {"idempotency_key": None}
            ):
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Idempotency key was used with different input",
                    status_code=409,
                )
            response = await _structure_response(
                session,
                structure,
                revision=int(receipt["revision"]),
            )
            impact = await session.get(
                NarrativeImpactAssessment,
                UUID(str(receipt["impact_id"])),
            )
            if impact is None:
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR,
                    "Narrative revision receipt is incomplete",
                    status_code=500,
                )
            return NarrativeRevisionResponse(
                structure=response,
                impact=_impact_response(impact),
            )
        if structure.revision != request.expected_revision:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Narrative structure has changed",
                status_code=409,
                details={"current_revision": structure.revision},
            )
        version, source, episode = await _script_context(
            session,
            structure.script_version_id,
        )
        if (
            request.expected_current_script_version_id != structure.script_version_id
            or episode.current_script_version_id != structure.script_version_id
        ):
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Current script version has changed",
                status_code=409,
                details={
                    "current_script_version_id": (
                        str(episode.current_script_version_id)
                        if episode.current_script_version_id is not None
                        else None
                    )
                },
            )
        if source.episode_id != structure.episode_id:
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Narrative structure belongs to another episode",
                status_code=409,
            )
        _validate_ranges(version.body, request)
        units = await repository.find_units(
            session,
            [item.unit_id for item in request.units],
            for_update=True,
        )
        unit_by_id = {unit.id: unit for unit in units}
        if len(unit_by_id) != len(request.units):
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Narrative unit belongs to another structure version",
                status_code=409,
            )
        for item in request.units:
            unit = unit_by_id[item.unit_id]
            if (
                unit.workspace_id != structure.workspace_id
                or unit.episode_id != structure.episode_id
                or unit.kind != item.kind
            ):
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Narrative unit scope or kind cannot be changed",
                    status_code=409,
                )
        previous_versions = await repository.list_versions(
            session,
            structure.id,
            structure.revision,
        )
        previous_version_by_unit = {item.unit_id: item for item in previous_versions}
        if set(previous_version_by_unit) != set(unit_by_id):
            raise ApiError(
                ErrorCode.RESOURCE_CONFLICT,
                "Narrative correction cannot add or remove stable units",
                status_code=409,
            )
        new_revision = structure.revision + 1
        new_versions: list[NarrativeUnitVersion] = []
        hash_units: list[dict[str, object]] = []
        for position, item in enumerate(request.units, start=1):
            unit = unit_by_id[item.unit_id]
            previous_version = previous_version_by_unit[item.unit_id]
            exact = version.body[item.source_start : item.source_end]
            prefix, suffix = _context(version.body, item.source_start, item.source_end)
            new_version = NarrativeUnitVersion(
                id=uuid7(),
                workspace_id=structure.workspace_id,
                episode_id=structure.episode_id,
                structure_id=structure.id,
                script_version_id=structure.script_version_id,
                unit_id=unit.id,
                version_no=previous_version.version_no + 1,
                structure_revision=new_revision,
                position=position,
                source_start=item.source_start,
                source_end=item.source_end,
                exact_text=exact,
                text_hash=sha256(exact.encode("utf-8")).hexdigest(),
                prefix_text=prefix,
                suffix_text=suffix,
                required_for_coverage=item.required_for_coverage,
                payload=previous_version.payload,
                source_scene_id=previous_version.source_scene_id,
                source_dialogue_id=previous_version.source_dialogue_id,
                origin="manual",
                created_by=claims.sub,
            )
            new_versions.append(new_version)
            unit.current_version_id = new_version.id
            unit.revision += 1
            unit.updated_at = datetime.now(UTC)
            hash_units.append(
                _unit_hash_payload(
                    unit_id=unit.id,
                    kind=unit.kind,
                    position=position,
                    source_start=item.source_start,
                    source_end=item.source_end,
                    exact_text=exact,
                    required_for_coverage=item.required_for_coverage,
                )
            )
        session.add_all(new_versions)
        await session.flush()
        old_structure_hash = structure.structure_hash
        old_dependency_hash = structure.dependency_hash
        structure.revision = new_revision
        structure.structure_hash = structure_hash(
            script_version_id=structure.script_version_id,
            revision=new_revision,
            units=hash_units,
        )
        structure.dependency_hash = dependency_hash(
            script_version_id=structure.script_version_id,
            structure_hash_value=structure.structure_hash,
            unit_version_ids=[item.id for item in new_versions],
        )
        structure.updated_at = datetime.now(UTC)
        impact = await _append_impact(
            session,
            workspace_id=structure.workspace_id,
            episode_id=structure.episode_id,
            episode_revision=episode.revision,
            trigger="structure_corrected",
            previous_script_version_id=structure.script_version_id,
            current_script_version_id=structure.script_version_id,
            previous_structure_hash=old_structure_hash,
            current_structure_hash=structure.structure_hash,
            previous_dependency_hash=old_dependency_hash,
            current_dependency_hash=structure.dependency_hash,
            previous_unit_count=len(previous_versions),
            current_unit_count=len(new_versions),
            affected_shot_ids=[],
            actor_id=claims.sub,
        )
        command_hash = canonical_hash(request.model_dump(mode="json") | {"idempotency_key": None})
        structure.command_receipts = {
            **structure.command_receipts,
            request.idempotency_key: {
                "command_hash": command_hash,
                "revision": new_revision,
                "impact_id": str(impact.id),
            },
        }
        append_audit_event(
            session,
            workspace_id=structure.workspace_id,
            actor_id=claims.sub,
            action="script.narrative_structure_corrected",
            target_type="narrative_structure",
            target_id=structure.id,
            trace_id=trace_id,
            metadata={
                "script_version_id": str(structure.script_version_id),
                "revision": structure.revision,
                "unit_count": len(new_versions),
                "impact_id": str(impact.id),
            },
        )
        await session.flush()
        response = await _structure_response(session, structure)
    return NarrativeRevisionResponse(
        structure=response,
        impact=_impact_response(impact),
    )


async def get_dependency(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    *,
    evaluation_hash: str | None,
) -> NarrativeDependencyResponse:
    async with session.begin():
        episode = await episode_for_content_read(session, claims, episode_id)
        if episode.current_script_version_id is None:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Episode has no current script version",
                status_code=409,
            )
        structure = await repository.find_structure_by_script(
            session,
            episode.current_script_version_id,
        )
        if structure is None:
            raise ApiError(
                ErrorCode.DEPENDENCY_UNAVAILABLE,
                "Current narrative structure is unavailable",
                status_code=503,
                next_action="review_script_publication",
            )
        response = NarrativeDependencyResponse(
            episode_id=episode.episode_id,
            current_script_version_id=episode.current_script_version_id,
            current_structure_id=structure.id,
            current_structure_revision=structure.revision,
            current_dependency_hash=structure.dependency_hash,
            evaluated_hash=evaluation_hash,
            status=(
                "fresh"
                if evaluation_hash is None or evaluation_hash == structure.dependency_hash
                else "stale"
            ),
        )
    return response


async def get_latest_impact(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
) -> NarrativeImpactResponse:
    await episode_for_content_read(session, claims, episode_id)
    impact = await repository.latest_impact(session, episode_id)
    if impact is None:
        raise resource_not_found("Narrative impact")
    return _impact_response(impact)
