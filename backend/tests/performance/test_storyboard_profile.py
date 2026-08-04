import asyncio
import json
import math
import os
import platform
import socket
import sys
import tempfile
from datetime import UTC, datetime, timedelta
from pathlib import Path
from typing import Any, Literal, cast
from uuid import UUID

import httpx
import pytest
from sqlalchemy import select
from sqlalchemy.ext.asyncio import AsyncEngine, AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.projects.models import Episode
from app.modules.scripts.models import Dialogue, Scene, ScriptSource, ScriptVersion
from app.modules.storyboards.hashing import storyboard_content_hashes
from app.modules.storyboards.models import AssetReference, Shot, ShotSpecVersion
from app.modules.storyboards.schemas import AssetReferenceRequest, ShotSpec
from tests.support.identity_builders import register_identity_response
from tests.support.media_builders import seed_ready_media_version
from tests.support.project_builders import project_payload

BACKEND_ROOT = Path(__file__).resolve().parents[2]
PERFORMANCE_ENABLED = os.environ.get("RUN_STORYBOARD_PERFORMANCE") == "1"


def _data(response: httpx.Response, *, expected_status: int) -> dict[str, Any]:
    assert response.status_code == expected_status, response.text
    return cast(dict[str, Any], response.json()["data"])


async def _seed_confirmed_episode(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    shot_count: int,
) -> tuple[dict[str, str], dict[str, UUID]]:
    identity = _data(
        await register_identity_response(
            client,
            email=f"storyboard-profile-{shot_count}@example.com",
        ),
        expected_status=201,
    )
    headers = {"authorization": f"Bearer {identity['access_token']}"}
    workspace_id = UUID(identity["workspace"]["id"])
    actor_id = UUID(identity["user"]["id"])
    project = _data(
        await client.post(
            "/api/v1/projects",
            headers=headers,
            json=project_payload(str(workspace_id), f"分镜性能样本 {shot_count}"),
        ),
        expected_status=201,
    )
    episode = _data(
        await client.post(
            f"/api/v1/projects/{project['id']}/episodes",
            headers=headers,
            json={
                "name": f"{shot_count} 镜头样本",
                "target_duration_ms": max(90_000, shot_count * 3_000),
            },
        ),
        expected_status=201,
    )

    episode_id = UUID(episode["id"])
    source_id = uuid7()
    script_version_id = uuid7()
    scene_id = uuid7()
    dialogue_id = uuid7()
    async with session_factory() as session, session.begin():
        session.add(
            ScriptSource(
                id=source_id,
                workspace_id=workspace_id,
                episode_id=episode_id,
                input_type="text",
                title="已确认虚构性能剧本",
                rights_declaration="仅用于本地性能验证的虚构文本",
                status="active",
                revision=1,
                idempotency_key=f"storyboard-profile-{shot_count}",
            )
        )
        await session.flush()
        session.add(
            ScriptVersion(
                id=script_version_id,
                workspace_id=workspace_id,
                source_id=source_id,
                version_no=1,
                status="published",
                body="雨夜车站\n林澈：有人吗？",
                content_hash=f"{shot_count % 10}" * 64,
                structure_summary={
                    "confirmation_batch_id": str(uuid7()),
                    "source_script_version_id": str(script_version_id),
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
                workspace_id=workspace_id,
                script_version_id=script_version_id,
                position=1,
                heading="雨夜车站",
                location="旧车站月台",
                time_of_day="夜",
                summary="林澈和同伴进入空无一人的月台",
                source_start=0,
                source_end=10,
            )
        )
        await session.flush()
        session.add(
            Dialogue(
                id=dialogue_id,
                workspace_id=workspace_id,
                scene_id=scene_id,
                position=1,
                speaker_candidate="林澈",
                dialogue_kind="spoken",
                text="有人吗？",
                source_start=11,
                source_end=16,
            )
        )
        persisted_episode = await session.scalar(
            select(Episode).where(Episode.id == episode_id).with_for_update()
        )
        assert persisted_episode is not None
        persisted_episode.current_script_version_id = script_version_id
        persisted_episode.revision += 1

    return headers, {
        "workspace_id": workspace_id,
        "actor_id": actor_id,
        "project_id": UUID(project["id"]),
        "episode_id": episode_id,
        "script_version_id": script_version_id,
        "scene_id": scene_id,
        "dialogue_id": dialogue_id,
    }


def _asset_spec(
    kind: Literal["character", "location", "visual_style", "voice"],
    *,
    ordinal: int,
) -> dict[str, object]:
    if kind == "character":
        return {
            "kind": kind,
            "identity": f"虚构角色 {ordinal}",
            "appearance": "黑发，深色风衣，轮廓稳定",
            "age_impression": "二十五岁",
            "temperament": ["克制", "警觉"],
        }
    if kind == "location":
        return {
            "kind": kind,
            "spatial_description": "封闭的旧车站月台",
            "time_weather": "雨夜",
            "visual_elements": ["旧灯箱", "积水"],
            "lighting": "冷蓝顶光",
        }
    if kind == "visual_style":
        return {
            "kind": kind,
            "visual_language": "写实电影感二维漫剧",
            "palette": "冷蓝与低饱和灰",
            "lighting_language": "冷色侧逆光",
            "negative_constraints": ["避免高饱和", "避免比例漂移"],
        }
    return {
        "kind": kind,
        "source_kind": "synthetic_recording",
        "language": "zh-CN",
        "performance_traits": ["克制", "清晰"],
        "allowed_usage": ["ai_short_drama_generation"],
    }


async def _seed_ready_asset_version(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    headers: dict[str, str],
    refs: dict[str, UUID],
    kind: Literal["character", "location", "visual_style", "voice"],
    ordinal: int,
) -> UUID:
    media_kind = "audio" if kind == "voice" else "image"
    suffix = "wav" if media_kind == "audio" else "png"
    mime_type = "audio/wav" if media_kind == "audio" else "image/png"
    purpose = {
        "character": "portrait",
        "location": "environment",
        "visual_style": "style_reference",
        "voice": "voice_sample",
    }[kind]
    media_version_id = await seed_ready_media_version(
        session_factory,
        workspace_id=refs["workspace_id"],
        actor_id=refs["actor_id"],
        kind=media_kind,
        filename=f"storyboard-profile-{kind}-{ordinal}.{suffix}",
        mime_type=mime_type,
    )
    asset = _data(
        await client.post(
            f"/api/v1/projects/{refs['project_id']}/assets",
            headers=headers,
            json={
                "kind": kind,
                "name": f"性能样本 {kind} {ordinal}",
                "aliases": [],
                "tags": ["performance-fixture"],
            },
        ),
        expected_status=201,
    )
    created = _data(
        await client.post(
            f"/api/v1/assets/{asset['id']}/versions",
            headers=headers,
            json={
                "spec": _asset_spec(kind, ordinal=ordinal),
                "prompt_description": f"固定 {kind} 性能样本 {ordinal}",
                "media_references": [
                    {
                        "media_version_id": str(media_version_id),
                        "purpose": purpose,
                        "position": 1,
                    }
                ],
                "source_type": "manual",
                "source_id": None,
                "expected_current_version_id": None,
                "set_as_current": True,
            },
        ),
        expected_status=201,
    )
    version_id = UUID(created["version"]["id"])
    now = datetime.now(UTC)
    _data(
        await client.post(
            "/api/v1/consents",
            headers=headers,
            json={
                "workspace_id": str(refs["workspace_id"]),
                "subject_identity": {
                    "reference": f"synthetic-profile-{kind}-{ordinal}",
                    "kind": "fictional_adult",
                },
                "scope": {
                    "type": "media_usage",
                    "subject_type": "ASSET_VERSION",
                    "subject_id": str(version_id),
                    "rights_holder_role": "synthetic_creator",
                    "rights_types": ["copyright", "image"],
                    "authorized_purposes": ["ai_short_drama_generation"],
                    "channels": ["lanverse_preview"],
                    "regions": ["CN"],
                    "valid_from": (now - timedelta(days=1)).isoformat(),
                    "valid_to": (now + timedelta(days=365)).isoformat(),
                },
                "proof_media_version_ids": [str(media_version_id)],
                "reason": "分镜性能样本授权",
                "idempotency_key": f"storyboard-profile-consent-{version_id}",
            },
        ),
        expected_status=201,
    )
    return version_id


async def _seed_shots(
    session_factory: async_sessionmaker[AsyncSession],
    *,
    refs: dict[str, UUID],
    asset_version_ids: dict[str, UUID],
    shot_count: int,
) -> None:
    spec = ShotSpec.model_validate(
        {
            "schema_version": 1,
            "script_reference": {
                "confirmed_script_version_id": str(refs["script_version_id"]),
                "scene_id": str(refs["scene_id"]),
                "dialogue_ids": [str(refs["dialogue_id"])],
            },
            "narrative": {
                "purpose": "固定规模分镜准备度性能样本",
                "continuity_note": "人物服装与雨夜光线连续",
            },
            "visual": {
                "shot_size": "medium",
                "camera_angle": "eye_level",
                "camera_movement": "static",
                "composition": "两名角色位于月台中景",
                "environment": "雨夜旧车站月台",
                "subject_placements": [
                    {"subject_key": "hero", "placement": "画面左侧"},
                    {"subject_key": "companion", "placement": "画面右侧"},
                ],
                "mood_lighting": "冷蓝侧逆光",
            },
            "action_beats": [
                {"beat_key": "enter", "order": 1, "description": "两人进入月台"},
                {"beat_key": "pause", "order": 2, "description": "林澈停下并回头"},
            ],
            "dialogue_or_narration": [
                {
                    "source_dialogue_id": str(refs["dialogue_id"]),
                    "beat_key": "pause",
                    "speaker_subject_key": "hero",
                    "render_as_audio": True,
                    "performance_note": "保持警惕",
                }
            ],
            "duration_ms": 3_000,
            "audio_intent": {"ambient": "雨声", "sound_effects": []},
            "generation_intent": {
                "mode": "text_to_video",
                "first_frame": None,
                "last_frame": None,
                "keyframe_notes": None,
            },
        }
    )
    reference_requests = [
        AssetReferenceRequest(
            slot_key="location-main",
            role="location",
            asset_version_id=asset_version_ids["location"],
            subject_key=None,
        ),
        AssetReferenceRequest(
            slot_key="character-hero",
            role="character",
            asset_version_id=asset_version_ids["character-1"],
            subject_key="hero",
        ),
        AssetReferenceRequest(
            slot_key="character-companion",
            role="character",
            asset_version_id=asset_version_ids["character-2"],
            subject_key="companion",
        ),
        AssetReferenceRequest(
            slot_key="voice-hero",
            role="voice",
            asset_version_id=asset_version_ids["voice"],
            subject_key="hero",
        ),
        AssetReferenceRequest(
            slot_key="style-main",
            role="visual_style",
            asset_version_id=asset_version_ids["visual_style"],
            subject_key=None,
        ),
    ]
    hashes = storyboard_content_hashes(spec, reference_requests)
    async with session_factory() as session, session.begin():
        shots: list[Shot] = []
        versions: list[ShotSpecVersion] = []
        references: list[AssetReference] = []
        for position in range(1, shot_count + 1):
            shot_id = uuid7()
            spec_version_id = uuid7()
            shots.append(
                Shot(
                    id=shot_id,
                    workspace_id=refs["workspace_id"],
                    episode_id=refs["episode_id"],
                    position=position,
                    title=f"性能镜头 {position:03d}",
                    source_script_version_id=refs["script_version_id"],
                    source_scene_id=refs["scene_id"],
                    source_candidate_id=None,
                    creation_key=f"profile-{shot_count}-{position:03d}",
                    status="active",
                    current_spec_version_id=spec_version_id,
                    revision=1,
                    created_by=refs["actor_id"],
                )
            )
            versions.append(
                ShotSpecVersion(
                    id=spec_version_id,
                    workspace_id=refs["workspace_id"],
                    shot_id=shot_id,
                    version_no=1,
                    schema_version=1,
                    spec=spec.model_dump(mode="json"),
                    content_hash=hashes.content_hash,
                    input_hash=hashes.input_hash,
                    created_by=refs["actor_id"],
                )
            )
            references.extend(
                AssetReference(
                    id=uuid7(),
                    workspace_id=refs["workspace_id"],
                    shot_spec_version_id=spec_version_id,
                    slot_key=request.slot_key,
                    role=request.role,
                    asset_version_id=request.asset_version_id,
                    subject_key=request.subject_key,
                )
                for request in reference_requests
            )
        session.add_all(shots)
        await session.flush()
        session.add_all(versions)
        await session.flush()
        session.add_all(references)


async def _build_profile_episode(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    shot_count: int,
) -> tuple[dict[str, str], str]:
    headers, refs = await _seed_confirmed_episode(
        client,
        session_factory,
        shot_count=shot_count,
    )
    asset_version_ids = {
        "location": await _seed_ready_asset_version(
            client,
            session_factory,
            headers=headers,
            refs=refs,
            kind="location",
            ordinal=1,
        ),
        "character-1": await _seed_ready_asset_version(
            client,
            session_factory,
            headers=headers,
            refs=refs,
            kind="character",
            ordinal=1,
        ),
        "character-2": await _seed_ready_asset_version(
            client,
            session_factory,
            headers=headers,
            refs=refs,
            kind="character",
            ordinal=2,
        ),
        "voice": await _seed_ready_asset_version(
            client,
            session_factory,
            headers=headers,
            refs=refs,
            kind="voice",
            ordinal=1,
        ),
        "visual_style": await _seed_ready_asset_version(
            client,
            session_factory,
            headers=headers,
            refs=refs,
            kind="visual_style",
            ordinal=1,
        ),
    }
    await _seed_shots(
        session_factory,
        refs=refs,
        asset_version_ids=asset_version_ids,
        shot_count=shot_count,
    )
    return headers, str(refs["episode_id"])


def _free_loopback_port() -> int:
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as listener:
        listener.bind(("127.0.0.1", 0))
        return cast(int, listener.getsockname()[1])


async def _wait_for_server(
    process: asyncio.subprocess.Process,
    base_url: str,
) -> None:
    async with httpx.AsyncClient(base_url=base_url, timeout=1.0) as client:
        for _ in range(100):
            if process.returncode is not None:
                raise RuntimeError("performance API process exited during startup")
            try:
                response = await client.get("/healthz")
                if response.status_code == 200:
                    return
            except httpx.HTTPError:
                pass
            await asyncio.sleep(0.1)
    raise RuntimeError("performance API process did not become healthy")


async def _request_profile_pair(
    client: httpx.AsyncClient,
    *,
    episode_id: str,
    headers: dict[str, str],
    expected_count: int,
) -> tuple[str, str]:
    list_request_id = str(uuid7())
    readiness_request_id = str(uuid7())
    listed = await client.get(
        f"/api/v1/episodes/{episode_id}/shots",
        headers={**headers, "x-request-id": list_request_id},
    )
    assert listed.status_code == 200, listed.text
    assert len(listed.json()["data"]["items"]) == expected_count
    readiness = await client.get(
        f"/api/v1/episodes/{episode_id}/shot-readiness",
        headers={**headers, "x-request-id": readiness_request_id},
    )
    assert readiness.status_code == 200, readiness.text
    summary = readiness.json()["data"]["summary"]
    assert summary == {
        "total": expected_count,
        "ready": expected_count,
        "blocked": 0,
        "unavailable": 0,
    }
    return list_request_id, readiness_request_id


async def _exercise_reorder(
    client: httpx.AsyncClient,
    *,
    episode_id: str,
    headers: dict[str, str],
) -> None:
    initial = _data(
        await client.get(
            f"/api/v1/episodes/{episode_id}/shots",
            headers={**headers, "x-request-id": str(uuid7())},
        ),
        expected_status=200,
    )
    shot_ids = [item["id"] for item in initial["items"]]
    order_hash = initial["order_hash"]
    assert len(shot_ids) == 120
    for _ in range(30):
        shot_ids = [*shot_ids[1:], shot_ids[0]]
        reordered = _data(
            await client.post(
                f"/api/v1/episodes/{episode_id}/shots/reorder",
                headers={
                    **headers,
                    "x-request-id": str(uuid7()),
                },
                json={
                    "shot_ids": shot_ids,
                    "expected_order_hash": order_hash,
                },
            ),
            expected_status=200,
        )
        assert [item["position"] for item in reordered["items"]] == list(
            range(1, 121)
        )
        assert [item["id"] for item in reordered["items"]] == shot_ids
        order_hash = reordered["order_hash"]


def _server_durations(log_text: str) -> dict[str, float]:
    durations: dict[str, float] = {}
    for line in log_text.splitlines():
        try:
            payload = json.loads(line)
        except json.JSONDecodeError:
            continue
        request_id = payload.get("request_id")
        duration_ms = payload.get("duration_ms")
        if isinstance(request_id, str) and isinstance(duration_ms, (int, float)):
            durations[request_id] = float(duration_ms)
    return durations


def _percentile_95(samples: list[float]) -> float:
    assert len(samples) == 50
    return sorted(samples)[math.ceil(len(samples) * 0.95) - 1]


@pytest.mark.skipif(
    not PERFORMANCE_ENABLED,
    reason="set RUN_STORYBOARD_PERFORMANCE=1 for the fixed S3 profile",
)
@pytest.mark.asyncio
async def test_fixed_storyboard_performance_profile(
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    engine = session_factory.kw.get("bind")
    assert isinstance(engine, AsyncEngine)
    database_url = engine.url.render_as_string(hide_password=False)
    port = _free_loopback_port()
    base_url = f"http://127.0.0.1:{port}"
    environment = {
        "PATH": os.environ.get("PATH", ""),
        "LANG": os.environ.get("LANG", "en_US.UTF-8"),
        "ENVIRONMENT": "production",
        "DATABASE_URL": database_url,
        "JWT_SECRET_KEY": "storyboard-performance-secret-at-least-32-bytes",
        "DEEPSEEK_API_KEY": "",
        "LOG_LEVEL": "INFO",
    }
    sample_request_ids: dict[int, list[tuple[str, str]]] = {36: [], 120: []}

    with tempfile.TemporaryFile(mode="w+", encoding="utf-8") as server_log:
        process = await asyncio.create_subprocess_exec(
            sys.executable,
            "-m",
            "uvicorn",
            "app.main:app",
            "--host",
            "127.0.0.1",
            "--port",
            str(port),
            "--workers",
            "1",
            "--no-access-log",
            cwd=str(BACKEND_ROOT),
            env=environment,
            stdout=server_log,
            stderr=asyncio.subprocess.STDOUT,
        )
        try:
            await _wait_for_server(process, base_url)
            async with httpx.AsyncClient(base_url=base_url, timeout=10.0) as client:
                episodes = {
                    shot_count: await _build_profile_episode(
                        client,
                        session_factory,
                        shot_count=shot_count,
                    )
                    for shot_count in (12, 36, 120)
                }
                headers_12, episode_12 = episodes[12]
                await _request_profile_pair(
                    client,
                    episode_id=episode_12,
                    headers=headers_12,
                    expected_count=12,
                )
                for shot_count in (36, 120):
                    headers, episode_id = episodes[shot_count]
                    for _ in range(10):
                        await _request_profile_pair(
                            client,
                            episode_id=episode_id,
                            headers=headers,
                            expected_count=shot_count,
                        )
                    for _ in range(50):
                        sample_request_ids[shot_count].append(
                            await _request_profile_pair(
                                client,
                                episode_id=episode_id,
                                headers=headers,
                                expected_count=shot_count,
                            )
                        )
                headers_120, episode_120 = episodes[120]
                await _exercise_reorder(
                    client,
                    episode_id=episode_120,
                    headers=headers_120,
                )
        finally:
            process.terminate()
            try:
                await asyncio.wait_for(process.wait(), timeout=10)
            except TimeoutError:
                process.kill()
                await asyncio.wait_for(process.wait(), timeout=10)
        server_log.seek(0)
        durations = _server_durations(server_log.read())

    p95_results: dict[int, float] = {}
    for shot_count, request_ids in sample_request_ids.items():
        combined_samples = [
            durations[list_request_id] + durations[readiness_request_id]
            for list_request_id, readiness_request_id in request_ids
        ]
        p95_results[shot_count] = _percentile_95(combined_samples)

    summary = {
        "environment": {
            "macos": platform.mac_ver()[0],
            "machine": platform.machine(),
            "python": platform.python_version(),
        },
        "protocol": {
            "fixture_sizes": [12, 36, 120],
            "warmups": 10,
            "samples": 50,
            "reorders_120": 30,
            "processes": 1,
        },
        "server_p95_ms": {str(key): value for key, value in p95_results.items()},
    }
    print(json.dumps(summary, ensure_ascii=False, sort_keys=True))
    assert p95_results[36] <= 800
    assert p95_results[120] <= 2_000
