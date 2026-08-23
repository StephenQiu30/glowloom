import io
import json
import zipfile
from collections.abc import AsyncIterator
from pathlib import Path
from typing import Any, cast
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.modules.identity.models import Membership
from app.modules.media import MediaProbePort, MediaProbeResult
from app.modules.media.models import MediaVersion
from app.modules.media.storage import (
    ObjectStoragePort,
    StorageObjectMetadata,
    StorageUnavailable,
)
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.scripts.extractions.schemas import ScriptExtractionResult
from app.modules.scripts.models import ExtractionBatch
from app.modules.scripts.narratives.parser import parse_narrative_units
from app.modules.storyboards.drafts import DraftProviderResult, record_draft_result
from app.modules.storyboards.exports.models import StoryboardExportManifest
from app.runtime.workers import io as io_worker
from app.runtime.workers import media as media_worker
from tests.integration.storyboards.test_storyboards_api import (
    create_ready_location_asset,
)
from tests.support.project_builders import project_payload, register_project_owner

FIXTURE = (
    Path(__file__).parents[4]
    / "backend/tests/fixtures/mvp_a/golden_candidate_harbor_countdown.json"
)


def _fixture_text() -> str:
    raw = json.loads(FIXTURE.read_text(encoding="utf-8"))
    return str(raw["full_script"])


class JourneyStorage(ObjectStoragePort):
    def __init__(self) -> None:
        self.objects: dict[str, tuple[bytes, str]] = {}
        self.put_calls: list[str] = []
        self.stat_failures = 0

    async def ensure_bucket(self) -> None:
        return None

    async def presign_upload(self, object_key: str, expires_seconds: int) -> str:
        raise AssertionError("export journey must not presign uploads")

    async def presign_download(self, object_key: str, expires_seconds: int) -> str:
        raise AssertionError("worker must not presign downloads")

    async def stat(self, object_key: str) -> StorageObjectMetadata:
        if self.stat_failures:
            self.stat_failures -= 1
            raise StorageUnavailable("temporary journey storage failure")
        content, media_type = self.objects[object_key]
        return StorageObjectMetadata(len(content), media_type, "journey-etag")

    async def put(self, object_key: str, data: bytes, content_type: str) -> None:
        existing = self.objects.get(object_key)
        if existing is not None and existing != (data, content_type):
            raise AssertionError("a deterministic object key changed bytes")
        self.objects[object_key] = (data, content_type)
        self.put_calls.append(object_key)

    async def copy(self, source_key: str, target_key: str) -> None:
        raise AssertionError("export journey must not copy objects")

    def stream(self, object_key: str) -> AsyncIterator[bytes]:
        async def chunks() -> AsyncIterator[bytes]:
            yield self.objects[object_key][0]

        return chunks()

    async def delete(self, object_key: str) -> None:
        self.objects.pop(object_key, None)


class NoProbe(MediaProbePort):
    async def probe(
        self,
        content: AsyncIterator[bytes],
        *,
        kind: str,
        mime_type: str,
    ) -> MediaProbeResult:
        raise AssertionError("storyboard packages are delivery media")


class RecordedDelivery:
    def __init__(self, body: bytes) -> None:
        self.body = body
        self.ack_count = 0
        self.nack_requeues: list[bool] = []

    async def ack(self) -> None:
        self.ack_count += 1

    async def nack(self, *, requeue: bool) -> None:
        self.nack_requeues.append(requeue)


