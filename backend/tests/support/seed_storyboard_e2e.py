from __future__ import annotations

import argparse
import asyncio
import os
from hashlib import sha256
from urllib.parse import urlsplit
from uuid import UUID

from sqlalchemy import select
from sqlalchemy.ext.asyncio import async_sessionmaker
from uuid6 import uuid7

from app.core.database import create_engine
from app.model_registry import register_implemented_models
from app.modules.assets.models import Asset, AssetState
from app.modules.identity.models import Membership
from app.modules.production.models import Task
from app.modules.projects.models import Episode
from app.modules.scripts.models import (
    CandidateDecision,
    Dialogue,
    ExtractionBatch,
    ExtractionCandidate,
    Scene,
    ScriptSource,
    ScriptVersion,
)


def _validated_database_url() -> str:
    database_url = os.environ.get("DATABASE_URL", "")
    database_name = urlsplit(
        database_url.replace("postgresql+asyncpg", "postgresql", 1)
    ).path.lstrip("/")
    if not database_url or not database_name.endswith("_test"):
        raise RuntimeError("storyboard E2E seed requires a *_test DATABASE_URL")
    return database_url


async def seed_confirmed_structure(episode_id: UUID) -> None:
    register_implemented_models()
    engine = create_engine(_validated_database_url())
    session_factory = async_sessionmaker(engine, expire_on_commit=False)
    try:
        async with session_factory() as session, session.begin():
            episode = await session.scalar(
                select(Episode).where(Episode.id == episode_id).with_for_update()
            )
            if episode is None:
                raise RuntimeError(f"episode {episode_id} does not exist")

            if episode.current_script_version_id is not None:
                current_version = await session.get(
                    ScriptVersion, episode.current_script_version_id
                )
                if current_version and current_version.structure_summary.get(
                    "confirmation_batch_id"
                ):
                    return
                raise RuntimeError("episode already has an unconfirmed current script")

            actor_id = await session.scalar(
                select(Membership.user_id).where(
                    Membership.workspace_id == episode.workspace_id,
                    Membership.role == "owner",
                    Membership.status == "active",
                )
            )
            if actor_id is None:
                raise RuntimeError("episode workspace has no active owner")

            source_id = uuid7()
            source_version_id = uuid7()
            confirmed_version_id = uuid7()
            scene_id = uuid7()
            dialogue_id = uuid7()
            batch_id = uuid7()
            task_id = uuid7()
            scene_candidate_id = uuid7()
            shot_candidate_id = uuid7()
            body = "雨夜车站\n林澈：有人吗？"
            content_hash = sha256(body.encode()).hexdigest()
            session.add(
                ScriptSource(
                    id=source_id,
                    workspace_id=episode.workspace_id,
                    episode_id=episode.id,
                    input_type="text",
                    title="S3 本地确认结构夹具",
                    rights_declaration="虚构测试文本，仅用于无密钥浏览器验收",
                    status="active",
                    revision=1,
                    idempotency_key=f"storyboard-e2e:{episode.id}",
                )
            )
            await session.flush()
            session.add(
                ScriptVersion(
                    id=source_version_id,
                    workspace_id=episode.workspace_id,
                    source_id=source_id,
                    version_no=1,
                    status="published",
                    body=body,
                    content_hash=content_hash,
                    structure_summary={},
                    created_by=actor_id,
                )
            )
            await session.flush()
            session.add(
                ScriptVersion(
                    id=confirmed_version_id,
                    workspace_id=episode.workspace_id,
                    source_id=source_id,
                    version_no=2,
                    status="published",
                    body=body,
                    content_hash=content_hash,
                    structure_summary={
                        "confirmation_batch_id": str(batch_id),
                        "source_script_version_id": str(source_version_id),
                        "scene_count": 1,
                        "dialogue_count": 1,
                    },
                    created_by=actor_id,
                )
            )
            await session.flush()
            session.add(
                Scene(
                    id=scene_id,
                    workspace_id=episode.workspace_id,
                    script_version_id=confirmed_version_id,
                    position=1,
                    heading="雨夜车站",
                    location="旧车站月台",
                    time_of_day="夜",
                    summary="林澈进入空无一人的月台",
                    source_start=0,
                    source_end=4,
                )
            )
            await session.flush()
            session.add(
                Dialogue(
                    id=dialogue_id,
                    workspace_id=episode.workspace_id,
                    scene_id=scene_id,
                    position=1,
                    speaker_candidate="林澈",
                    dialogue_kind="spoken",
                    text="有人吗？",
                    performance_note="压低声音，保持警惕",
                    source_start=5,
                    source_end=len(body),
                )
            )
            session.add(
                Task(
                    id=task_id,
                    workspace_id=episode.workspace_id,
                    task_type="script_extraction",
                    request_type="extraction_batch",
                    request_id=batch_id,
                    episode_id=episode.id,
                    input_version_id=source_version_id,
                    input_hash=content_hash,
                    status="succeeded",
                    progress_stage="completed",
                    next_action="review_candidates",
                    cancel_status="none",
                    idempotency_key=f"storyboard-e2e-task:{episode.id}",
                    requested_by=actor_id,
                    revision=2,
                )
            )
            await session.flush()
            batch = ExtractionBatch(
                id=batch_id,
                workspace_id=episode.workspace_id,
                script_version_id=source_version_id,
                task_id=task_id,
                scope="full",
                extractor_version="test-confirmed-structure",
                input_hash=content_hash,
                status="succeeded",
                confirmed_script_version_id=confirmed_version_id,
                result_hash=sha256(
                    f"confirmed:{confirmed_version_id}".encode()
                ).hexdigest(),
                candidate_count=2,
                idempotency_key=f"storyboard-e2e-confirmation:{episode.id}",
                created_by=actor_id,
            )
            session.add(batch)
            await session.flush()
            session.add_all(
                [
                    ExtractionCandidate(
                        id=scene_candidate_id,
                        workspace_id=episode.workspace_id,
                        batch_id=batch_id,
                        candidate_key="test-scene-001",
                        kind="scene",
                        source_start=0,
                        source_end=4,
                        proposal={
                            "kind": "scene",
                            "heading": "雨夜车站",
                            "location": "旧车站月台",
                            "time_of_day": "夜",
                            "summary": "林澈进入空无一人的月台",
                        },
                        confidence_note="测试确认结构中的本地候选，不代表模型输出",
                        required=True,
                        status="accepted",
                        revision=2,
                    ),
                    ExtractionCandidate(
                        id=shot_candidate_id,
                        workspace_id=episode.workspace_id,
                        batch_id=batch_id,
                        candidate_key="test-shot-001",
                        kind="shot",
                        source_start=0,
                        source_end=4,
                        proposal={
                            "kind": "shot",
                            "scene_candidate_key": "test-scene-001",
                            "title": "本地候选：进入车站",
                            "purpose": "验证已确认候选到稳定镜头身份的本地业务链路",
                        },
                        confidence_note="测试确认结构中的本地候选，不代表模型输出",
                        required=False,
                        status="accepted",
                        revision=2,
                    ),
                ]
            )
            await session.flush()
            session.add_all(
                [
                    CandidateDecision(
                        id=uuid7(),
                        workspace_id=episode.workspace_id,
                        candidate_id=scene_candidate_id,
                        sequence=1,
                        decision_key=f"storyboard-e2e-scene:{episode.id}",
                        action="accept_new",
                        payload={},
                        actor_id=actor_id,
                    ),
                    CandidateDecision(
                        id=uuid7(),
                        workspace_id=episode.workspace_id,
                        candidate_id=shot_candidate_id,
                        sequence=1,
                        decision_key=f"storyboard-e2e-shot:{episode.id}",
                        action="accept_new",
                        payload={},
                        actor_id=actor_id,
                    ),
                ]
            )
            episode.current_script_version_id = confirmed_version_id
            episode.revision += 1
    finally:
        await engine.dispose()


