from typing import Literal, cast
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.assets import resolve_export_assets
from app.modules.identity import Capability, actor_context
from app.modules.production import (
    StoryboardExportTaskCommand,
    create_storyboard_export_task,
)
from app.modules.projects import (
    EpisodeContentContext,
    episode_for_content_read,
    lock_active_episode_for_content_write,
)
from app.modules.scripts import (
    resolve_script_version_snapshot,
    resolve_storyboard_narrative,
)
from app.modules.storyboards import repository as storyboard_repository
from app.modules.storyboards import service as storyboard_service
from app.modules.storyboards.coverage import service as coverage_service
from app.modules.storyboards.exports import repository
from app.modules.storyboards.exports.contracts import (
    ExportAsset,
    ExportAssetReference,
    ExportBlocker,
    ExportNarrativeReference,
    ExportPreparation,
    ExportShot,
    ExportSnapshot,
    ExportUnit,
)
from app.modules.storyboards.exports.models import (
    StoryboardExportJob,
    StoryboardExportManifest,
)
from app.modules.storyboards.exports.schemas import (
    ExportBlockerResponse,
    ExportHistoryResponse,
    ExportManifestResponse,
    ExportPreflightResponse,
    ExportRequest,
    ExportResponse,
)
from app.modules.storyboards.hashing import canonical_payload_hash
from app.modules.storyboards.schemas import ShotSpec


def _blocker(
    *,
    code: str,
    summary: str,
    next_action: str,
    shot_id: UUID | None = None,
    dependency_id: UUID | None = None,
) -> ExportBlocker:
    return ExportBlocker(
        code=code,
        summary=summary,
        next_action=next_action,
        shot_id=shot_id,
        dependency_id=dependency_id,
    )


def _prompt(spec: ShotSpec) -> str:
    visual = spec.visual
    action = "；".join(beat.description for beat in spec.action_beats)
    return "，".join(
        (
            visual.environment,
            visual.composition,
            action,
            visual.mood_lighting,
            f"{visual.shot_size}/{visual.camera_angle}/{visual.camera_movement}",
        )
    )