async def _materialize_first_episode(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> tuple[dict[str, str], dict[str, Any], dict[str, UUID]]:
    headers, workspace_id = await register_project_owner(
        client,
        email="script-to-export-journey@example.com",
    )
    created_project = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(workspace_id, name="黄金剧联合验收"),
    )
    assert created_project.status_code == 201
    project = created_project.json()["data"]
    imported = await client.post(
        f"/api/v1/projects/{project['id']}/script-imports",
        headers=headers,
        json={
            "input_type": "text",
            "title": "雾港倒计时整剧原稿",
            "text": _fixture_text(),
            "language": "zh-CN",
            "rights_declaration": "原创合成黄金样本",
            "idempotency_key": "journey-document-import",
        },
    )
    assert imported.status_code == 201
    revision = imported.json()["data"]["revision"]
    planned = await client.post(
        f"/api/v1/document-revisions/{revision['id']}/episode-plans",
        headers=headers,
        json={
            "strategy": "explicit_markers",
            "target_duration_ms": 90_000,
            "requested_episode_count": None,
            "idempotency_key": "journey-episode-plan",
        },
    )
    assert planned.status_code == 201
    plan = planned.json()["data"]
    assert len(plan["proposals"]) == 5
    confirmed = await client.post(
        f"/api/v1/episode-plans/{plan['plan']['id']}/confirm",
        headers=headers,
        json={
            "expected_revision": plan["plan"]["revision"],
            "idempotency_key": "journey-plan-confirm",
        },
    )
    assert confirmed.status_code == 200
    confirmation = confirmed.json()["data"]
    materialized = await client.post(
        f"/api/v1/episode-plans/{plan['plan']['id']}/materializations",
        headers=headers,
        json={
            "mode": "append_new",
            "expected_plan_revision": confirmation["plan"]["revision"],
            "expected_project_revision": confirmation["impact"]["project_revision"],
            "expected_active_order_hash": confirmation["impact"]["active_order_hash"],
            "idempotency_key": "journey-materialize",
        },
    )
    assert materialized.status_code == 201
    batch = materialized.json()["data"]
    published = await client.post(
        f"/api/v1/import-commits/{batch['commit']['id']}/publish",
        headers=headers,
        json={
            "expected_revision": batch["commit"]["revision"],
            "idempotency_key": "journey-publish",
        },
    )
    assert published.status_code == 200
    published_batch = published.json()["data"]
    assert len(published_batch["segments"]) == 5
    assert all(item["published_version_id"] for item in published_batch["segments"])
    first = published_batch["segments"][0]
    async with session_factory() as session:
        actor_id = await session.scalar(
            select(Membership.user_id).where(
                Membership.workspace_id == UUID(workspace_id),
                Membership.role == "owner",
                Membership.status == "active",
            )
        )
    assert actor_id is not None
    return (
        headers,
        project,
        {
            "workspace_id": UUID(workspace_id),
            "actor_id": actor_id,
            "episode_id": UUID(first["episode_id"]),
            "script_version_id": UUID(first["published_version_id"]),
        },
    )


