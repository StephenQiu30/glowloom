from __future__ import annotations

import json
from dataclasses import replace
from uuid import uuid4

import pytest
from httpx import ASGITransport, AsyncClient

from core.config import ApplicationSettings
from db.pool import DatabasePool
from integrations.object_storage import MinioObjectStore
from main import create_app
from tests.integration.media_library.support import (
    MemoryTransport,
    accepted_tts_task,
    media_job_handler,
    run_media_job,
)


@pytest.mark.asyncio
async def test_candidate_list_hides_object_location_and_preview_is_episode_scoped(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        task_id, _, _ = await accepted_tts_task(database, "candidate-api")
        transport = MemoryTransport()
        await run_media_job(
            database,
            task_id,
            media_job_handler(database, transport=transport),
        )
        async with database.transaction() as connection:
            row = await connection.fetchrow(
                """
                SELECT episode_id,usage_type,usage_id,input_version_id,input_hash,
                       media_version_id
                FROM generation_candidates WHERE task_id=$1
                """,
                task_id,
            )
    finally:
        await database.close()
    settings = ApplicationSettings.model_validate(
        {"DATABASE_URL": migrated_database_url, "environment": "test"}
    )
    app = create_app(settings)

    async with app.router.lifespan_context(app):
        app.state.runtime = replace(
            app.state.runtime,
            object_store=MinioObjectStore(transport, bucket="lanverse"),
        )
        async with AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client:
            listed = await client.get(
                f"/v1/episodes/{row['episode_id']}/candidates",
                params={
                    "usage_type": row["usage_type"],
                    "usage_id": str(row["usage_id"]),
                    "input_version_id": str(row["input_version_id"]),
                    "input_hash": row["input_hash"],
                },
            )
            authorized = await client.post(
                f"/v1/media-versions/{row['media_version_id']}/preview-authorizations",
                json={"episode_id": str(row["episode_id"])},
            )
            denied = await client.post(
                f"/v1/media-versions/{row['media_version_id']}/preview-authorizations",
                json={"episode_id": str(uuid4())},
            )

    assert listed.status_code == 200
    assert len(listed.json()["items"]) == 1
    serialized = json.dumps(listed.json(), sort_keys=True)
    assert "bucket" not in serialized
    assert "object_key" not in serialized
    assert "http" not in serialized
    candidate = listed.json()["items"][0]
    assert candidate["media_version_id"] == str(row["media_version_id"])
    assert candidate["technical_summary"]["sample_rate"] == 48000
    assert candidate["active_adoption_id"] is None

    assert authorized.status_code == 200
    assert authorized.json()["media_version_id"] == str(row["media_version_id"])
    assert authorized.json()["expires_in_seconds"] == 900
    assert "X-Amz-Expires=900" in authorized.json()["url"]
    assert transport.authorizations[0][2] == 900
    assert denied.status_code == 404
    assert denied.json()["code"] == "CANDIDATE_NOT_FOUND"


@pytest.mark.asyncio
async def test_candidate_adoption_is_idempotent_and_visible_in_the_slot(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=3)
    await database.start()
    try:
        task_id, _, _ = await accepted_tts_task(database, "candidate-adoption-api")
        await run_media_job(database, task_id, media_job_handler(database))
        async with database.transaction() as connection:
            candidate = await connection.fetchrow(
                "SELECT * FROM generation_candidates WHERE task_id=$1", task_id
            )
        assert candidate is not None
    finally:
        await database.close()
    app = create_app(
        ApplicationSettings.model_validate(
            {"DATABASE_URL": migrated_database_url, "environment": "test"}
        )
    )
    body = {
        "usage_type": candidate["usage_type"],
        "usage_id": str(candidate["usage_id"]),
        "input_version_id": str(candidate["input_version_id"]),
        "input_hash": candidate["input_hash"],
        "candidate_id": str(candidate["id"]),
    }
    async with (
        app.router.lifespan_context(app),
        AsyncClient(transport=ASGITransport(app=app), base_url="http://test") as client,
    ):
        created = await client.post(
            "/v1/adoptions", json=body, headers={"Idempotency-Key": "adopt:api:0001"}
        )
        replay = await client.post(
            "/v1/adoptions", json=body, headers={"Idempotency-Key": "adopt:api:0001"}
        )
        reused = await client.post(
            "/v1/adoptions",
            json={**body, "candidate_id": str(uuid4())},
            headers={"Idempotency-Key": "adopt:api:0001"},
        )
        listed = await client.get(
            f"/v1/episodes/{candidate['episode_id']}/candidates",
            params={key: value for key, value in body.items() if key != "candidate_id"},
        )

    assert created.status_code == 201
    assert replay.status_code == 201
    assert replay.json() == created.json()
    assert created.json()["candidate_id"] == str(candidate["id"])
    assert created.json()["status"] == "active"
    assert reused.status_code == 409
    assert reused.json()["code"] == "IDEMPOTENCY_KEY_REUSED"
    assert listed.json()["items"][0]["active_adoption_id"] == created.json()["id"]


def test_candidate_operations_are_in_the_single_openapi_contract() -> None:
    schema = create_app().openapi()
    paths = schema["paths"]

    assert paths["/v1/episodes/{episode_id}/candidates"]["get"]["operationId"] == ("listCandidates")
    authorization = paths["/v1/media-versions/{media_version_id}/preview-authorizations"]["post"]
    assert authorization["operationId"] == "authorizeCandidatePreview"
    assert authorization["requestBody"]["required"] is True
    adoption = paths["/v1/adoptions"]["post"]
    assert adoption["operationId"] == "adoptCandidate"
    assert adoption["requestBody"]["required"] is True
