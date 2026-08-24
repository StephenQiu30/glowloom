from collections.abc import Sequence
from uuid import UUID

from sqlalchemy import func, or_, select
from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.scripts.models import (
    CandidateDecision,
    Dialogue,
    ExtractionBatch,
    ExtractionCandidate,
    Scene,
    ScriptSource,
    ScriptVersion,
)


async def find_source_by_idempotency(
    session: AsyncSession,
    episode_id: UUID,
    idempotency_key: str,
) -> ScriptSource | None:
    return await session.scalar(
        select(ScriptSource).where(
            ScriptSource.episode_id == episode_id,
            ScriptSource.idempotency_key == idempotency_key,
        )
    )


async def find_initial_version(
    session: AsyncSession,
    source_id: UUID,
) -> ScriptVersion | None:
    return await session.scalar(
        select(ScriptVersion).where(
            ScriptVersion.source_id == source_id,
            ScriptVersion.version_no == 1,
        )
    )


async def find_source(
    session: AsyncSession,
    source_id: UUID,
    *,
    for_update: bool = False,
) -> ScriptSource | None:
    query = select(ScriptSource).where(ScriptSource.id == source_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def list_sources(
    session: AsyncSession,
    episode_id: UUID,
    *,
    limit: int,
    offset: int,
) -> tuple[list[ScriptSource], int]:
    total = await session.scalar(
        select(func.count()).select_from(ScriptSource).where(ScriptSource.episode_id == episode_id)
    )
    rows = await session.scalars(
        select(ScriptSource)
        .where(ScriptSource.episode_id == episode_id)
        .order_by(ScriptSource.created_at.desc(), ScriptSource.id.desc())
        .limit(limit)
        .offset(offset)
    )
    return list(rows), total or 0


async def count_versions_by_episode(
    session: AsyncSession,
    workspace_id: UUID,
    episode_ids: list[UUID],
) -> list[tuple[UUID, int]]:
    if not episode_ids:
        return []
    rows = await session.execute(
        select(ScriptSource.episode_id, func.count(ScriptVersion.id))
        .join(ScriptVersion, ScriptVersion.source_id == ScriptSource.id)
        .where(
            ScriptSource.workspace_id == workspace_id,
            ScriptSource.episode_id.in_(episode_ids),
        )
        .group_by(ScriptSource.episode_id)
    )
    return [(episode_id, count) for episode_id, count in rows]


async def latest_version_number(
    session: AsyncSession,
    source_id: UUID,
) -> int:
    latest = await session.scalar(
        select(func.max(ScriptVersion.version_no)).where(ScriptVersion.source_id == source_id)
    )
    return latest or 0


async def find_version(
    session: AsyncSession,
    version_id: UUID,
    *,
    for_update: bool = False,
) -> ScriptVersion | None:
    query = select(ScriptVersion).where(ScriptVersion.id == version_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def list_versions(
    session: AsyncSession,
    source_id: UUID,
    *,
    limit: int,
    offset: int,
) -> tuple[list[ScriptVersion], int]:
    total = await session.scalar(
        select(func.count()).select_from(ScriptVersion).where(ScriptVersion.source_id == source_id)
    )
    rows = await session.scalars(
        select(ScriptVersion)
        .where(ScriptVersion.source_id == source_id)
        .order_by(ScriptVersion.version_no, ScriptVersion.id)
        .limit(limit)
        .offset(offset)
    )
    return list(rows), total or 0


async def find_idempotent_extraction_batch(
    session: AsyncSession,
    script_version_id: UUID,
    idempotency_key: str,
) -> ExtractionBatch | None:
    return await session.scalar(
        select(ExtractionBatch).where(
            ExtractionBatch.script_version_id == script_version_id,
            ExtractionBatch.idempotency_key == idempotency_key,
        )
    )


async def find_extraction_batch(
    session: AsyncSession,
    batch_id: UUID,
    *,
    for_update: bool = False,
) -> ExtractionBatch | None:
    query = select(ExtractionBatch).where(ExtractionBatch.id == batch_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def find_extraction_batch_by_confirmed_version(
    session: AsyncSession,
    confirmed_script_version_id: UUID,
) -> ExtractionBatch | None:
    return await session.scalar(
        select(ExtractionBatch)
        .where(
            ExtractionBatch.confirmed_script_version_id == confirmed_script_version_id,
            ExtractionBatch.status == "succeeded",
        )
        .order_by(ExtractionBatch.updated_at.desc(), ExtractionBatch.id.desc())
        .limit(1)
    )


async def list_extraction_batches_referencing_version(
    session: AsyncSession,
    version_id: UUID,
) -> list[ExtractionBatch]:
    batches = await session.scalars(
        select(ExtractionBatch)
        .where(
            or_(
                ExtractionBatch.script_version_id == version_id,
                ExtractionBatch.confirmed_script_version_id == version_id,
            )
        )
        .order_by(ExtractionBatch.created_at, ExtractionBatch.id)
    )
    return list(batches)


async def list_extraction_batches_for_current_versions(
    session: AsyncSession,
    version_ids: list[UUID],
) -> list[ExtractionBatch]:
    if not version_ids:
        return []
    rows = await session.scalars(
        select(ExtractionBatch)
        .where(
            or_(
                ExtractionBatch.script_version_id.in_(version_ids),
                ExtractionBatch.confirmed_script_version_id.in_(version_ids),
            )
        )
        .order_by(ExtractionBatch.created_at.desc(), ExtractionBatch.id.desc())
    )
    return list(rows)


async def count_pending_required_candidates(
    session: AsyncSession,
    batch_ids: list[UUID],
) -> dict[UUID, int]:
    if not batch_ids:
        return {}
    rows = await session.execute(
        select(ExtractionCandidate.batch_id, func.count())
        .where(
            ExtractionCandidate.batch_id.in_(batch_ids),
            ExtractionCandidate.required.is_(True),
            ExtractionCandidate.status == "pending",
        )
        .group_by(ExtractionCandidate.batch_id)
    )
    return {batch_id: count for batch_id, count in rows}


async def count_asset_candidate_decisions(
    session: AsyncSession,
    workspace_id: UUID,
    asset_ids: list[UUID],
) -> dict[UUID, int]:
    if not asset_ids:
        return {}
    rows = await session.execute(
        select(CandidateDecision.downstream_id, func.count())
        .where(
            CandidateDecision.workspace_id == workspace_id,
            CandidateDecision.downstream_type == "ASSET",
            CandidateDecision.downstream_id.in_(asset_ids),
        )
        .group_by(CandidateDecision.downstream_id)
    )
    return {asset_id: count for asset_id, count in rows if asset_id is not None}


async def list_extraction_candidates(
    session: AsyncSession,
    batch_id: UUID,
    *,
    kind: str | None,
    status: str | None,
    limit: int,
    offset: int,
) -> tuple[list[ExtractionCandidate], int]:
    filters = [ExtractionCandidate.batch_id == batch_id]
    if kind is not None:
        filters.append(ExtractionCandidate.kind == kind)
    if status is not None:
        filters.append(ExtractionCandidate.status == status)
    total = await session.scalar(
        select(func.count()).select_from(ExtractionCandidate).where(*filters)
    )
    candidates = await session.scalars(
        select(ExtractionCandidate)
        .where(*filters)
        .order_by(
            ExtractionCandidate.source_start,
            ExtractionCandidate.source_end,
            ExtractionCandidate.id,
        )
        .limit(limit)
        .offset(offset)
    )
    return list(candidates), total or 0


async def find_extraction_candidate(
    session: AsyncSession,
    candidate_id: UUID,
    *,
    for_update: bool = False,
) -> ExtractionCandidate | None:
    query = select(ExtractionCandidate).where(ExtractionCandidate.id == candidate_id)
    if for_update:
        query = query.with_for_update()
    return await session.scalar(query)


async def list_structure_candidates(
    session: AsyncSession,
    batch_id: UUID,
    *,
    for_update: bool = False,
) -> list[ExtractionCandidate]:
    query = (
        select(ExtractionCandidate)
        .where(
            ExtractionCandidate.batch_id == batch_id,
            ExtractionCandidate.kind.in_(
                ("scene", "dialogue", "asset_occurrence", "continuity")
            ),
        )
        .order_by(
            ExtractionCandidate.source_start,
            ExtractionCandidate.source_end,
            ExtractionCandidate.id,
        )
    )
    if for_update:
        query = query.with_for_update()
    return list(await session.scalars(query))


async def find_candidate_decision_by_key(
    session: AsyncSession,
    candidate_id: UUID,
    decision_key: str,
) -> CandidateDecision | None:
    return await session.scalar(
        select(CandidateDecision).where(
            CandidateDecision.candidate_id == candidate_id,
            CandidateDecision.decision_key == decision_key,
        )
    )


async def list_candidate_decisions(
    session: AsyncSession,
    candidate_id: UUID,
    *,
    limit: int,
    offset: int,
) -> tuple[list[CandidateDecision], int]:
    total = await session.scalar(
        select(func.count())
        .select_from(CandidateDecision)
        .where(CandidateDecision.candidate_id == candidate_id)
    )
    decisions = await session.scalars(
        select(CandidateDecision)
        .where(CandidateDecision.candidate_id == candidate_id)
        .order_by(CandidateDecision.sequence, CandidateDecision.id)
        .limit(limit)
        .offset(offset)
    )
    return list(decisions), total or 0


async def list_candidate_decisions_for_candidates(
    session: AsyncSession,
    candidate_ids: Sequence[UUID],
) -> list[CandidateDecision]:
    if not candidate_ids:
        return []
    decisions = await session.scalars(
        select(CandidateDecision)
        .where(CandidateDecision.candidate_id.in_(candidate_ids))
        .order_by(
            CandidateDecision.candidate_id,
            CandidateDecision.sequence,
            CandidateDecision.id,
        )
    )
    return list(decisions)


async def list_scenes(
    session: AsyncSession,
    script_version_id: UUID,
) -> list[Scene]:
    scenes = await session.scalars(
        select(Scene)
        .where(Scene.script_version_id == script_version_id)
        .order_by(Scene.position, Scene.id)
    )
    return list(scenes)


async def find_scene(
    session: AsyncSession,
    scene_id: UUID,
) -> Scene | None:
    return await session.scalar(select(Scene).where(Scene.id == scene_id))


async def find_structure_rows(
    session: AsyncSession,
    script_version_ids: Sequence[UUID],
    scene_ids: Sequence[UUID],
) -> list[tuple[ScriptVersion, ScriptSource, Scene]]:
    if not script_version_ids or not scene_ids:
        return []
    rows = await session.execute(
        select(ScriptVersion, ScriptSource, Scene)
        .join(ScriptSource, ScriptSource.id == ScriptVersion.source_id)
        .join(Scene, Scene.script_version_id == ScriptVersion.id)
        .where(
            ScriptVersion.id.in_(script_version_ids),
            Scene.id.in_(scene_ids),
        )
    )
    return [(row[0], row[1], row[2]) for row in rows]


async def list_dialogues(
    session: AsyncSession,
    scene_ids: Sequence[UUID],
) -> list[Dialogue]:
    if not scene_ids:
        return []
    dialogues = await session.scalars(
        select(Dialogue)
        .where(Dialogue.scene_id.in_(scene_ids))
        .order_by(Dialogue.scene_id, Dialogue.position, Dialogue.id)
    )
    return list(dialogues)