class JourneyScriptExtractor:
    def __init__(self) -> None:
        self.inputs: list[str] = []

    async def extract(
        self,
        script_body: str,
        *,
        trace_id: str | None = None,
        episode_number: int | None = None,
    ) -> ScriptExtractionResult:
        del trace_id, episode_number
        self.inputs.append(script_body)
        dialogue_units = [
            unit for unit in parse_narrative_units(script_body) if unit.kind == "dialogue"
        ]
        if not dialogue_units:
            raise AssertionError("journey fixture must contain dialogue")
        dialogue_candidates: list[dict[str, object]] = []
        speakers: list[str] = []
        for position, unit in enumerate(dialogue_units, start=1):
            separator = "：" if "：" in unit.exact_text else ":"
            speaker, dialogue_text = unit.exact_text.split(separator, maxsplit=1)
            speakers.append(speaker.strip())
            dialogue_candidates.append(
                {
                    "candidate_key": f"journey-dialogue-{position}",
                    "source_range": {
                        "start": unit.source_start,
                        "end": unit.source_end,
                    },
                    "proposal": {
                        "kind": "dialogue",
                        "scene_candidate_key": "journey-scene",
                        "speaker_candidate": speaker.strip(),
                        "dialogue_kind": "spoken",
                        "text": dialogue_text.strip(),
                    },
                }
            )
        first_dialogue = dialogue_units[0]
        first_speaker = speakers[0]
        return ScriptExtractionResult.model_validate(
            {
                "candidates": [
                    {
                        "candidate_key": "journey-scene",
                        "source_range": {"start": 0, "end": len(script_body)},
                        "proposal": {
                            "kind": "scene",
                            "heading": "雾港控制室",
                            "location": "雾港控制室",
                            "time_of_day": "夜",
                            "summary": "警报触发后，角色开始处理港口危机。",
                            "episode_number": 1,
                            "scene_number": 1,
                            "characters": list(dict.fromkeys(speakers)),
                            "production_tasks": [
                                {
                                    "task_type": "shot_breakdown",
                                    "title": "拆解雾港危机镜头",
                                    "objective": "生成可执行的逐镜分镜候选。",
                                    "priority": "high",
                                }
                            ],
                        },
                    },
                    *dialogue_candidates,
                    {
                        "candidate_key": "journey-character",
                        "source_range": {
                            "start": first_dialogue.source_start,
                            "end": first_dialogue.source_start + len(first_speaker),
                        },
                        "proposal": {
                            "kind": "asset",
                            "asset_kind": "character",
                            "name": first_speaker,
                            "description": "雾港危机现场的核心角色。",
                            "first_seen_episode": 1,
                            "episode_numbers": [1],
                        },
                    },
                    {
                        "candidate_key": "journey-shot",
                        "source_range": {"start": 0, "end": len(script_body)},
                        "proposal": {
                            "kind": "shot",
                            "scene_candidate_key": "journey-scene",
                            "title": "雾港警报",
                            "purpose": "建立本集危机并推进角色行动。",
                            "shot_type": "wide",
                            "asset_names": [first_speaker],
                        },
                    },
                ]
            }
        )


async def _extract_and_confirm_narrative_sources(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    headers: dict[str, str],
    refs: dict[str, UUID],
) -> tuple[UUID, list[UUID]]:
    async with session_factory() as session:
        batch = await session.scalar(
            select(ExtractionBatch).where(
                ExtractionBatch.script_version_id == refs["script_version_id"]
            )
        )
        assert batch is not None and batch.task_id is not None
        event = await session.scalar(
            select(OutboxEvent).where(OutboxEvent.aggregate_id == batch.task_id)
        )
        assert event is not None

    extractor = JourneyScriptExtractor()
    message = RecordedDelivery(envelope_from_event(event).model_dump_json().encode())
    assert (
        await io_worker.process_incoming_message(
            message,
            session_factory,
            extractor=extractor,
        )
        == "completed"
    )
    assert len(extractor.inputs) == 1

    listed = await client.get(
        f"/api/v1/extraction-batches/{batch.id}/candidates",
        headers=headers,
        params={"limit": 100},
    )
    assert listed.status_code == 200
    candidates = cast(list[dict[str, Any]], listed.json()["data"]["items"])
    assert {item["kind"] for item in candidates} == {"scene", "dialogue", "asset", "shot"}
    scene_candidate = next(item for item in candidates if item["kind"] == "scene")
    assert scene_candidate["proposal"]["production_tasks"][0]["task_type"] == "shot_breakdown"
    character_candidate = next(item for item in candidates if item["kind"] == "asset")
    assert character_candidate["proposal"]["episode_numbers"] == [1]
    for candidate in candidates:
        if candidate["kind"] not in {"scene", "dialogue"}:
            continue
        decided = await client.post(
            f"/api/v1/extraction-candidates/{candidate['id']}/decisions",
            headers=headers,
            json={
                "decision_key": f"journey-accept:{candidate['id']}",
                "expected_revision": candidate["revision"],
                "decision": {"action": "accept_new"},
            },
        )
        assert decided.status_code == 201

    confirmed = await client.post(
        f"/api/v1/extraction-batches/{batch.id}/confirm-structure",
        headers=headers,
    )
    assert confirmed.status_code == 201
    structure = confirmed.json()["data"]
    confirmed_version_id = UUID(structure["confirmed_version"]["id"])
    switched = await client.post(
        f"/api/v1/episodes/{refs['episode_id']}/current-script-version",
        headers=headers,
        json={
            "version_id": str(confirmed_version_id),
            "expected_current_version_id": str(refs["script_version_id"]),
        },
    )
    assert switched.status_code == 200
    refs["script_version_id"] = confirmed_version_id
    scenes = cast(list[dict[str, Any]], structure["scenes"])
    assert len(scenes) == 1
    return UUID(scenes[0]["id"]), [UUID(dialogue["id"]) for dialogue in scenes[0]["dialogues"]]