async def _prepare(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode: EpisodeContentContext,
) -> ExportPreparation:
    coverage = await coverage_service.resolve_report(session, episode)
    readiness = await storyboard_service.get_episode_readiness(
        session,
        claims,
        episode.episode_id,
    )
    blockers: list[ExportBlocker] = []
    if coverage.status == "unavailable":
        blockers.append(
            _blocker(
                code="COVERAGE_DEPENDENCY_UNAVAILABLE",
                summary="Coverage dependencies are unavailable",
                next_action="retry_coverage",
            )
        )
    elif not coverage.ready:
        if coverage.summary.uncovered:
            blockers.append(
                _blocker(
                    code="COVERAGE_UNACCOUNTED",
                    summary="Required narrative units are not fully accounted for",
                    next_action="map_or_omit_narrative_units",
                )
            )
        if coverage.summary.orphan:
            blockers.append(
                _blocker(
                    code="SHOT_SOURCE_ORPHAN",
                    summary="Active shots have no approved narrative source",
                    next_action="map_or_approve_invented_shots",
                )
            )
        if coverage.summary.stale:
            blockers.append(
                _blocker(
                    code="COVERAGE_STALE",
                    summary="Coverage evidence is stale",
                    next_action="review_stale_coverage",
                )
            )
    for item in readiness.items:
        for issue in item.blocking_reasons:
            blockers.append(
                _blocker(
                    code=issue.code,
                    summary=issue.summary,
                    next_action=issue.next_action,
                    shot_id=item.shot_id,
                    dependency_id=issue.dependency_id,
                )
            )
    blockers = list(
        {
            (
                value.code,
                value.shot_id,
                value.dependency_id,
            ): value
            for value in blockers
        }.values()
    )
    if blockers:
        status: Literal["blocked", "unavailable"] = (
            "unavailable"
            if coverage.status == "unavailable" or readiness.summary.unavailable
            else "blocked"
        )
        return ExportPreparation(
            status=status,
            snapshot=None,
            input_hash=None,
            blockers=tuple(blockers),
        )
    if episode.current_script_version_id is None:
        return ExportPreparation(
            status="unavailable",
            snapshot=None,
            blockers=(
                _blocker(
                    code="SCRIPT_VERSION_UNAVAILABLE",
                    summary="Current script version is unavailable",
                    next_action="set_current_script_version",
                ),
            ),
        )

    script = await resolve_script_version_snapshot(
        session,
        episode.workspace_id,
        episode.episode_id,
        episode.current_script_version_id,
    )
    narrative = await resolve_storyboard_narrative(
        session,
        episode.workspace_id,
        episode.current_script_version_id,
    )
    if script is None or narrative is None:
        return ExportPreparation(
            status="unavailable",
            snapshot=None,
            blockers=(
                _blocker(
                    code="SCRIPT_VERSION_UNAVAILABLE",
                    summary="Script or narrative structure is unavailable",
                    next_action="retry_export_preflight",
                ),
            ),
        )

    rows = await storyboard_repository.list_active_shots_with_current_specs(
        session,
        episode.episode_id,
    )
    versions = [version for _shot, version in rows if version is not None]
    if len(versions) != len(rows):
        return ExportPreparation(
            status="blocked",
            snapshot=None,
            blockers=(
                _blocker(
                    code="CURRENT_SPEC_MISSING",
                    summary="Every active shot requires a current immutable spec",
                    next_action="complete_shot_specs",
                ),
            ),
        )
    version_ids = [version.id for version in versions]
    asset_references = await storyboard_repository.list_asset_references(
        session,
        version_ids,
    )
    narrative_references = [
        reference
        for reference in coverage.references
        if reference.shot_spec_version_id in set(version_ids)
    ]
    asset_version_ids = list(
        dict.fromkeys(reference.asset_version_id for reference in asset_references)
    )
    assets = await resolve_export_assets(
        session,
        episode.workspace_id,
        episode.project_id,
        asset_version_ids,
    )
    if len(assets) != len(asset_version_ids) or any(
        value.readiness_status != "ready" for value in assets
    ):
        return ExportPreparation(
            status="blocked",
            snapshot=None,
            blockers=(
                _blocker(
                    code="ASSET_NOT_READY",
                    summary="Fixed asset versions are not ready for export",
                    next_action="review_asset_readiness",
                ),
            ),
        )
    readiness_by_shot = {item.shot_id: item for item in readiness.items}
    assets_by_version = {value.asset_version_id: value for value in assets}
    asset_refs_by_spec: dict[UUID, list[ExportAssetReference]] = {}
    for value in asset_references:
        asset_refs_by_spec.setdefault(value.shot_spec_version_id, []).append(
            ExportAssetReference(
                slot_key=value.slot_key,
                role=value.role,
                asset_id=value.asset_id,
                asset_state_id=value.asset_state_id,
                asset_version_id=value.asset_version_id,
                binding_source=cast(Literal["manual", "ai"], value.binding_source),
                subject_key=value.subject_key,
            )
        )
    narrative_refs_by_spec: dict[UUID, list[ExportNarrativeReference]] = {}
    for value in narrative_references:
        narrative_refs_by_spec.setdefault(value.shot_spec_version_id, []).append(
            ExportNarrativeReference(
                reference_id=value.id,
                narrative_unit_id=value.narrative_unit_id,
                unit_version_id=value.unit_version_id,
                channel=value.channel,
                role=value.role,
                coverage_mode=value.coverage_mode,
                segment_start=value.segment_start,
                segment_end=value.segment_end,
                contribution=value.contribution,
                origin=value.origin,
            )
        )
    shots: list[ExportShot] = []
    for shot, version in rows:
        assert version is not None
        spec = ShotSpec.model_validate(version.spec)
        shot_readiness = readiness_by_shot[shot.id]
        shots.append(
            ExportShot(
                shot_id=shot.id,
                shot_spec_version_id=version.id,
                position=shot.position,
                title=shot.title,
                spec_version_no=version.version_no,
                spec_content_hash=version.content_hash,
                spec_input_hash=version.input_hash,
                spec=spec,
                prompt=_prompt(spec),
                readiness_hash=shot_readiness.evaluation_hash,
                asset_references=tuple(asset_refs_by_spec.get(version.id, [])),
                narrative_references=tuple(narrative_refs_by_spec.get(version.id, [])),
            )
        )
    snapshot = ExportSnapshot(
        workspace_id=episode.workspace_id,
        project_id=episode.project_id,
        episode_id=episode.episode_id,
        script_version_id=script.version_id,
        script_content_hash=script.content_hash,
        narrative_structure_id=narrative.structure_id,
        narrative_structure_revision=narrative.structure_revision,
        narrative_dependency_hash=narrative.dependency_hash,
        coverage_basis_hash=coverage.basis_hash,
        coverage_evaluation_hash=coverage.evaluation_hash,
        readiness_evaluation_hash=readiness.evaluation_hash,
        units=tuple(
            ExportUnit(
                narrative_unit_id=value.narrative_unit_id,
                unit_version_id=value.unit_version_id,
                position=value.position,
                kind=value.kind,
                exact_text=value.exact_text,
                text_hash=next(
                    unit.text_hash
                    for unit in narrative.units
                    if unit.unit_version_id == value.unit_version_id
                ),
                required_for_coverage=value.required_for_coverage,
                coverage_status=value.status,
            )
            for value in coverage.units
        ),
        assets=tuple(
            ExportAsset(
                asset_id=value.asset_id,
                asset_state_id=value.asset_state_id,
                asset_version_id=value.asset_version_id,
                kind=value.kind,
                name=value.name,
                state_label=value.state_label,
                state_revision=value.state_revision,
                content_hash=value.content_hash,
                readiness_hash=value.readiness_hash,
            )
            for value in (assets_by_version[version_id] for version_id in asset_version_ids)
        ),
        shots=tuple(shots),
    )
    input_hash = canonical_payload_hash(snapshot.model_dump(mode="json"))
    return ExportPreparation(
        status="ready",
        snapshot=snapshot,
        input_hash=input_hash,
        blockers=(),
    )


