import hashlib
import json
from dataclasses import dataclass
from datetime import UTC, datetime
from difflib import unified_diff
from typing import Literal, cast
from uuid import UUID

from pydantic import ValidationError
from sqlalchemy.dialects.postgresql import insert
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.audit import append_audit_event
from app.modules.identity import Capability, actor_context
from app.modules.production import (
    ScriptAdaptationTaskCommand,
    cancel_script_adaptation_task,
    create_script_adaptation_task,
)
from app.modules.projects import (
    compare_and_set_current_script_version,
    lock_active_episode_for_content_write,
    lock_episode_content_context,
)
from app.modules.scripts import repository as scripts_repository
from app.modules.scripts.adaptations import repository
from app.modules.scripts.adaptations.ports import (
    SCRIPT_ADAPTATION_ENGINE_VERSION,
    SCRIPT_ADAPTATION_PROMPT_VERSION,
    SCRIPT_ADAPTATION_SCHEMA_VERSION,
    adaptation_duration_bounds,
)
from app.modules.scripts.adaptations.schemas import (
    AdaptationCancelRequest,
    AdaptationConstraintsResponse,
    AdaptationDiffResponse,
    AdaptationDraftUpdateRequest,
    AdaptationPublishRequest,
    AdaptationPublishResponse,
    AdaptationRunCreateRequest,
    AdaptationRunResponse,
    ScriptAdaptationProviderResult,
)
from app.modules.scripts.authorization import require_resource_access, resource_not_found
from app.modules.scripts.contracts import (
    NarrativeImpactRecorder,
    ScriptVersionImpactReader,
)
from app.modules.scripts.models import AdaptationRun, ScriptVersion
from app.modules.scripts.versions.schemas import (
    CurrentScriptVersionResponse,
    ScriptVersionImpactResponse,
    ScriptVersionResponse,
)

ADAPTATION_MODEL_NAME = "codex-local"


@dataclass(frozen=True, slots=True)
class AdaptationInput:
    run_id: UUID
    task_id: UUID
    script_body: str
    target_duration_ms: int
    core_plot_points: list[str]
    pacing: str
    colloquial_dialogue: bool


class AdaptationInputChanged(RuntimeError):
    pass


def _run_response(run: AdaptationRun) -> AdaptationRunResponse:
    return AdaptationRunResponse(
        id=run.id,
        workspace_id=run.workspace_id,
        episode_id=run.episode_id,
        source_id=run.source_id,
        input_script_version_id=run.input_script_version_id,
        input_hash=run.input_hash,
        constraints=AdaptationConstraintsResponse(
            target_duration_ms=run.target_duration_ms,
            core_plot_points=list(run.core_plot_points),
            pacing=cast(Literal["slow", "balanced", "fast"], run.pacing),
            colloquial_dialogue=run.colloquial_dialogue,
        ),
        status=cast(
            Literal[
                "queued",
                "running",
                "succeeded",
                "published",
                "failed",
                "cancelled",
                "unknown",
            ],
            run.status,
        ),
        revision=run.revision,
        task_id=run.task_id,
        candidate_body=run.candidate_body,
        candidate_hash=run.candidate_hash,
        draft_body=run.draft_body,
        draft_hash=run.draft_hash,
        change_summary=run.change_summary,
        estimated_duration_ms=run.estimated_duration_ms,
        error_code=run.error_code,
        published_script_version_id=run.published_script_version_id,
        created_at=run.created_at,
        updated_at=run.updated_at,
    )


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


def _same_create(run: AdaptationRun, request: AdaptationRunCreateRequest) -> bool:
    return (
        run.input_script_version_id == request.input_script_version_id
        and run.target_duration_ms == request.target_duration_ms
        and run.core_plot_points == request.core_plot_points
        and run.pacing == request.pacing
        and run.colloquial_dialogue == request.colloquial_dialogue
    )