def _draft_result(
    structure: dict[str, Any],
    *,
    scene_id: UUID,
    asset_version_id: str,
    target_duration_ms: int,
) -> DraftProviderResult:
    duration_count = (target_duration_ms + 14_999) // 15_000
    shot_count = max(12, duration_count) if target_duration_ms >= 60_000 else duration_count
    duration_ms, remainder = divmod(target_duration_ms, shot_count)
    unit_ids = [item["id"] for item in structure["units"]]
    return DraftProviderResult.model_validate(
        {
            "shots": [
                {
                    "proposal_key": f"golden-shot-{position}",
                    "position": position,
                    "title": f"雾港警报 {position}",
                    "narrative_unit_version_ids": (unit_ids if position == 1 else unit_ids[:1]),
                    "spec": {
                        "script_reference": {
                            "confirmed_script_version_id": structure["script_version_id"],
                            "scene_id": str(scene_id),
                            "dialogue_ids": [],
                        },
                        "narrative": {"purpose": f"推进雾港危机节拍 {position}"},
                        "visual": {
                            "shot_size": "wide",
                            "camera_angle": "eye_level",
                            "camera_movement": "dolly",
                            "composition": "控制台位于前景，警报灯引向港口",
                            "environment": "暴雨中的雾港控制室",
                            "subject_placements": [],
                            "mood_lighting": "冷蓝环境光与红色警报灯",
                        },
                        "action_beats": [
                            {
                                "beat_key": "alarm",
                                "order": 1,
                                "description": "警报灯亮起，值班员冲向控制台",
                            }
                        ],
                        "dialogue_or_narration": [],
                        "duration_ms": duration_ms + (1 if position <= remainder else 0),
                        "audio_intent": {
                            "ambient": "暴雨和远处汽笛",
                            "sound_effects": ["警报声"],
                        },
                        "generation_intent": {"mode": "reference_to_video"},
                    },
                    "asset_references": [
                        {
                            "slot_key": "location-main",
                            "role": "location",
                            "asset_version_id": asset_version_id,
                            "subject_key": None,
                        }
                    ],
                    "risk_codes": [],
                }
                for position in range(1, shot_count + 1)
            ]
        }
    )


