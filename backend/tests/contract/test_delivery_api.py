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
from services.render_delivery import StartRenderDeliveryHandler
from services.render_submission import RenderEpisodeCommand, RenderEpisodeCoordinator
from tests.integration.delivery.render_fixture import complete_ready_delivery
from tests.integration.delivery.test_render_submission import recipe
from workers.provider_execution import FaultInjector


@pytest.mark.asyncio
async def test_delivery_query_hides_locations_and_authorizes_ready_exact_set(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=12)
    await database.start()
    try:
        episode_id, delivery_id, transport = await complete_ready_delivery(
            database, "delivery-contract"
        )
    finally:
        await database.close()
    app = create_app(
        ApplicationSettings.model_validate(
            {"DATABASE_URL": migrated_database_url, "environment": "test"}
        )
    )
    async with app.router.lifespan_context(app):
        app.state.runtime = replace(
            app.state.runtime,
            object_store=MinioObjectStore(transport, bucket="lanverse"),
        )
        async with AsyncClient(
            transport=ASGITransport(app=app), base_url="http://test"
        ) as client:
            listed = await client.get(f"/v1/episodes/{episode_id}/deliveries")
            detail = await client.get(f"/v1/deliveries/{delivery_id}")
            authorized = await client.post(
                f"/v1/deliveries/{delivery_id}/download-authorizations",
                json={
                    "episode_id": str(episode_id),
                    "artifact_types": ["mp4", "srt", "manifest"],
                },
            )
            denied = await client.post(
                f"/v1/deliveries/{delivery_id}/download-authorizations",
                json={"episode_id": str(uuid4()), "artifact_types": ["mp4"]},
            )

    assert listed.status_code == 200
    assert [item["id"] for item in listed.json()["items"]] == [str(delivery_id)]
    assert detail.status_code == 200
    serialized = json.dumps(detail.json(), sort_keys=True)
    assert "bucket" not in serialized and "object_key" not in serialized
    assert "http://" not in serialized and "https://" not in serialized
    lineage = detail.json()["lineage"]
    assert lineage["source_revision"]["id"]
    assert lineage["script_version"]["id"]
    assert lineage["creative_asset_versions"]
    assert lineage["shot_spec_version"]["id"]
    assert lineage["subtitle_version"]["id"]
    assert lineage["render_snapshot"]["id"]
    assert lineage["render_task"]["id"]
    assert lineage["render_attempts"]
    assert all(item["provider_id"] and item["model_id"] for item in lineage["input_media"])
    assert {item["artifact_type"] for item in lineage["delivery_media"]} == {
        "mp4",
        "srt",
        "manifest",
    }

    assert authorized.status_code == 200
    assert {item["artifact_type"] for item in authorized.json()["items"]} == {
        "mp4",
        "srt",
        "manifest",
    }
    assert all(item["expires_in_seconds"] == 900 for item in authorized.json()["items"])
    assert all("X-Amz-Expires=900" in item["url"] for item in authorized.json()["items"])
    assert denied.status_code == 404
    assert denied.json()["code"] == "DELIVERY_NOT_FOUND"


@pytest.mark.asyncio
async def test_non_ready_delivery_has_no_download_authorization(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=12)
    await database.start()
    try:
        episode_id, ready_id, transport = await complete_ready_delivery(database, "non-ready")
        del ready_id
        accepted = await RenderEpisodeCoordinator(
            database,
            recipe=recipe(),
            release_version="test-release",
            fault=FaultInjector(),
        ).execute(RenderEpisodeCommand(episode_id, "delivery-api:still-rendering"))
        rendering = await StartRenderDeliveryHandler(database).execute(accepted.task_id)
    finally:
        await database.close()
    app = create_app(
        ApplicationSettings.model_validate(
            {"DATABASE_URL": migrated_database_url, "environment": "test"}
        )
    )
    async with app.router.lifespan_context(app):
        app.state.runtime = replace(
            app.state.runtime,
            object_store=MinioObjectStore(transport, bucket="lanverse"),
        )
        async with AsyncClient(
            transport=ASGITransport(app=app), base_url="http://test"
        ) as client:
            response = await client.post(
                f"/v1/deliveries/{rendering.id}/download-authorizations",
                json={"episode_id": str(episode_id), "artifact_types": ["mp4"]},
            )
    assert response.status_code == 404
    assert response.json()["code"] == "DELIVERY_NOT_FOUND"


def test_delivery_operations_are_in_the_single_openapi_contract() -> None:
    paths = create_app().openapi()["paths"]
    assert paths["/v1/episodes/{episode_id}/deliveries"]["get"]["operationId"] == (
        "listDeliveries"
    )
    assert paths["/v1/deliveries/{delivery_id}"]["get"]["operationId"] == "getDelivery"
    authorization = paths["/v1/deliveries/{delivery_id}/download-authorizations"]["post"]
    assert authorization["operationId"] == "authorizeDownload"
    assert authorization["requestBody"]["required"] is True
