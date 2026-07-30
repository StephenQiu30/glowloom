import asyncio
from typing import Any

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.modules.messaging.models import OutboxEvent
from app.modules.projects.models import Episode
from app.modules.scripts.models import ScriptSource, ScriptVersion


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