def _preflight_response(
    episode_id: UUID,
    value: ExportPreparation,
) -> ExportPreflightResponse:
    snapshot = value.snapshot
    return ExportPreflightResponse(
        episode_id=episode_id,
        status=value.status,
        input_hash=value.input_hash,
        script_version_id=snapshot.script_version_id if snapshot else None,
        narrative_structure_id=snapshot.narrative_structure_id if snapshot else None,
        narrative_unit_version_ids=(
            [unit.unit_version_id for unit in snapshot.units] if snapshot else []
        ),
        shot_spec_version_ids=(
            [shot.shot_spec_version_id for shot in snapshot.shots] if snapshot else []
        ),
        asset_version_ids=(
            [asset.asset_version_id for asset in snapshot.assets] if snapshot else []
        ),
        coverage_basis_hash=snapshot.coverage_basis_hash if snapshot else None,
        coverage_evaluation_hash=(snapshot.coverage_evaluation_hash if snapshot else None),
        readiness_evaluation_hash=(snapshot.readiness_evaluation_hash if snapshot else None),
        blockers=[
            ExportBlockerResponse.model_validate(item.model_dump()) for item in value.blockers
        ],
    )


async def preflight_export(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
) -> ExportPreflightResponse:
    episode = await episode_for_content_read(session, claims, episode_id)
    return _preflight_response(
        episode_id,
        await _prepare(session, claims, episode),
    )