def _command_hash(payload: dict[str, object]) -> str:
    serialized = json.dumps(payload, sort_keys=True, separators=(",", ":"))
    return hashlib.sha256(serialized.encode("utf-8")).hexdigest()


def _revision_conflict(current_revision: int) -> ApiError:
    return ApiError(
        ErrorCode.VERSION_CONFLICT,
        "Adaptation run has changed",
        status_code=409,
        details={"current_revision": current_revision},
    )


async def create_run(
    session: AsyncSession,
    claims: AccessTokenClaims,
    episode_id: UUID,
    request: AdaptationRunCreateRequest,
    *,
    trace_id: str,
) -> AdaptationRunResponse:
    now = datetime.now(UTC)
    async with session.begin():
        episode = await lock_active_episode_for_content_write(session, claims, episode_id)
        actor = await actor_context(
            session,
            claims,
            episode.workspace_id,
            Capability.CONTENT_WRITE,
        )
        version = await scripts_repository.find_version(
            session,
            request.input_script_version_id,
        )
        if version is None:
            raise resource_not_found("Script version")
        source = await scripts_repository.find_source(session, version.source_id)
        if (
            version.workspace_id != episode.workspace_id
            or version.status != "published"
            or source is None
            or source.workspace_id != episode.workspace_id
            or source.episode_id != episode.episode_id
            or source.status != "active"
        ):
            raise resource_not_found("Script version")
        if episode.current_script_version_id != version.id:
            raise ApiError(
                ErrorCode.VERSION_CONFLICT,
                "Adaptation input is not the current script version",
                status_code=409,
                details={
                    "current_script_version_id": (
                        str(episode.current_script_version_id)
                        if episode.current_script_version_id is not None
                        else None
                    )
                },
            )

        run_id = uuid7()
        inserted_id = await session.scalar(
            insert(AdaptationRun)
            .values(
                id=run_id,
                workspace_id=episode.workspace_id,
                episode_id=episode.episode_id,
                source_id=source.id,
                input_script_version_id=version.id,
                input_hash=version.content_hash,
                target_duration_ms=request.target_duration_ms,
                core_plot_points=request.core_plot_points,
                pacing=request.pacing,
                colloquial_dialogue=request.colloquial_dialogue,
                adaptation_engine_version=SCRIPT_ADAPTATION_ENGINE_VERSION,
                model_name=ADAPTATION_MODEL_NAME,
                prompt_version=SCRIPT_ADAPTATION_PROMPT_VERSION,
                schema_version=SCRIPT_ADAPTATION_SCHEMA_VERSION,
                status="queued",
                revision=1,
                idempotency_key=request.idempotency_key,
                created_by=claims.sub,
                created_at=now,
                updated_at=now,
            )
            .on_conflict_do_nothing(constraint="uq_scr_adaptation_episode_idempotency")
            .returning(AdaptationRun.id)
        )
        if inserted_id is None:
            existing = await repository.find_run_by_idempotency(
                session,
                episode.episode_id,
                request.idempotency_key,
            )
            if existing is None:
                raise ApiError(
                    ErrorCode.INTERNAL_ERROR,
                    "Adaptation run state is unavailable",
                    status_code=500,
                )
            if not _same_create(existing, request):
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Idempotency key was used with different input",
                    status_code=409,
                )
            return _run_response(existing)

        task = await create_script_adaptation_task(
            session,
            actor,
            ScriptAdaptationTaskCommand(
                workspace_id=episode.workspace_id,
                episode_id=episode.episode_id,
                run_id=inserted_id,
                input_version_id=version.id,
                input_hash=version.content_hash,
                idempotency_key=f"adaptation:{inserted_id}",
            ),
            trace_id=trace_id,
        )
        run = await repository.find_run(session, inserted_id, for_update=True)
        if run is None:
            raise ApiError(
                ErrorCode.INTERNAL_ERROR,
                "Adaptation run state is unavailable",
                status_code=500,
            )
        run.task_id = task.id
        append_audit_event(
            session,
            workspace_id=run.workspace_id,
            actor_id=claims.sub,
            action="script.adaptation_created",
            target_type="adaptation_run",
            target_id=run.id,
            trace_id=trace_id,
            metadata={
                "episode_id": str(run.episode_id),
                "input_script_version_id": str(run.input_script_version_id),
                "task_id": str(task.id),
                "revision": run.revision,
            },
            occurred_at=now,
        )
        await session.flush()
    return _run_response(run)


