from __future__ import annotations

from httpx import ASGITransport, AsyncClient

import pytest

from core.config import ApplicationSettings
from db.pool import DatabasePool
from main import create_app
from services.storyboards import ConfirmStoryboardCommand, ConfirmStoryboardHandler
from tests.integration.story_development.support import storyboard_draft


@pytest.mark.asyncio
async def test_generate_media_http_accepts_one_slot_and_replays_task(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        episode_id, generated = await storyboard_draft(database, "api:media:image")
        confirmed = await ConfirmStoryboardHandler(database).execute(
            ConfirmStoryboardCommand(
                generated.storyboard.id, generated.storyboard.resource_version
            )
        )
    finally:
        await database.close()
    shot = confirmed.storyboard.content.shots[0]
    body = {
        "usage_type": "shot_image",
        "usage_id": str(shot.shot_id),
        "input_version_id": str(confirmed.storyboard.id),
    }
    app = create_app(
        ApplicationSettings.model_validate(
            {"DATABASE_URL": migrated_database_url, "environment": "test"}
        )
    )

    async with app.router.lifespan_context(app), AsyncClient(
        transport=ASGITransport(app=app), base_url="http://test"
    ) as client:
        first = await client.post(
            f"/v1/episodes/{episode_id}/media-generations",
            headers={"Idempotency-Key": "api:media:image:001"},
            json=body,
        )
        replay = await client.post(
            f"/v1/episodes/{episode_id}/media-generations",
            headers={"Idempotency-Key": "api:media:image:001"},
            json=body,
        )
        extra = await client.post(
            f"/v1/episodes/{episode_id}/media-generations",
            headers={"Idempotency-Key": "api:media:image:002"},
            json={**body, "prompt": "client must not control the frozen prompt"},
        )

    assert first.status_code == 202
    assert first.json() == replay.json()
    assert first.json()["status_url"] == f"/v1/tasks/{first.json()['task_id']}"
    assert extra.status_code == 422
    assert extra.json()["code"] == "VALIDATION_ERROR"


@pytest.mark.asyncio
async def test_generate_media_http_rejects_visual_style_slot(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        episode_id, generated = await storyboard_draft(database, "api:media:style")
        confirmed = await ConfirmStoryboardHandler(database).execute(
            ConfirmStoryboardCommand(
                generated.storyboard.id, generated.storyboard.resource_version
            )
        )
    finally:
        await database.close()
    style = next(item for item in confirmed.assets if item.asset_type == "visual_style")
    app = create_app(
        ApplicationSettings.model_validate(
            {"DATABASE_URL": migrated_database_url, "environment": "test"}
        )
    )

    async with app.router.lifespan_context(app), AsyncClient(
        transport=ASGITransport(app=app), base_url="http://test"
    ) as client:
        response = await client.post(
            f"/v1/episodes/{episode_id}/media-generations",
            headers={"Idempotency-Key": "api:media:style:001"},
            json={
                "usage_type": "asset_image",
                "usage_id": str(style.asset_id),
                "input_version_id": str(style.id),
            },
        )

    assert response.status_code == 422
    assert response.json()["code"] == "MEDIA_USAGE_UNSUPPORTED"


def test_generate_media_is_in_the_single_openapi_contract() -> None:
    schema = create_app().openapi()
    operation = schema["paths"]["/v1/episodes/{episode_id}/media-generations"]["post"]

    assert operation["operationId"] == "generateMedia"
    assert operation["responses"]["202"]
    assert operation["requestBody"]["required"] is True