async def _apply_reviewed_draft(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    headers: dict[str, str],
    refs: dict[str, UUID],
    asset_version: dict[str, Any],
    scene_id: UUID,
) -> int:
    structure_response = await client.get(
        f"/api/v1/script-versions/{refs['script_version_id']}/narrative-structure",
        headers=headers,
    )
    assert structure_response.status_code == 200
    structure = structure_response.json()["data"]
    created = await client.post(
        f"/api/v1/episodes/{refs['episode_id']}/storyboard-draft-batches",
        headers=headers,
        json={
            "input_script_version_id": str(refs["script_version_id"]),
            "asset_state_ids": [asset_version["asset_state_id"]],
            "idempotency_key": "journey-storyboard-draft",
        },
    )
    assert created.status_code == 202, created.text
    batch = created.json()["data"]
    async with session_factory() as session, session.begin():
        await record_draft_result(
            session,
            batch_id=UUID(batch["id"]),
            result=_draft_result(
                structure,
                scene_id=scene_id,
                asset_version_id=asset_version["id"],
                target_duration_ms=batch["input"]["target_duration_ms"],
            ),
        )
    fetched = await client.get(
        f"/api/v1/storyboard-draft-batches/{batch['id']}",
        headers=headers,
    )
    assert fetched.status_code == 200
    batch = fetched.json()["data"]
    assert batch["status"] == "needs_review"
    for draft in batch["drafts"]:
        decided = await client.post(
            f"/api/v1/storyboard-drafts/{draft['id']}/decisions",
            headers=headers,
            json={
                "action": "accepted",
                "expected_batch_revision": batch["revision"],
                "idempotency_key": f"journey-draft-accept:{draft['id']}",
            },
        )
        assert decided.status_code == 201
        batch = decided.json()["data"]["batch"]
    approved = await client.post(
        f"/api/v1/storyboard-draft-batches/{batch['id']}/approve",
        headers=headers,
        json={
            "expected_revision": batch["revision"],
            "idempotency_key": "journey-draft-approve",
        },
    )
    assert approved.status_code == 200
    batch = approved.json()["data"]
    preflight = await client.post(
        f"/api/v1/storyboard-draft-batches/{batch['id']}/apply-preflight",
        headers=headers,
        json={"expected_revision": batch["revision"]},
    )
    assert preflight.status_code == 200
    impact = preflight.json()["data"]
    applied = await client.post(
        f"/api/v1/storyboard-draft-batches/{batch['id']}/apply",
        headers=headers,
        json={
            "expected_revision": batch["revision"],
            "expected_order_hash": impact["order_hash"],
            "impact_hash": impact["impact_hash"],
            "idempotency_key": "journey-draft-apply",
        },
    )
    assert applied.status_code == 201
    return len(applied.json()["data"]["created_shot_ids"])


async def _request_export(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    headers: dict[str, str],
    episode_id: UUID,
    idempotency_key: str,
) -> tuple[dict[str, Any], bytes]:
    preflight = await client.post(
        f"/api/v1/episodes/{episode_id}/storyboard-exports/preflight",
        headers=headers,
    )
    assert preflight.status_code == 200
    prepared = preflight.json()["data"]
    assert prepared["status"] == "ready"
    created = await client.post(
        f"/api/v1/episodes/{episode_id}/storyboard-exports",
        headers=headers,
        json={
            "expected_input_hash": prepared["input_hash"],
            "idempotency_key": idempotency_key,
        },
    )
    assert created.status_code == 202
    export = cast(dict[str, Any], created.json()["data"])
    async with session_factory() as session:
        event = await session.scalar(
            select(OutboxEvent).where(OutboxEvent.aggregate_id == UUID(export["task_id"]))
        )
        assert event is not None
        body = envelope_from_event(event).model_dump_json().encode()
    return export, body


async def _change_asset_current(
    client: httpx.AsyncClient,
    *,
    headers: dict[str, str],
    asset_version: dict[str, Any],
) -> None:
    states = await client.get(
        f"/api/v1/assets/{asset_version['asset_id']}/states",
        headers=headers,
    )
    assert states.status_code == 200
    state = states.json()["data"]["items"][0]
    changed = await client.post(
        f"/api/v1/asset-states/{state['id']}/versions",
        headers=headers,
        json={
            "spec": {
                "kind": "location",
                "spatial_description": "警报后的雾港控制室",
                "time_weather": "暴雨深夜",
                "visual_elements": ["红色警报灯", "积水玻璃"],
                "lighting": "红蓝交替警报光",
            },
            "prompt_description": "切换 current 以验证固定导出快照",
            "media_references": asset_version["media_references"],
            "source_type": "manual",
            "source_id": None,
            "expected_revision": state["revision"],
            "expected_current_version_id": asset_version["id"],
            "set_as_current": True,
        },
    )
    assert changed.status_code == 201, changed.text