async def get_run(
    session: AsyncSession,
    claims: AccessTokenClaims,
    run_id: UUID,
) -> AdaptationRunResponse:
    run = await repository.find_run(session, run_id)
    if run is None:
        raise resource_not_found("Adaptation run")
    await require_resource_access(session, claims, run.workspace_id, "Adaptation run")
    return _run_response(run)


async def update_draft(
    session: AsyncSession,
    claims: AccessTokenClaims,
    run_id: UUID,
    request: AdaptationDraftUpdateRequest,
    *,
    trace_id: str,
) -> AdaptationRunResponse:
    now = datetime.now(UTC)
    async with session.begin():
        run = await repository.find_run(session, run_id, for_update=True)
        if run is None:
            raise resource_not_found("Adaptation run")
        await actor_context(session, claims, run.workspace_id, Capability.CONTENT_WRITE)
        if run.revision != request.expected_revision:
            raise _revision_conflict(run.revision)
        if run.status != "succeeded" or run.candidate_body is None:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Adaptation draft is not editable",
                status_code=409,
            )
        run.draft_body = request.body
        run.draft_hash = hashlib.sha256(request.body.encode("utf-8")).hexdigest()
        run.revision += 1
        run.updated_at = now
        append_audit_event(
            session,
            workspace_id=run.workspace_id,
            actor_id=claims.sub,
            action="script.adaptation_draft_updated",
            target_type="adaptation_run",
            target_id=run.id,
            trace_id=trace_id,
            metadata={"revision": run.revision, "draft_hash": run.draft_hash},
            occurred_at=now,
        )
        await session.flush()
    return _run_response(run)


async def diff_run(
    session: AsyncSession,
    claims: AccessTokenClaims,
    run_id: UUID,
) -> AdaptationDiffResponse:
    run = await repository.find_run(session, run_id)
    if run is None:
        raise resource_not_found("Adaptation run")
    await require_resource_access(session, claims, run.workspace_id, "Adaptation run")
    version = await scripts_repository.find_version(session, run.input_script_version_id)
    if version is None or version.workspace_id != run.workspace_id:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Adaptation input is unavailable",
            status_code=500,
        )
    if run.draft_body is None:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Adaptation draft is unavailable",
            status_code=409,
        )
    lines = list(
        unified_diff(
            version.body.splitlines(),
            run.draft_body.splitlines(),
            fromfile=f"script-version:{version.id}",
            tofile=f"adaptation-run:{run.id}",
            lineterm="",
        )
    )
    return AdaptationDiffResponse(
        base_version_id=version.id,
        adaptation_run_id=run.id,
        added_lines=sum(1 for line in lines if line.startswith("+") and not line.startswith("+++")),
        removed_lines=sum(
            1 for line in lines if line.startswith("-") and not line.startswith("---")
        ),
        diff_lines=lines,
    )