def _manifest_response(value: StoryboardExportManifest) -> ExportManifestResponse:
    snapshot = ExportSnapshot.model_validate(value.input_snapshot)
    payload = dict(value.file_manifest)
    return ExportManifestResponse(
        id=value.id,
        schema_version=value.schema_version,
        input_hash=value.input_hash,
        script_version_id=snapshot.script_version_id,
        narrative_structure_id=snapshot.narrative_structure_id,
        narrative_unit_version_ids=[unit.unit_version_id for unit in snapshot.units],
        shot_spec_version_ids=[shot.shot_spec_version_id for shot in snapshot.shots],
        asset_version_ids=[asset.asset_version_id for asset in snapshot.assets],
        coverage_basis_hash=snapshot.coverage_basis_hash,
        coverage_evaluation_hash=snapshot.coverage_evaluation_hash,
        files=payload.get("files", []),
        media_version_id=value.media_version_id,
        package_sha256=value.package_sha256,
        package_size_bytes=value.package_size_bytes,
        created_at=value.created_at,
    )


def _job_response(
    job: StoryboardExportJob,
    manifest: StoryboardExportManifest | None,
) -> ExportResponse:
    return ExportResponse(
        id=job.id,
        episode_id=job.episode_id,
        status=cast(Literal["queued", "running", "succeeded", "failed"], job.status),
        input_hash=job.input_hash,
        task_id=job.task_id,
        error_code=job.error_code,
        manifest=_manifest_response(manifest) if manifest is not None else None,
        created_at=job.created_at,
        updated_at=job.updated_at,
    )


async def request_export(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    request: ExportRequest,
    *,
    trace_id: str,
) -> ExportResponse:
    async with session.begin():
        episode = await lock_active_episode_for_content_write(
            session,
            claims,
            episode_id,
        )
        existing = await repository.find_job_by_key(
            session,
            episode_id,
            request.idempotency_key,
        )
        if existing is not None:
            if existing.input_hash != request.expected_input_hash:
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Idempotency key was used with different input",
                    status_code=409,
                    details={"reason": "idempotency_key_reused"},
                )
            manifests = await repository.find_manifests(session, [existing.id])
            return _job_response(existing, manifests.get(existing.id))
        preparation = await _prepare(session, claims, episode)
        if preparation.input_hash != request.expected_input_hash:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Storyboard export input has changed",
                status_code=409,
                next_action="refresh_export_preflight",
                details={"reason": "export_input_changed"},
            )
        if preparation.status == "unavailable":
            raise ApiError(
                ErrorCode.DEPENDENCY_UNAVAILABLE,
                "Storyboard export dependencies are unavailable",
                status_code=503,
                next_action="retry_export_preflight",
            )
        if preparation.status != "ready" or preparation.snapshot is None:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Storyboard export is blocked",
                status_code=409,
                next_action="review_export_blockers",
                details={"reason": "export_blocked"},
            )
        snapshot = preparation.snapshot
        input_hash = preparation.input_hash
        assert input_hash is not None
        command_hash = canonical_payload_hash(request.model_dump(mode="json"))
        job = StoryboardExportJob(
            id=uuid7(),
            workspace_id=episode.workspace_id,
            project_id=episode.project_id,
            episode_id=episode.episode_id,
            schema_version=1,
            input_hash=input_hash,
            input_snapshot=snapshot.model_dump(mode="json"),
            command_hash=command_hash,
            idempotency_key=request.idempotency_key,
            status="queued",
            created_by=claims.sub,
        )
        session.add(job)
        await session.flush()
        actor = await actor_context(
            session,
            claims,
            episode.workspace_id,
            Capability.CONTENT_WRITE,
        )
        task = await create_storyboard_export_task(
            session,
            actor,
            StoryboardExportTaskCommand(
                workspace_id=episode.workspace_id,
                episode_id=episode.episode_id,
                job_id=job.id,
                input_version_id=snapshot.script_version_id,
                input_hash=input_hash,
                idempotency_key=f"storyboard-export:{job.id}",
            ),
            trace_id=trace_id,
        )
        job.task_id = task.id
        await session.flush()
    return _job_response(job, None)


async def list_exports(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
) -> ExportHistoryResponse:
    await episode_for_content_read(session, claims, episode_id)
    jobs, total = await repository.list_jobs(session, episode_id)
    manifests = await repository.find_manifests(session, [job.id for job in jobs])
    return ExportHistoryResponse(
        items=[_job_response(job, manifests.get(job.id)) for job in jobs],
        total=total,
    )
