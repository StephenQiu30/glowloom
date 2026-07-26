from __future__ import annotations

import pytest
from httpx import ASGITransport, AsyncClient

from core.config import ApplicationSettings
from db.pool import DatabasePool
from main import create_app
from tests.integration.delivery.support import story_with_tts_adoptions


@pytest.mark.asyncio
async def test_subtitle_http_create_edit_confirm_derive_and_history(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=4)
    await database.start()
    try:
        episode_id, _, _ = await story_with_tts_adoptions(database, "api")
    finally:
        await database.close()
    app = create_app(
        ApplicationSettings.model_validate(
            {"DATABASE_URL": migrated_database_url, "environment": "test"}
        )
    )
    async with (
        app.router.lifespan_context(app),
        AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client,
    ):
        created = await client.post(
            f"/v1/episodes/{episode_id}/subtitle-versions",
            headers={"Idempotency-Key": "subtitle:api:create01"},
        )
        assert created.status_code == 201, created.text
        body = created.json()
        first = body["content"]["cues"][0]
        first["text"] = "API 校对字幕"
        first["start_ticks"] += 3750
        first["end_ticks"] += 3750
        saved = await client.put(
            f"/v1/subtitle-versions/{body['id']}",
            headers={"If-Match": created.headers["etag"]},
            json={"content": body["content"]},
        )
        assert saved.status_code == 200, saved.text
        confirmed = await client.post(
            f"/v1/subtitle-versions/{body['id']}:confirm",
            headers={"If-Match": saved.headers["etag"]},
        )
        current = await client.get(f"/v1/episodes/{episode_id}/subtitles")
        restored = await client.get(f"/v1/subtitle-versions/{body['id']}")
        history = await client.get(f"/v1/episodes/{episode_id}/subtitle-versions")
        derived = await client.post(
            f"/v1/subtitle-versions/{body['id']}/drafts",
            headers={"Idempotency-Key": "subtitle:api:derive01"},
        )
        immutable = await client.put(
            f"/v1/subtitle-versions/{body['id']}",
            headers={"If-Match": confirmed.headers["etag"]},
            json={"content": body["content"]},
        )

    assert saved.status_code == 200 and saved.headers["etag"] == '"2"'
    assert saved.json()["content"]["cues"][0]["text"] == "API 校对字幕"
    assert confirmed.status_code == 200 and confirmed.json()["status"] == "confirmed"
    assert current.json() == restored.json()
    assert len(history.json()["items"]) == 1
    assert derived.status_code == 201 and derived.json()["parent_id"] == body["id"]
    assert immutable.status_code == 409
    assert immutable.json()["code"] == "VERSION_IMMUTABLE"


def test_subtitle_operations_are_in_the_single_openapi_contract() -> None:
    paths = create_app().openapi()["paths"]
    expected = {
        ("/v1/episodes/{episode_id}/subtitle-versions", "post"): "createSubtitles",
        ("/v1/episodes/{episode_id}/subtitles", "get"): "getSubtitles",
        ("/v1/subtitle-versions/{version_id}", "put"): "saveSubtitles",
        ("/v1/subtitle-versions/{version_id}", "get"): "getSubtitleVersion",
        ("/v1/episodes/{episode_id}/subtitle-versions", "get"):
            "listSubtitleVersions",
        ("/v1/subtitle-versions/{version_id}/drafts", "post"):
            "deriveSubtitleDraft",
        ("/v1/subtitle-versions/{version_id}:confirm", "post"): "confirmSubtitles",
    }
    assert {
        key: paths[key[0]][key[1]]["operationId"] for key in expected
    } == expected
