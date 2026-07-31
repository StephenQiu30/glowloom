import asyncio
from typing import Any

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.messaging.models import OutboxEvent
from app.modules.projects.models import Episode
from app.modules.scripts.models import Dialogue, Scene, ScriptSource, ScriptVersion
from app.modules.storyboards.models import Shot


async def _identity(
    client: httpx.AsyncClient,
    *,
    email: str,
) -> tuple[dict[str, str], str]:
    response = await client.post(
        "/api/v1/auth/register",
        json={
            "email": email,
            "password": "a-secure-script-password",
            "display_name": "剧本负责人",
        },
    )
    assert response.status_code == 201
    data = response.json()["data"]
    return {"authorization": f"Bearer {data['access_token']}"}, data["workspace"]["id"]


async def _episode(
    client: httpx.AsyncClient,
    headers: dict[str, str],
    workspace_id: str,
) -> dict[str, Any]:
    project_response = await client.post(
        "/api/v1/projects",
        headers=headers,
        json={
            "workspace_id": workspace_id,
            "name": "剧本验收项目",
            "aspect_ratio": "9:16",
            "language": "zh-CN",
            "target_duration_ms": 90000,
        },
    )
    assert project_response.status_code == 201
    project = project_response.json()["data"]
    episode_response = await client.post(
        f"/api/v1/projects/{project['id']}/episodes",
        headers=headers,
        json={"name": "第一集", "target_duration_ms": 90000},
    )
    assert episode_response.status_code == 201
    return episode_response.json()["data"]


def _import_payload(*, body: str = "第一场\r\n角色甲：开始吧。") -> dict[str, str]:
    return {
        "input_type": "text",
        "title": "第一集原始剧本",
        "body": body,
        "rights_declaration": "确认拥有该测试文本的使用权",
        "idempotency_key": "script-import-001",
    }


async def _import_script(
    client: httpx.AsyncClient,
    headers: dict[str, str],
    episode_id: str,
    *,
    body: str = "第一场\n角色甲：开始吧。",
    idempotency_key: str = "script-import-001",
) -> dict[str, Any]:
    response = await client.post(
        f"/api/v1/episodes/{episode_id}/script-sources",
        headers=headers,
        json=_import_payload(body=body) | {"idempotency_key": idempotency_key},
    )
    assert response.status_code == 201
    return response.json()["data"]


def _publish_payload(
    body: str,
    expected_current_version_id: str | None,
) -> dict[str, str | None]:
    return {
        "body": body,
        "expected_current_version_id": expected_current_version_id,
    }