async def publish_run(
    session: AsyncSession,
    claims: AccessTokenClaims,
    run_id: UUID,
    request: AdaptationPublishRequest,
    impact_reader: ScriptVersionImpactReader,
    narrative_impact_recorder: NarrativeImpactRecorder,
    *,
    trace_id: str,
) -> AdaptationPublishResponse:
    command_hash = _command_hash(
        {
            "expected_run_revision": request.expected_run_revision,
            "expected_current_version_id": str(request.expected_current_version_id),
        }
    )
    now = datetime.now(UTC)
    async with session.begin():
        run = await repository.find_run(session, run_id, for_update=True)
        if run is None:
            raise resource_not_found("Adaptation run")
        await actor_context(session, claims, run.workspace_id, Capability.CONTENT_WRITE)
        if run.publish_idempotency_key == request.idempotency_key:
            if run.publish_command_hash != command_hash or run.status != "published":
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Idempotency key was used with different input",
                    status_code=409,
                )
            return await _published_response(session, run)
        if run.revision != request.expected_run_revision:
            raise _revision_conflict(run.revision)
        if run.status != "succeeded" or run.draft_body is None:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Adaptation run is not publishable",
                status_code=409,
            )
        source = await scripts_repository.find_source(session, run.source_id, for_update=True)
        if source is None or source.status != "active" or source.workspace_id != run.workspace_id:
            raise resource_not_found("Script source")
        version = ScriptVersion(
            id=uuid7(),
            workspace_id=run.workspace_id,
            source_id=source.id,
            version_no=await scripts_repository.latest_version_number(session, source.id) + 1,
            status="published",
            body=run.draft_body,
            content_hash=run.draft_hash
            or hashlib.sha256(run.draft_body.encode("utf-8")).hexdigest(),
            structure_summary={},
            created_by=claims.sub,
            created_at=now,
        )
        current = await compare_and_set_current_script_version(
            session,
            claims,
            run.episode_id,
            request.expected_current_version_id,
            version.id,
        )
        session.add(version)
        await session.flush()
        affected_shot_ids = await impact_reader(
            episode_id=run.episode_id,
            current_script_version_id=version.id,
        )
        narrative_impact = await narrative_impact_recorder(
            workspace_id=run.workspace_id,
            episode_id=run.episode_id,
            episode_revision=current.revision,
            previous_script_version_id=request.expected_current_version_id,
            current_script_version_id=version.id,
            affected_shot_ids=affected_shot_ids,
            actor_id=claims.sub,
        )
        run.status = "published"
        run.published_script_version_id = version.id
        run.publish_idempotency_key = request.idempotency_key
        run.publish_command_hash = command_hash
        run.publish_result_snapshot = {
            "previous_script_version_id": str(request.expected_current_version_id),
            "current_script_version_id": str(version.id),
            "episode_revision": current.revision,
            "affected_shot_ids": [str(item) for item in affected_shot_ids],
            "narrative_impact_id": str(narrative_impact.impact_id),
            "previous_narrative_dependency_hash": (narrative_impact.previous_dependency_hash),
            "current_narrative_dependency_hash": (narrative_impact.current_dependency_hash),
            "invalidated_scopes": list(narrative_impact.invalidated_scopes),
        }
        run.revision += 1
        run.updated_at = now
        append_audit_event(
            session,
            workspace_id=run.workspace_id,
            actor_id=claims.sub,
            action="script.adaptation_published",
            target_type="adaptation_run",
            target_id=run.id,
            trace_id=trace_id,
            metadata={
                "revision": run.revision,
                "published_script_version_id": str(version.id),
                "episode_revision": current.revision,
            },
            occurred_at=now,
        )
        await session.flush()
    return AdaptationPublishResponse(
        run=_run_response(run),
        version=_version_response(version),
        current=CurrentScriptVersionResponse(
            episode_id=run.episode_id,
            current_script_version_id=version.id,
            episode_revision=current.revision,
            impact=ScriptVersionImpactResponse(
                previous_script_version_id=request.expected_current_version_id,
                current_script_version_id=version.id,
                affected_shot_ids=affected_shot_ids,
                narrative_impact_id=narrative_impact.impact_id,
                previous_narrative_dependency_hash=(narrative_impact.previous_dependency_hash),
                current_narrative_dependency_hash=(narrative_impact.current_dependency_hash),
                invalidated_scopes=cast(
                    list[Literal["shot_readiness", "coverage", "export"]],
                    list(narrative_impact.invalidated_scopes),
                ),
            ),
        ),
    )


