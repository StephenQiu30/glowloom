from __future__ import annotations

from collections.abc import AsyncIterator

import pytest
from httpx import ASGITransport, AsyncClient

from core.config import ApplicationSettings
from db.pool import DatabasePool
from main import create_app
from tests.integration.story_development.support import storyboard_draft


async def api_client(database_url: str) -> AsyncIterator[AsyncClient]:
    app = create_app(
        ApplicationSettings.model_validate(
            {"DATABASE_URL": database_url, "environment": "test"}
        )
    )
    async with app.router.lifespan_context(app), AsyncClient(
        transport=ASGITransport(app=app), base_url="http://test"
    ) as client:
        yield client


@pytest.mark.asyncio
async def test_script_http_versions_generation_etags_and_history(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        episode_id, generated = await storyboard_draft(database, "api:script:00001")
    finally:
        await database.close()
    script_id = generated.storyboard.content.script_version_id

    async for client in api_client(migrated_database_url):
        current = await client.get(f"/v1/episodes/{episode_id}/script")
        restored = await client.get(f"/v1/script-versions/{script_id}")
        history = await client.get(f"/v1/episodes/{episode_id}/script-versions")
        generated_task = await client.post(
            f"/v1/episodes/{episode_id}/script-generations",
            headers={"Idempotency-Key": "api:script:generate2"},
        )
        derived = await client.post(
            f"/v1/script-versions/{script_id}/drafts",
            headers={"Idempotency-Key": "api:script:derive01"},
        )
        derived_body = derived.json()
        derived_body["content"]["title"] = "API 人工剧本"
        saved = await client.put(
            f"/v1/script-versions/{derived_body['id']}",
            headers={"If-Match": derived.headers["etag"]},
            json={"content": derived_body["content"]},
        )
        assert saved.status_code == 200, saved.text
        confirmed = await client.post(
            f"/v1/script-versions/{derived_body['id']}:confirm",
            headers={"If-Match": saved.headers["etag"]},
        )
        stale = await client.put(
            f"/v1/script-versions/{derived_body['id']}",
            headers={"If-Match": saved.headers["etag"]},
            json={"content": derived_body["content"]},
        )

    assert current.status_code == restored.status_code == 200
    assert current.json()["id"] == str(script_id)
    assert restored.headers["etag"] == '"2"'
    assert history.json()["items"][0]["id"] == str(script_id)
    assert generated_task.status_code == 202
    assert generated_task.json()["status_url"].startswith("/v1/tasks/")
    assert derived.status_code == 201 and derived.headers["etag"] == '"1"'
    assert saved.status_code == 200 and saved.json()["content"]["title"] == "API 人工剧本"
    assert confirmed.json()["status"] == "confirmed"
    assert stale.status_code == 412 and stale.json()["code"] == "VERSION_CONFLICT"


@pytest.mark.asyncio
async def test_storyboard_asset_http_joint_confirmation_and_derivation(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        episode_id, generated = await storyboard_draft(database, "api:board:00001")
    finally:
        await database.close()
    board_id = generated.storyboard.id
    asset_id = generated.assets[0].id

    async for client in api_client(migrated_database_url):
        board = await client.get(f"/v1/shot-spec-versions/{board_id}")
        assets = await client.get(
            f"/v1/episodes/{episode_id}/creative-assets",
            params={"include_versions": "true"},
        )
        asset = await client.get(f"/v1/creative-asset-versions/{asset_id}")
        asset_body = asset.json()
        asset_body["content"]["description"] = "API 资产设定"
        saved_asset = await client.put(
            f"/v1/creative-asset-versions/{asset_id}",
            headers={"If-Match": asset.headers["etag"]},
            json={"content": asset_body["content"]},
        )
        saved_board = await client.put(
            f"/v1/shot-spec-versions/{board_id}",
            headers={"If-Match": board.headers["etag"]},
            json={"content": board.json()["content"]},
        )
        assert saved_board.status_code == 200, saved_board.text
        confirmed = await client.post(
            f"/v1/shot-spec-versions/{board_id}:confirm",
            headers={"If-Match": saved_board.headers["etag"]},
        )
        current = await client.get(f"/v1/episodes/{episode_id}/storyboard")
        derived = await client.post(
            f"/v1/shot-spec-versions/{board_id}/drafts",
            headers={"Idempotency-Key": "api:board:derive001"},
        )
        history = await client.get(f"/v1/episodes/{episode_id}/shot-spec-versions")
        generated_task = await client.post(
            f"/v1/episodes/{episode_id}/storyboard-generations",
            headers={"Idempotency-Key": "api:board:generate2"},
        )

    assert board.status_code == 200 and board.headers["etag"] == '"1"'
    assert len(assets.json()["items"]) == 3
    assert saved_asset.json()["content"]["description"] == "API 资产设定"
    assert confirmed.status_code == 200
    assert all(item["status"] == "confirmed" for item in confirmed.json()["assets"])
    assert current.json()["id"] == str(board_id)
    assert derived.status_code == 201 and len(derived.json()["assets"]) == 3
    assert len(history.json()["items"]) == 2
    assert generated_task.status_code == 202