@pytest.mark.asyncio
async def test_text_import_is_idempotent_private_and_creates_immutable_version(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="script-owner@example.com"
    )
    episode = await _episode(client, headers, workspace_id)
    endpoint = f"/api/v1/episodes/{episode['id']}/script-sources"

    created = await client.post(endpoint, headers=headers, json=_import_payload())
    assert created.status_code == 201
    imported = created.json()["data"]
    source = imported["source"]
    version = imported["version"]
    assert source["episode_id"] == episode["id"]
    assert source["input_type"] == "text"
    assert source["status"] == "active"
    assert source["revision"] == 1
    assert "idempotency_key" not in source
    assert version["source_id"] == source["id"]
    assert version["version_no"] == 1
    assert version["status"] == "draft"
    assert version["body"] == "第一场\n角色甲：开始吧。"
    assert len(version["content_hash"]) == 64

    repeated = await client.post(
        endpoint,
        headers=headers,
        json=_import_payload(body="第一场\n角色甲：开始吧。"),
    )
    assert repeated.status_code == 201
    assert repeated.json()["data"] == imported

    conflicting = await client.post(
        endpoint,
        headers=headers,
        json=_import_payload(body="同一个键不能替换成另一份正文"),
    )
    assert conflicting.status_code == 409
    assert conflicting.json()["error"]["code"] == "resource_conflict"

    fetched_source = await client.get(
        f"/api/v1/script-sources/{source['id']}", headers=headers
    )
    assert fetched_source.status_code == 200
    assert fetched_source.json()["data"] == source

    sources = await client.get(
        f"/api/v1/episodes/{episode['id']}/script-sources", headers=headers
    )
    assert sources.status_code == 200
    assert sources.json()["data"] == {
        "items": [source],
        "total": 1,
        "limit": 20,
        "offset": 0,
    }

    history = await client.get(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
    )
    assert history.status_code == 200
    assert history.json()["data"]["total"] == 1
    assert history.json()["data"]["items"] == [version]

    fetched = await client.get(
        f"/api/v1/script-versions/{version['id']}", headers=headers
    )
    assert fetched.status_code == 200
    assert fetched.json()["data"] == version
    immutable = await client.patch(
        f"/api/v1/script-versions/{version['id']}",
        headers=headers,
        json={"body": "不允许覆盖"},
    )
    assert immutable.status_code == 405

    too_long = await client.post(
        endpoint,
        headers=headers,
        json=_import_payload(body="字" * 20_001)
        | {"idempotency_key": "too-long"},
    )
    assert too_long.status_code == 422
    blank = await client.post(
        endpoint,
        headers=headers,
        json=_import_payload(body=" \r\n ") | {"idempotency_key": "blank"},
    )
    assert blank.status_code == 422

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScriptSource)) == 1
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 1
        assert await session.scalar(select(func.count()).select_from(OutboxEvent)) == 0


@pytest.mark.asyncio
async def test_import_is_concurrency_safe_and_cross_workspace_hidden(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    owner_headers, workspace_id = await _identity(
        client, email="script-concurrent-owner@example.com"
    )
    episode = await _episode(client, owner_headers, workspace_id)
    endpoint = f"/api/v1/episodes/{episode['id']}/script-sources"

    first, second = await asyncio.gather(
        client.post(endpoint, headers=owner_headers, json=_import_payload()),
        client.post(endpoint, headers=owner_headers, json=_import_payload()),
    )
    assert first.status_code == 201
    assert second.status_code == 201
    assert first.json()["data"] == second.json()["data"]
    imported = first.json()["data"]

    stranger_headers, _ = await _identity(
        client, email="script-stranger@example.com"
    )
    hidden_source = await client.get(
        f"/api/v1/script-sources/{imported['source']['id']}",
        headers=stranger_headers,
    )
    hidden_version = await client.get(
        f"/api/v1/script-versions/{imported['version']['id']}",
        headers=stranger_headers,
    )
    assert hidden_source.status_code == 404
    assert hidden_version.status_code == 404

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScriptSource)) == 1
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 1