async def _published_response(
    session: AsyncSession,
    run: AdaptationRun,
) -> AdaptationPublishResponse:
    if run.published_script_version_id is None:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Published adaptation state is unavailable",
            status_code=500,
        )
    version = await scripts_repository.find_version(session, run.published_script_version_id)
    snapshot = run.publish_result_snapshot
    if version is None or not snapshot:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Published adaptation state is unavailable",
            status_code=500,
        )
    previous = snapshot.get("previous_script_version_id")
    current_version_id = UUID(cast(str, snapshot["current_script_version_id"]))
    episode_revision = cast(int, snapshot["episode_revision"])
    affected_snapshot = cast(list[str], snapshot["affected_shot_ids"])
    narrative_impact_id = UUID(cast(str, snapshot["narrative_impact_id"]))
    previous_narrative_hash = cast(
        str | None,
        snapshot["previous_narrative_dependency_hash"],
    )
    current_narrative_hash = cast(
        str,
        snapshot["current_narrative_dependency_hash"],
    )
    invalidated_scopes = cast(
        list[Literal["shot_readiness", "coverage", "export"]],
        snapshot["invalidated_scopes"],
    )
    return AdaptationPublishResponse(
        run=_run_response(run),
        version=_version_response(version),
        current=CurrentScriptVersionResponse(
            episode_id=run.episode_id,
            current_script_version_id=current_version_id,
            episode_revision=episode_revision,
            impact=ScriptVersionImpactResponse(
                previous_script_version_id=UUID(str(previous)) if previous else None,
                current_script_version_id=current_version_id,
                affected_shot_ids=[UUID(item) for item in affected_snapshot],
                narrative_impact_id=narrative_impact_id,
                previous_narrative_dependency_hash=previous_narrative_hash,
                current_narrative_dependency_hash=current_narrative_hash,
                invalidated_scopes=invalidated_scopes,
            ),
        ),
    )


async def cancel_run(
    session: AsyncSession,
    claims: AccessTokenClaims,
    run_id: UUID,
    request: AdaptationCancelRequest,
    *,
    trace_id: str,
) -> AdaptationRunResponse:
    command_hash = _command_hash({"expected_revision": request.expected_revision})
    now = datetime.now(UTC)
    async with session.begin():
        run = await repository.find_run(session, run_id, for_update=True)
        if run is None:
            raise resource_not_found("Adaptation run")
        await actor_context(session, claims, run.workspace_id, Capability.CONTENT_WRITE)
        if run.cancel_idempotency_key == request.idempotency_key:
            if run.cancel_command_hash != command_hash or run.status != "cancelled":
                raise ApiError(
                    ErrorCode.RESOURCE_CONFLICT,
                    "Idempotency key was used with different input",
                    status_code=409,
                )
            return _run_response(run)
        if run.revision != request.expected_revision:
            raise _revision_conflict(run.revision)
        if run.status != "queued" or run.task_id is None:
            raise ApiError(
                ErrorCode.STATE_CONFLICT,
                "Only a queued adaptation run can be cancelled",
                status_code=409,
            )
        await cancel_script_adaptation_task(
            session,
            run.task_id,
            now=now,
            trace_id=trace_id,
        )
        run.status = "cancelled"
        run.cancel_idempotency_key = request.idempotency_key
        run.cancel_command_hash = command_hash
        run.revision += 1
        run.updated_at = now
        append_audit_event(
            session,
            workspace_id=run.workspace_id,
            actor_id=claims.sub,
            action="script.adaptation_cancelled",
            target_type="adaptation_run",
            target_id=run.id,
            trace_id=trace_id,
            metadata={"revision": run.revision, "task_id": str(run.task_id)},
            occurred_at=now,
        )
        await session.flush()
    return _run_response(run)


