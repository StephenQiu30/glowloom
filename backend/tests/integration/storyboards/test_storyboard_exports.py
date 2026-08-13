from typing import Any
from uuid import UUID

import httpx
import pytest
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.modules.assets.models import Asset
from tests.integration.storyboards.test_storyboards_api import (
    create_episode_with_confirmed_structure,
    create_ready_location_asset,
    seed_ready_storyboard_shots,
)


async def create_ready_export_episode(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    email: str,
) -> tuple[dict[str, str], dict[str, Any], dict[str, UUID], dict[str, Any]]:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email=email,
    )
    asset_version, _consent = await create_ready_location_asset(
        client,
        session_factory,
        headers=headers,
        project_id=UUID(episode["project_id"]),
        refs=refs,
    )
    await seed_ready_storyboard_shots(
        session_factory,
        refs=refs,
        location_version_id=UUID(asset_version["id"]),
        count=1,
    )
    coverage = await client.get(
        f"/api/v1/episodes/{episode['id']}/coverage",
        headers=headers,
    )
    assert coverage.status_code == 200
    assert coverage.json()["data"]["status"] == "ready"
    readiness = await client.get(
        f"/api/v1/episodes/{episode['id']}/shot-readiness",
        headers=headers,
    )
    assert readiness.status_code == 200
    assert readiness.json()["data"]["summary"]["ready"] == 1
    return headers, episode, refs, asset_version


@pytest.mark.asyncio
async def test_export_preflight_fixes_ready_inputs_and_request_is_idempotent(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs, asset_version = await create_ready_export_episode(
        client,
        session_factory,
        email="storyboard-export-ready@example.com",
    )

    preflight = await client.post(
        f"/api/v1/episodes/{episode['id']}/storyboard-exports/preflight",
        headers=headers,
    )
    assert preflight.status_code == 200, preflight.text
    prepared = preflight.json()["data"]
    assert prepared["status"] == "ready"
    assert prepared["blockers"] == []
    assert prepared["script_version_id"] == str(refs["script_version_id"])
    assert prepared["coverage_evaluation_hash"]
    assert prepared["narrative_unit_version_ids"]
    assert prepared["shot_spec_version_ids"]
    assert prepared["asset_version_ids"] == [asset_version["id"]]
    assert len(prepared["input_hash"]) == 64

    payload = {
        "expected_input_hash": prepared["input_hash"],
        "idempotency_key": "storyboard-export-ready-001",
    }
    created = await client.post(
        f"/api/v1/episodes/{episode['id']}/storyboard-exports",
        headers=headers,
        json=payload,
    )
    assert created.status_code == 202, created.text
    export = created.json()["data"]
    assert export["status"] == "queued"
    assert export["input_hash"] == prepared["input_hash"]
    assert export["manifest"] is None
    assert export["task_id"]

    replay = await client.post(
        f"/api/v1/episodes/{episode['id']}/storyboard-exports",
        headers=headers,
        json=payload,
    )
    assert replay.status_code == 202
    assert replay.json()["data"]["id"] == export["id"]

    conflict = await client.post(
        f"/api/v1/episodes/{episode['id']}/storyboard-exports",
        headers=headers,
        json={
            "expected_input_hash": "0" * 64,
            "idempotency_key": payload["idempotency_key"],
        },
    )
    assert conflict.status_code == 409
    assert conflict.json()["error"]["details"] == {
        "reason": "idempotency_key_reused"
    }


@pytest.mark.asyncio
async def test_export_preflight_blocks_uncovered_and_disabled_asset(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    blocked_headers, blocked_episode, _blocked_refs = (
        await create_episode_with_confirmed_structure(
            client,
            session_factory,
            email="storyboard-export-uncovered@example.com",
        )
    )
    uncovered = await client.post(
        f"/api/v1/episodes/{blocked_episode['id']}/storyboard-exports/preflight",
        headers=blocked_headers,
    )
    assert uncovered.status_code == 200
    uncovered_data = uncovered.json()["data"]
    assert uncovered_data["status"] == "blocked"
    assert uncovered_data["input_hash"] is None
    assert "COVERAGE_UNACCOUNTED" in {
        blocker["code"] for blocker in uncovered_data["blockers"]
    }

    headers, episode, _refs, asset_version = await create_ready_export_episode(
        client,
        session_factory,
        email="storyboard-export-disabled@example.com",
    )
    async with session_factory() as session, session.begin():
        asset = await session.get(Asset, UUID(asset_version["asset_id"]), with_for_update=True)
        assert asset is not None
        asset.availability = "disabled"
        asset.revision += 1

    disabled = await client.post(
        f"/api/v1/episodes/{episode['id']}/storyboard-exports/preflight",
        headers=headers,
    )
    assert disabled.status_code == 200
    disabled_data = disabled.json()["data"]
    assert disabled_data["status"] == "blocked"
    assert disabled_data["input_hash"] is None
    assert "ASSET_DISABLED" in {
        blocker["code"] for blocker in disabled_data["blockers"]
    }


@pytest.mark.asyncio
async def test_export_request_rejects_stale_preflight_without_writing_history(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, _refs, asset_version = await create_ready_export_episode(
        client,
        session_factory,
        email="storyboard-export-stale@example.com",
    )
    preflight = await client.post(
        f"/api/v1/episodes/{episode['id']}/storyboard-exports/preflight",
        headers=headers,
    )
    assert preflight.status_code == 200
    input_hash = preflight.json()["data"]["input_hash"]

    async with session_factory() as session, session.begin():
        asset = await session.get(Asset, UUID(asset_version["asset_id"]), with_for_update=True)
        assert asset is not None
        asset.availability = "disabled"
        asset.revision += 1

    rejected = await client.post(
        f"/api/v1/episodes/{episode['id']}/storyboard-exports",
        headers=headers,
        json={
            "expected_input_hash": input_hash,
            "idempotency_key": "storyboard-export-stale-001",
        },
    )
    assert rejected.status_code == 409
    assert rejected.json()["error"]["details"]["reason"] == "export_input_changed"

    history = await client.get(
        f"/api/v1/episodes/{episode['id']}/storyboard-exports",
        headers=headers,
    )
    assert history.status_code == 200
    assert history.json()["data"]["items"] == []
