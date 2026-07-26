from __future__ import annotations

import pytest
from httpx import ASGITransport, AsyncClient

from core.config import ApplicationSettings
from main import create_app


def settings_for(database_url: str) -> ApplicationSettings:
    return ApplicationSettings.model_validate(
        {"DATABASE_URL": database_url, "environment": "test"}
    )


@pytest.mark.asyncio
async def test_new_api_instance_recovers_project_current_source_and_history(
    migrated_database_url: str,
) -> None:
    first_app = create_app(settings_for(migrated_database_url))
    async with first_app.router.lifespan_context(first_app), AsyncClient(
        transport=ASGITransport(app=first_app), base_url="http://test"
    ) as client:
        created = await client.post(
            "/v1/projects",
            headers={"Idempotency-Key": "restart:project:01"},
            json={"title": "重启恢复"},
        )
        project_id = created.json()["project"]["id"]
        episode_id = created.json()["episode"]["id"]
        source = await client.post(
            f"/v1/episodes/{episode_id}/sources",
            headers={"Idempotency-Key": "restart:source:001"},
            json={
                "content": "汉字恢复" + "a" * 296,
                "rights_basis": "original",
                "parent_id": None,
            },
        )
        source_id = source.json()["id"]
        confirmed = await client.post(
            f"/v1/source-revisions/{source_id}:confirm",
            headers={"If-Match": source.headers["etag"]},
        )

    second_app = create_app(settings_for(migrated_database_url))
    async with second_app.router.lifespan_context(second_app), AsyncClient(
        transport=ASGITransport(app=second_app), base_url="http://test"
    ) as client:
        restored_project = await client.get(f"/v1/projects/{project_id}")
        restored_episode = await client.get(f"/v1/episodes/{episode_id}")
        restored_history = await client.get(
            f"/v1/episodes/{episode_id}/source-revisions"
        )

    assert restored_project.status_code == 200
    assert restored_project.json()["episode"]["current_source_revision_id"] == source_id
    assert restored_episode.json()["current_source_revision_id"] == source_id
    assert restored_history.json()["items"] == [confirmed.json()]