async def prepare_adaptation_input(
    session: AsyncSession,
    *,
    run_id: UUID,
    task_id: UUID,
) -> AdaptationInput | None:
    run = await repository.find_run(session, run_id, for_update=True)
    if run is None or run.task_id != task_id:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Adaptation run state is unavailable",
            status_code=500,
        )
    if run.status in {"cancelled", "failed", "unknown", "succeeded", "published"}:
        return None
    context = await lock_episode_content_context(session, run.workspace_id, run.episode_id)
    version = await scripts_repository.find_version(session, run.input_script_version_id)
    source = await scripts_repository.find_source(session, run.source_id)
    if (
        context is None
        or context.current_script_version_id != run.input_script_version_id
        or version is None
        or version.workspace_id != run.workspace_id
        or version.source_id != run.source_id
        or version.status != "published"
        or version.content_hash != run.input_hash
        or hashlib.sha256(version.body.encode("utf-8")).hexdigest() != run.input_hash
        or source is None
        or source.episode_id != run.episode_id
        or source.status != "active"
    ):
        raise AdaptationInputChanged
    run.status = "running"
    run.updated_at = datetime.now(UTC)
    await session.flush()
    return AdaptationInput(
        run_id=run.id,
        task_id=task_id,
        script_body=version.body,
        target_duration_ms=run.target_duration_ms,
        core_plot_points=list(run.core_plot_points),
        pacing=run.pacing,
        colloquial_dialogue=run.colloquial_dialogue,
    )


async def record_adaptation_result(
    session: AsyncSession,
    *,
    run_id: UUID,
    task_id: UUID,
    result: ScriptAdaptationProviderResult | dict[str, object],
) -> None:
    try:
        validated = ScriptAdaptationProviderResult.model_validate(result)
    except ValidationError as error:
        raise ApiError(
            ErrorCode.INVALID_REQUEST,
            "AI returned an invalid adaptation result",
            status_code=422,
        ) from error
    run = await repository.find_run(session, run_id, for_update=True)
    if run is None or run.task_id != task_id or run.status != "running":
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Adaptation run cannot accept a result",
            status_code=409,
        )
    duration_lower_ms, duration_upper_ms = adaptation_duration_bounds(run.target_duration_ms)
    if not duration_lower_ms <= validated.estimated_duration_ms <= duration_upper_ms:
        raise ApiError(
            ErrorCode.INVALID_REQUEST,
            "AI returned an adaptation outside the target duration range",
            status_code=422,
        )
    context = await lock_episode_content_context(session, run.workspace_id, run.episode_id)
    if context is None or context.current_script_version_id != run.input_script_version_id:
        raise AdaptationInputChanged
    candidate_hash = hashlib.sha256(validated.adapted_script_text.encode("utf-8")).hexdigest()
    run.candidate_body = validated.adapted_script_text
    run.candidate_hash = candidate_hash
    run.draft_body = validated.adapted_script_text
    run.draft_hash = candidate_hash
    run.change_summary = validated.change_summary
    run.estimated_duration_ms = validated.estimated_duration_ms
    run.status = "succeeded"
    run.error_code = None
    run.revision += 1
    run.updated_at = datetime.now(UTC)
    await session.flush()


async def record_adaptation_error(
    session: AsyncSession,
    *,
    run_id: UUID,
    task_id: UUID,
    error_code: str,
    unknown: bool = False,
) -> None:
    run = await repository.find_run(session, run_id, for_update=True)
    if run is None or run.task_id != task_id:
        raise ApiError(
            ErrorCode.INTERNAL_ERROR,
            "Adaptation run state is unavailable",
            status_code=500,
        )
    if run.status in {"succeeded", "published", "cancelled", "failed", "unknown"}:
        return
    run.status = "unknown" if unknown else "failed"
    run.error_code = error_code
    run.candidate_body = None
    run.candidate_hash = None
    run.draft_body = None
    run.draft_hash = None
    run.revision += 1
    run.updated_at = datetime.now(UTC)
    await session.flush()
