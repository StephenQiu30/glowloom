from __future__ import annotations

from collections.abc import AsyncIterator

import pytest
from httpx import ASGITransport, AsyncClient

from lanverse.bootstrap.api import create_app
from lanverse.shared_kernel.config import ApplicationSettings


async def api_client(database_url: str) -> AsyncIterator[AsyncClient]:
    settings = ApplicationSettings.model_validate(
        {"DATABASE_URL": database_url, "environment": "test"}
    )
    app = create_app(settings)
    async with app.router.lifespan_context(app), AsyncClient(
        transport=ASGITransport(app=app), base_url="http://test"
    ) as client:
        yield client


def source_text(label: str = "甲") -> str:
    return "汉字来源" + label + "a" * 295


@pytest.mark.asyncio
async def test_project_and_source_http_round_trip_with_etags(
    migrated_database_url: str,
) -> None:
    async for client in api_client(migrated_database_url):
        created = await client.post(
            "/v1/projects",
            headers={"Idempotency-Key": "api:project:0001"},
            json={"title": "  API 短剧  "},
        )
        project_id = created.json()["project"]["id"]
        episode_id = created.json()["episode"]["id"]
        projects = await client.get("/v1/projects")
        project = await client.get(f"/v1/projects/{project_id}")
        episode = await client.get(f"/v1/episodes/{episode_id}")
        source = await client.post(
            f"/v1/episodes/{episode_id}/sources",
            headers={"Idempotency-Key": "api:source:00001"},
            json={"content": source_text(), "rights_basis": "original", "parent_id": None},
        )
        source_id = source.json()["id"]
        confirmed = await client.post(
            f"/v1/source-revisions/{source_id}:confirm",
            headers={"If-Match": source.headers["etag"]},
        )
        history = await client.get(f"/v1/episodes/{episode_id}/source-revisions")
        restored = await client.get(f"/v1/source-revisions/{source_id}")

    assert created.status_code == 201
    assert created.json()["project"]["title"] == "API 短剧"
    assert created.json()["project"]["production_spec"]["width"] == 720
    assert projects.json()["items"][0]["project"]["id"] == project_id
    assert project.json() == created.json()
    assert episode.json()["project_id"] == project_id
    assert source.status_code == 201
    assert source.headers["etag"] == '"1"'
    assert confirmed.status_code == 200
    assert confirmed.headers["etag"] == '"2"'
    assert confirmed.json()["status"] == "confirmed"
    assert history.json()["items"][0]["id"] == source_id
    assert restored.json() == confirmed.json()
    assert restored.headers["etag"] == '"2"'


@pytest.mark.asyncio
async def test_project_catalog_maps_validation_conflict_and_not_found_to_problems(
    migrated_database_url: str,
) -> None:
    async for client in api_client(migrated_database_url):
        missing_header = await client.post("/v1/projects", json={"title": "项目"})
        invalid_title = await client.post(
            "/v1/projects",
            headers={"Idempotency-Key": "api:project:bad1"},
            json={"title": ""},
        )
        created = await client.post(
            "/v1/projects",
            headers={"Idempotency-Key": "api:project:reuse"},
            json={"title": "项目甲"},
        )
        reused = await client.post(
            "/v1/projects",
            headers={"Idempotency-Key": "api:project:reuse"},
            json={"title": "项目乙"},
        )
        missing = await client.get("/v1/projects/00000000-0000-0000-0000-000000000000")
        episode_id = created.json()["episode"]["id"]
        invalid_source = await client.post(
            f"/v1/episodes/{episode_id}/sources",
            headers={"Idempotency-Key": "api:source:bad01"},
            json={"content": "汉" * 299, "rights_basis": "original", "parent_id": None},
        )

    for response in (missing_header, invalid_title, reused, missing, invalid_source):
        assert response.headers["content-type"].startswith("application/problem+json")
        assert response.json()["request_id"]
        assert "traceback" not in response.text.lower()
    assert missing_header.status_code == 422
    assert invalid_title.json()["code"] == "PROJECT_TITLE_LENGTH_OUT_OF_RANGE"
    assert reused.status_code == 409
    assert reused.json()["code"] == "IDEMPOTENCY_KEY_REUSED"
    assert missing.status_code == 404
    assert invalid_source.json()["metadata"]["actual"] == 299


@pytest.mark.asyncio
async def test_confirm_source_requires_a_current_strong_etag(
    migrated_database_url: str,
) -> None:
    async for client in api_client(migrated_database_url):
        created = await client.post(
            "/v1/projects",
            headers={"Idempotency-Key": "api:etag:project"},
            json={"title": "版本项目"},
        )
        episode_id = created.json()["episode"]["id"]
        source = await client.post(
            f"/v1/episodes/{episode_id}/sources",
            headers={"Idempotency-Key": "api:etag:source1"},
            json={"content": source_text(), "rights_basis": "original", "parent_id": None},
        )
        source_id = source.json()["id"]
        missing = await client.post(f"/v1/source-revisions/{source_id}:confirm")
        confirmed = await client.post(
            f"/v1/source-revisions/{source_id}:confirm", headers={"If-Match": '"1"'}
        )
        stale = await client.post(
            f"/v1/source-revisions/{source_id}:confirm", headers={"If-Match": '"1"'}
        )

    assert missing.status_code == 422
    assert confirmed.status_code == 200
    assert stale.status_code == 412
    assert stale.json()["code"] == "VERSION_CONFLICT"


def test_project_catalog_openapi_operations_and_input_limits_are_explicit() -> None:
    schema = create_app().openapi()
    operations = {
        operation["operationId"]
        for path in schema["paths"].values()
        for operation in path.values()
        if isinstance(operation, dict) and "operationId" in operation
    }

    assert operations == {
        "createProject",
        "listProjects",
        "getProject",
        "getEpisode",
        "createSourceRevision",
        "confirmSource",
        "listSourceRevisions",
        "getSourceRevision",
    }
    assert "post" not in schema["paths"].get("/v1/episodes", {})
    project = schema["components"]["schemas"]["CreateProjectRequest"]
    source = schema["components"]["schemas"]["CreateSourceRevisionRequest"]
    assert project["properties"]["title"]["maxLength"] == 120
    assert source["properties"]["content"]["minLength"] == 300
    assert source["properties"]["content"]["maxLength"] == 3000