@pytest.mark.asyncio
async def test_archived_episode_rejects_new_script_source(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await _identity(
        client, email="script-archived-owner@example.com"
    )
    episode = await _episode(client, headers, workspace_id)
    archived = await client.post(
        f"/api/v1/episodes/{episode['id']}/archive",
        headers=headers,
        json={"expected_revision": episode["revision"]},
    )
    assert archived.status_code == 200

    response = await client.post(
        f"/api/v1/episodes/{episode['id']}/script-sources",
        headers=headers,
        json=_import_payload(),
    )
    assert response.status_code == 409
    assert response.json()["error"]["code"] == "state_conflict"


@pytest.mark.asyncio
async def test_publish_appends_immutable_version_and_switches_episode_current(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="script-publish-owner@example.com"
    )
    episode = await _episode(client, headers, workspace_id)
    imported = await _import_script(client, headers, episode["id"])
    source = imported["source"]
    draft = imported["version"]

    published_response = await client.post(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
        json=_publish_payload("第二场\r\n角色乙：继续。", None),
    )
    assert published_response.status_code == 201
    result = published_response.json()["data"]
    published = result["version"]
    assert published["source_id"] == source["id"]
    assert published["version_no"] == 2
    assert published["status"] == "published"
    assert published["body"] == "第二场\n角色乙：继续。"
    assert result["current"] == {
        "episode_id": episode["id"],
        "current_script_version_id": published["id"],
        "episode_revision": 2,
        "impact": {
            "previous_script_version_id": None,
            "current_script_version_id": published["id"],
            "affected_shot_ids": [],
        },
    }

    fetched_episode = await client.get(
        f"/api/v1/episodes/{episode['id']}", headers=headers
    )
    assert fetched_episode.status_code == 200
    assert fetched_episode.json()["data"]["current_script_version_id"] == published["id"]
    assert fetched_episode.json()["data"]["revision"] == 2

    history = await client.get(
        f"/api/v1/script-sources/{source['id']}/versions", headers=headers
    )
    assert history.status_code == 200
    assert history.json()["data"]["items"] == [draft, published]
    immutable = await client.patch(
        f"/api/v1/script-versions/{published['id']}",
        headers=headers,
        json={"body": "不能覆盖发布版本"},
    )
    assert immutable.status_code == 405

    stale = await client.post(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
        json=_publish_payload("过期编辑器不应创建版本", None),
    )
    assert stale.status_code == 409
    assert stale.json()["error"]["code"] == "version_conflict"
    assert stale.json()["error"]["details"]["current_script_version_id"] == published["id"]

    blank = await client.post(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
        json=_publish_payload(" \r\n ", published["id"]),
    )
    assert blank.status_code == 422
    too_long = await client.post(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
        json=_publish_payload("字" * 20_001, published["id"]),
    )
    assert too_long.status_code == 422

    async with session_factory() as session:
        versions = list(
            await session.scalars(
                select(ScriptVersion).order_by(ScriptVersion.version_no)
            )
        )
        assert [(item.version_no, item.status, item.body) for item in versions] == [
            (1, "draft", "第一场\n角色甲：开始吧。"),
            (2, "published", "第二场\n角色乙：继续。"),
        ]
        assert await session.scalar(select(func.count()).select_from(OutboxEvent)) == 0


@pytest.mark.asyncio
async def test_concurrent_publish_creates_exactly_one_new_version(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="script-publish-concurrent@example.com"
    )
    episode = await _episode(client, headers, workspace_id)
    imported = await _import_script(client, headers, episode["id"])
    source_id = imported["source"]["id"]
    endpoint = f"/api/v1/script-sources/{source_id}/versions"

    first, second = await asyncio.gather(
        client.post(
            endpoint,
            headers=headers,
            json=_publish_payload("并发版本甲", None),
        ),
        client.post(
            endpoint,
            headers=headers,
            json=_publish_payload("并发版本乙", None),
        ),
    )
    responses = [first, second]
    assert sorted(response.status_code for response in responses) == [201, 409]
    winner = next(response for response in responses if response.status_code == 201)
    conflict = next(response for response in responses if response.status_code == 409)
    current_version_id = winner.json()["data"]["version"]["id"]
    assert conflict.json()["error"]["code"] == "version_conflict"
    assert (
        conflict.json()["error"]["details"]["current_script_version_id"]
        == current_version_id
    )

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 2
        version_numbers = list(
            await session.scalars(
                select(ScriptVersion.version_no).order_by(ScriptVersion.version_no)
            )
        )
        assert version_numbers == [1, 2]
        stored_episode = await session.get(Episode, episode["id"])
        assert stored_episode is not None
        assert str(stored_episode.current_script_version_id) == current_version_id


@pytest.mark.asyncio
async def test_current_switch_accepts_only_published_version_from_same_episode(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await _identity(
        client, email="script-current-owner@example.com"
    )
    episode = await _episode(client, headers, workspace_id)
    imported = await _import_script(client, headers, episode["id"])
    source_id = imported["source"]["id"]
    draft_id = imported["version"]["id"]

    second_response = await client.post(
        f"/api/v1/script-sources/{source_id}/versions",
        headers=headers,
        json=_publish_payload("已发布版本二", None),
    )
    assert second_response.status_code == 201
    second = second_response.json()["data"]["version"]
    third_response = await client.post(
        f"/api/v1/script-sources/{source_id}/versions",
        headers=headers,
        json=_publish_payload("已发布版本三", second["id"]),
    )
    assert third_response.status_code == 201
    third = third_response.json()["data"]["version"]

    switched = await client.post(
        f"/api/v1/episodes/{episode['id']}/current-script-version",
        headers=headers,
        json={
            "version_id": second["id"],
            "expected_current_version_id": third["id"],
        },
    )
    assert switched.status_code == 200
    assert switched.json()["data"] == {
        "episode_id": episode["id"],
        "current_script_version_id": second["id"],
        "episode_revision": 4,
        "impact": {
            "previous_script_version_id": third["id"],
            "current_script_version_id": second["id"],
            "affected_shot_ids": [],
        },
    }

    draft_rejected = await client.post(
        f"/api/v1/episodes/{episode['id']}/current-script-version",
        headers=headers,
        json={
            "version_id": draft_id,
            "expected_current_version_id": second["id"],
        },
    )
    assert draft_rejected.status_code == 409
    assert draft_rejected.json()["error"]["code"] == "state_conflict"

    stale = await client.post(
        f"/api/v1/episodes/{episode['id']}/current-script-version",
        headers=headers,
        json={
            "version_id": third["id"],
            "expected_current_version_id": third["id"],
        },
    )
    assert stale.status_code == 409
    assert stale.json()["error"]["code"] == "version_conflict"
    assert stale.json()["error"]["details"]["current_script_version_id"] == second["id"]

    other_episode = await _episode(client, headers, workspace_id)
    other_import = await _import_script(
        client,
        headers,
        other_episode["id"],
        idempotency_key="other-script-import",
    )
    other_published_response = await client.post(
        f"/api/v1/script-sources/{other_import['source']['id']}/versions",
        headers=headers,
        json=_publish_payload("另一个单集的版本", None),
    )
    assert other_published_response.status_code == 201
    other_published = other_published_response.json()["data"]["version"]
    wrong_episode = await client.post(
        f"/api/v1/episodes/{episode['id']}/current-script-version",
        headers=headers,
        json={
            "version_id": other_published["id"],
            "expected_current_version_id": second["id"],
        },
    )
    assert wrong_episode.status_code == 409
    assert wrong_episode.json()["error"]["code"] == "resource_conflict"

    stranger_headers, _ = await _identity(
        client, email="script-current-stranger@example.com"
    )
    hidden = await client.post(
        f"/api/v1/episodes/{episode['id']}/current-script-version",
        headers=stranger_headers,
        json={
            "version_id": third["id"],
            "expected_current_version_id": second["id"],
        },
    )
    assert hidden.status_code == 404


@pytest.mark.asyncio
async def test_publishing_new_current_version_reports_active_shots_with_older_script(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="script-impact-owner@example.com"
    )
    episode = await _episode(client, headers, workspace_id)
    imported = await _import_script(client, headers, episode["id"])
    published_response = await client.post(
        f"/api/v1/script-sources/{imported['source']['id']}/versions",
        headers=headers,
        json=_publish_payload("第一场\n角色甲：旧版镜头。", None),
    )
    assert published_response.status_code == 201
    published = published_response.json()["data"]["version"]

    scene_id = uuid7()
    shot_id = uuid7()
    async with session_factory() as session, session.begin():
        stored_version = await session.get(ScriptVersion, published["id"])
        assert stored_version is not None
        session.add(
            Scene(
                id=scene_id,
                workspace_id=stored_version.workspace_id,
                script_version_id=stored_version.id,
                position=1,
                heading="第一场",
                location="雨巷",
                time_of_day="夜",
                summary="旧版本镜头来源",
                source_start=0,
                source_end=len(stored_version.body),
            )
        )
        await session.flush()
        session.add(
            Shot(
                id=shot_id,
                workspace_id=stored_version.workspace_id,
                episode_id=episode["id"],
                position=1,
                title="仍引用旧剧本的镜头",
                source_script_version_id=stored_version.id,
                source_scene_id=scene_id,
                source_candidate_id=None,
                creation_key="script-impact-shot",
                status="active",
                current_spec_version_id=None,
                revision=1,
                created_by=stored_version.created_by,
            )
        )

    next_response = await client.post(
        f"/api/v1/script-sources/{imported['source']['id']}/versions",
        headers=headers,
        json=_publish_payload("第一场\n角色甲：新版镜头。", published["id"]),
    )

    assert next_response.status_code == 201
    result = next_response.json()["data"]
    assert result["current"]["impact"] == {
        "previous_script_version_id": published["id"],
        "current_script_version_id": result["version"]["id"],
        "affected_shot_ids": [str(shot_id)],
    }

    async with session_factory() as session:
        stored_shot = await session.get(Shot, shot_id)
        assert stored_shot is not None
        assert str(stored_shot.source_script_version_id) == published["id"]
        assert stored_shot.current_spec_version_id is None


@pytest.mark.asyncio
async def test_concurrent_current_switch_reports_winner_without_mutating_versions(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="script-current-concurrent@example.com"
    )
    episode = await _episode(client, headers, workspace_id)
    imported = await _import_script(client, headers, episode["id"])
    source_id = imported["source"]["id"]
    current_id: str | None = None
    published_versions: list[dict[str, Any]] = []
    for body in ("已发布版本二", "已发布版本三", "已发布版本四"):
        response = await client.post(
            f"/api/v1/script-sources/{source_id}/versions",
            headers=headers,
            json=_publish_payload(body, current_id),
        )
        assert response.status_code == 201
        version = response.json()["data"]["version"]
        published_versions.append(version)
        current_id = version["id"]

    second, third, fourth = published_versions
    endpoint = f"/api/v1/episodes/{episode['id']}/current-script-version"
    first_response, second_response = await asyncio.gather(
        client.post(
            endpoint,
            headers=headers,
            json={
                "version_id": second["id"],
                "expected_current_version_id": fourth["id"],
            },
        ),
        client.post(
            endpoint,
            headers=headers,
            json={
                "version_id": third["id"],
                "expected_current_version_id": fourth["id"],
            },
        ),
    )
    responses = [first_response, second_response]
    assert sorted(response.status_code for response in responses) == [200, 409]
    winner = next(response for response in responses if response.status_code == 200)
    conflict = next(response for response in responses if response.status_code == 409)
    result = winner.json()["data"]
    assert result["current_script_version_id"] in {second["id"], third["id"]}
    assert result["episode_revision"] == 5
    assert result["impact"] == {
        "previous_script_version_id": fourth["id"],
        "current_script_version_id": result["current_script_version_id"],
        "affected_shot_ids": [],
    }
    assert conflict.json()["error"]["code"] == "version_conflict"
    assert (
        conflict.json()["error"]["details"]["current_script_version_id"]
        == result["current_script_version_id"]
    )

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 4
        assert await session.scalar(select(func.count()).select_from(Scene)) == 0
        assert await session.scalar(select(func.count()).select_from(Dialogue)) == 0
        stored_episode = await session.get(Episode, episode["id"])
        assert stored_episode is not None
        assert (
            str(stored_episode.current_script_version_id)
            == result["current_script_version_id"]
        )


@pytest.mark.asyncio
async def test_delete_draft_version_requires_confirmation_and_is_private(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="script-delete-draft@example.com"
    )
    episode = await _episode(client, headers, workspace_id)
    imported = await _import_script(client, headers, episode["id"])
    draft = imported["version"]
    endpoint = f"/api/v1/script-versions/{draft['id']}"

    unconfirmed = await client.delete(endpoint, headers=headers)
    assert unconfirmed.status_code == 422
    declined = await client.delete(
        endpoint,
        headers=headers,
        params={"confirm": "false"},
    )
    assert declined.status_code == 422
    assert declined.json()["error"]["code"] == "invalid_request"
    assert (await client.get(endpoint, headers=headers)).status_code == 200

    stranger_headers, _ = await _identity(
        client, email="script-delete-draft-stranger@example.com"
    )
    hidden = await client.delete(
        endpoint,
        headers=stranger_headers,
        params={"confirm": "true"},
    )
    assert hidden.status_code == 404

    deleted = await client.delete(
        endpoint,
        headers=headers,
        params={"confirm": "true"},
    )
    assert deleted.status_code == 200
    assert deleted.json()["data"] == {
        "deleted": True,
        "script_version_id": draft["id"],
    }
    assert (await client.get(endpoint, headers=headers)).status_code == 404
    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 0


@pytest.mark.asyncio
async def test_delete_version_returns_current_and_extraction_blockers(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="script-delete-blockers@example.com"
    )
    episode = await _episode(client, headers, workspace_id)
    imported = await _import_script(client, headers, episode["id"])
    source_id = imported["source"]["id"]
    published_response = await client.post(
        f"/api/v1/script-sources/{source_id}/versions",
        headers=headers,
        json=_publish_payload("已提取版本", None),
    )
    assert published_response.status_code == 201
    extracted_version = published_response.json()["data"]["version"]
    extraction_response = await client.post(
        f"/api/v1/script-versions/{extracted_version['id']}/extractions",
        headers=headers,
        json={"scope": "full", "idempotency_key": "delete-blocker-extraction"},
    )
    assert extraction_response.status_code == 202
    batch = extraction_response.json()["data"]
    next_response = await client.post(
        f"/api/v1/script-sources/{source_id}/versions",
        headers=headers,
        json=_publish_payload("当前版本", extracted_version["id"]),
    )
    assert next_response.status_code == 201
    current_version = next_response.json()["data"]["version"]

    extracted_delete = await client.delete(
        f"/api/v1/script-versions/{extracted_version['id']}",
        headers=headers,
        params={"confirm": "true"},
    )
    assert extracted_delete.status_code == 409
    extracted_error = extracted_delete.json()["error"]
    assert extracted_error["code"] == "state_conflict"
    assert extracted_error["next_action"] == "review_script_version_delete_blockers"
    assert [
        blocker["code"] for blocker in extracted_error["details"]["blockers"]
    ] == ["VERSION_NOT_DRAFT", "HAS_EXTRACTION_BATCH"]
    assert extracted_error["details"]["blockers"][1]["resource_id"] == batch["id"]

    current_delete = await client.delete(
        f"/api/v1/script-versions/{current_version['id']}",
        headers=headers,
        params={"confirm": "true"},
    )
    assert current_delete.status_code == 409
    assert [
        blocker["code"]
        for blocker in current_delete.json()["error"]["details"]["blockers"]
    ] == ["VERSION_NOT_DRAFT", "CURRENT_VERSION"]

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 3


@pytest.mark.asyncio
async def test_source_archive_restore_keeps_versions_and_current_reference(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, workspace_id = await _identity(
        client, email="script-source-lifecycle@example.com"
    )
    episode = await _episode(client, headers, workspace_id)
    imported = await _import_script(client, headers, episode["id"])
    source = imported["source"]
    draft = imported["version"]
    published_response = await client.post(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
        json=_publish_payload("归档前发布版本", None),
    )
    assert published_response.status_code == 201
    published = published_response.json()["data"]["version"]

    archived_response = await client.post(
        f"/api/v1/script-sources/{source['id']}/archive",
        headers=headers,
        json={"expected_revision": 1},
    )
    assert archived_response.status_code == 200
    archived = archived_response.json()["data"]
    assert archived["status"] == "archived"
    assert archived["revision"] == 2

    fetched = await client.get(
        f"/api/v1/script-sources/{source['id']}", headers=headers
    )
    assert fetched.status_code == 200
    assert fetched.json()["data"] == archived
    history = await client.get(
        f"/api/v1/script-sources/{source['id']}/versions", headers=headers
    )
    assert history.status_code == 200
    assert history.json()["data"]["items"] == [draft, published]
    fetched_episode = await client.get(
        f"/api/v1/episodes/{episode['id']}", headers=headers
    )
    assert fetched_episode.json()["data"]["current_script_version_id"] == published["id"]

    blocked_publish = await client.post(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
        json=_publish_payload("归档来源不能继续发布", published["id"]),
    )
    assert blocked_publish.status_code == 409
    assert blocked_publish.json()["error"]["code"] == "state_conflict"
    stale_restore = await client.post(
        f"/api/v1/script-sources/{source['id']}/restore",
        headers=headers,
        json={"expected_revision": 1},
    )
    assert stale_restore.status_code == 409
    assert stale_restore.json()["error"]["code"] == "version_conflict"
    assert stale_restore.json()["error"]["details"]["current_revision"] == 2

    restored_response = await client.post(
        f"/api/v1/script-sources/{source['id']}/restore",
        headers=headers,
        json={"expected_revision": 2},
    )
    assert restored_response.status_code == 200
    restored = restored_response.json()["data"]
    assert restored["status"] == "active"
    assert restored["revision"] == 3
    next_publish = await client.post(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
        json=_publish_payload("恢复后版本", published["id"]),
    )
    assert next_publish.status_code == 201
    assert next_publish.json()["data"]["version"]["version_no"] == 3

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 3


@pytest.mark.asyncio
async def test_version_diff_is_derived_private_and_limited_to_one_source(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await _identity(
        client, email="script-diff-owner@example.com"
    )
    episode = await _episode(client, headers, workspace_id)
    imported = await _import_script(
        client,
        headers,
        episode["id"],
        body="第一场\n甲：开始\n保留行",
    )
    draft = imported["version"]
    published_response = await client.post(
        f"/api/v1/script-sources/{imported['source']['id']}/versions",
        headers=headers,
        json=_publish_payload("第一场\n甲：继续\n新增行\n保留行", None),
    )
    assert published_response.status_code == 201
    published = published_response.json()["data"]["version"]

    diff_response = await client.get(
        f"/api/v1/script-versions/{draft['id']}/diff",
        headers=headers,
        params={"other_version_id": published["id"]},
    )
    assert diff_response.status_code == 200
    diff = diff_response.json()["data"]
    assert diff["base_version_id"] == draft["id"]
    assert diff["target_version_id"] == published["id"]
    assert diff["added_lines"] == 2
    assert diff["removed_lines"] == 1
    assert "-甲：开始" in diff["diff_lines"]
    assert "+甲：继续" in diff["diff_lines"]
    assert "+新增行" in diff["diff_lines"]

    same_response = await client.get(
        f"/api/v1/script-versions/{draft['id']}/diff",
        headers=headers,
        params={"other_version_id": draft["id"]},
    )
    assert same_response.status_code == 200
    assert same_response.json()["data"]["added_lines"] == 0
    assert same_response.json()["data"]["removed_lines"] == 0
    assert same_response.json()["data"]["diff_lines"] == []

    other_source = await _import_script(
        client,
        headers,
        episode["id"],
        idempotency_key="script-diff-other-source",
    )
    cross_source = await client.get(
        f"/api/v1/script-versions/{draft['id']}/diff",
        headers=headers,
        params={"other_version_id": other_source["version"]["id"]},
    )
    assert cross_source.status_code == 409
    assert cross_source.json()["error"]["code"] == "resource_conflict"

    stranger_headers, _ = await _identity(
        client, email="script-diff-stranger@example.com"
    )
    hidden = await client.get(
        f"/api/v1/script-versions/{draft['id']}/diff",
        headers=stranger_headers,
        params={"other_version_id": published["id"]},
    )
    assert hidden.status_code == 404