async def seed_asset_candidate_reference(episode_id: UUID, asset_id: UUID) -> None:
    register_implemented_models()
    engine = create_engine(_validated_database_url())
    session_factory = async_sessionmaker(engine, expire_on_commit=False)
    try:
        async with session_factory() as session, session.begin():
            episode = await session.get(Episode, episode_id)
            asset = await session.scalar(
                select(Asset).where(Asset.id == asset_id).with_for_update()
            )
            if episode is None or asset is None:
                raise RuntimeError("episode or asset does not exist")
            base_state = await session.scalar(
                select(AssetState).where(
                    AssetState.asset_id == asset.id,
                    AssetState.state_key == "base",
                )
            )
            if (
                asset.workspace_id != episode.workspace_id
                or asset.project_id != episode.project_id
                or asset.kind != "character"
                or base_state is None
                or base_state.current_version_id is not None
            ):
                raise RuntimeError("asset is not an empty character in the episode project")
            existing = await session.scalar(
                select(CandidateDecision.id).where(
                    CandidateDecision.workspace_id == episode.workspace_id,
                    CandidateDecision.downstream_type == "ASSET",
                    CandidateDecision.downstream_id == asset.id,
                )
            )
            if existing is not None:
                return
            if episode.current_script_version_id is None:
                raise RuntimeError("episode has no confirmed script structure")
            current_version = await session.get(
                ScriptVersion,
                episode.current_script_version_id,
            )
            if current_version is None:
                raise RuntimeError("confirmed script version does not exist")
            raw_batch_id = current_version.structure_summary.get(
                "confirmation_batch_id"
            )
            if not raw_batch_id:
                raise RuntimeError("script structure is not confirmed")
            batch = await session.scalar(
                select(ExtractionBatch)
                .where(ExtractionBatch.id == UUID(str(raw_batch_id)))
                .with_for_update()
            )
            if batch is None:
                raise RuntimeError("confirmed extraction batch does not exist")
            actor_id = await session.scalar(
                select(Membership.user_id).where(
                    Membership.workspace_id == episode.workspace_id,
                    Membership.role == "owner",
                    Membership.status == "active",
                )
            )
            if actor_id is None:
                raise RuntimeError("episode workspace has no active owner")

            candidate_id = uuid7()
            session.add(
                ExtractionCandidate(
                    id=candidate_id,
                    workspace_id=episode.workspace_id,
                    batch_id=batch.id,
                    candidate_key=f"test-asset-reference:{asset.id}",
                    kind="asset",
                    source_start=0,
                    source_end=1,
                    proposal={
                        "kind": "asset",
                        "asset_kind": asset.kind,
                        "name": asset.name,
                        "description": "本地浏览器验收引用，不代表模型输出",
                    },
                    confidence_note="本地测试事实，不代表模型输出",
                    required=False,
                    status="linked",
                    revision=2,
                )
            )
            await session.flush()
            session.add(
                CandidateDecision(
                    id=uuid7(),
                    workspace_id=episode.workspace_id,
                    candidate_id=candidate_id,
                    sequence=1,
                    decision_key=f"storyboard-e2e-asset-reference:{asset.id}",
                    action="link_existing",
                    payload={"downstream_id": str(asset.id)},
                    downstream_type="ASSET",
                    downstream_id=asset.id,
                    actor_id=actor_id,
                )
            )
            batch.candidate_count += 1
    finally:
        await engine.dispose()


def main() -> None:
    parser = argparse.ArgumentParser()
    parser.add_argument("--episode-id", required=True, type=UUID)
    parser.add_argument("--asset-id", type=UUID)
    args = parser.parse_args()
    if args.asset_id is None:
        asyncio.run(seed_confirmed_structure(args.episode_id))
    else:
        asyncio.run(seed_asset_candidate_reference(args.episode_id, args.asset_id))


if __name__ == "__main__":
    main()