@pytest.mark.asyncio
async def test_whole_script_reaches_stable_export_after_restart(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, project, refs = await _materialize_first_episode(
        client,
        session_factory,
    )
    scene_id, _dialogue_ids = await _extract_and_confirm_narrative_sources(
        client,
        session_factory,
        headers,
        refs,
    )
    asset_version, _consent = await create_ready_location_asset(
        client,
        session_factory,
        headers=headers,
        project_id=UUID(project["id"]),
        refs=refs,
    )
    shot_count = await _apply_reviewed_draft(
        client,
        session_factory,
        headers=headers,
        refs=refs,
        asset_version=asset_version,
        scene_id=scene_id,
    )
    coverage = await client.get(
        f"/api/v1/episodes/{refs['episode_id']}/coverage",
        headers=headers,
    )
    readiness = await client.get(
        f"/api/v1/episodes/{refs['episode_id']}/shot-readiness",
        headers=headers,
    )
    assert coverage.status_code == 200
    assert coverage.json()["data"]["status"] == "ready"
    assert readiness.status_code == 200
    assert readiness.json()["data"]["summary"]["ready"] == shot_count

    storage = JourneyStorage()
    first_export, first_body = await _request_export(
        client,
        session_factory,
        headers=headers,
        episode_id=refs["episode_id"],
        idempotency_key="journey-export-first",
    )
    first_message = RecordedDelivery(first_body)
    assert (
        await media_worker.process_incoming_message(
            first_message,
            session_factory,
            storage=storage,
            probe=NoProbe(),
        )
        == "completed"
    )
    first_key = f"exports/{refs['workspace_id']}/{first_export['id']}.zip"
    first_bytes = storage.objects[first_key][0]
    with zipfile.ZipFile(io.BytesIO(first_bytes)) as package:
        assert package.namelist() == [
            "manifest.json",
            "storyboard.csv",
            "storyboard.html",
            "storyboard.json",
        ]

    second_export, second_body = await _request_export(
        client,
        session_factory,
        headers=headers,
        episode_id=refs["episode_id"],
        idempotency_key="journey-export-second",
    )
    assert second_export["input_hash"] == first_export["input_hash"]
    await _change_asset_current(
        client,
        headers=headers,
        asset_version=asset_version,
    )
    changed_preflight = await client.post(
        f"/api/v1/episodes/{refs['episode_id']}/storyboard-exports/preflight",
        headers=headers,
    )
    assert changed_preflight.status_code == 200
    assert changed_preflight.json()["data"]["input_hash"] != second_export["input_hash"]

    storage.stat_failures = 1
    interrupted = RecordedDelivery(second_body)
    assert (
        await media_worker.process_incoming_message(
            interrupted,
            session_factory,
            storage=storage,
            probe=NoProbe(),
        )
        == "requeued"
    )
    assert interrupted.nack_requeues == [True]
    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(StoryboardExportManifest)) == 1
        assert (
            await session.scalar(
                select(func.count())
                .select_from(InboxDelivery)
                .where(InboxDelivery.event_type == "storyboard_export.requested")
            )
            == 1
        )

    restarted = RecordedDelivery(second_body)
    assert (
        await media_worker.process_incoming_message(
            restarted,
            session_factory,
            storage=storage,
            probe=NoProbe(),
        )
        == "completed"
    )
    second_key = f"exports/{refs['workspace_id']}/{second_export['id']}.zip"
    assert storage.objects[second_key][0] == first_bytes
    assert storage.put_calls == [first_key, second_key, second_key]

    duplicate = RecordedDelivery(second_body)
    assert (
        await media_worker.process_incoming_message(
            duplicate,
            session_factory,
            storage=storage,
            probe=NoProbe(),
        )
        == "duplicate"
    )
    history = await client.get(
        f"/api/v1/episodes/{refs['episode_id']}/storyboard-exports",
        headers=headers,
    )
    assert history.status_code == 200
    assert history.json()["data"]["total"] == 2
    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(StoryboardExportManifest)) == 2
        assert (
            await session.scalar(
                select(func.count())
                .select_from(MediaVersion)
                .where(MediaVersion.mime_type == "application/zip")
            )
            == 2
        )
