from decimal import Decimal
from uuid import UUID

import httpx
import pytest
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.modules.identity.models import Membership
from tests.support.project_builders import project_payload, register_project_owner


@pytest.mark.asyncio
async def test_project_crud_budget_and_lifecycle_use_explicit_revisions(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await register_project_owner(client)
    created = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(workspace_id),
    )
    assert created.status_code == 201
    project = created.json()["data"]
    project_id = project["id"]
    assert project["revision"] == 1
    assert project["budget_limit"] == "0.000000"
    assert project["currency"] == "CNY"

    budget_in_patch = await client.patch(
        f"/api/v1/projects/{project_id}",
        headers=headers,
        json={"name": "非法夹带", "budget_limit": "100", "expected_revision": 1},
    )
    assert budget_in_patch.status_code == 422

    updated = await client.patch(
        f"/api/v1/projects/{project_id}",
        headers=headers,
        json={
            "name": "第一季",
            "description": "修改后的简介",
            "aspect_ratio": "9:16",
            "language": "zh-CN",
            "visual_style": "水墨写实",
            "target_duration_ms": 120000,
            "expected_revision": 1,
        },
    )
    assert updated.status_code == 200
    assert updated.json()["data"]["revision"] == 2

    stale_budget = await client.post(
        f"/api/v1/projects/{project_id}/budget-limit",
        headers=headers,
        json={"amount": "99.990000", "currency": "CNY", "expected_revision": 1},
    )
    assert stale_budget.status_code == 409

    budget = await client.post(
        f"/api/v1/projects/{project_id}/budget-limit",
        headers=headers,
        json={"amount": "99.990000", "currency": "CNY", "expected_revision": 2},
    )
    assert budget.status_code == 200
    assert Decimal(budget.json()["data"]["budget_limit"]) == Decimal("99.990000")
    assert budget.json()["data"]["revision"] == 3

    archived = await client.post(
        f"/api/v1/projects/{project_id}/archive",
        headers=headers,
        json={"expected_revision": 3},
    )
    assert archived.status_code == 200
    assert archived.json()["data"]["status"] == "archived"

    blocked_update = await client.patch(
        f"/api/v1/projects/{project_id}",
        headers=headers,
        json={"name": "不应写入", "expected_revision": 4},
    )
    assert blocked_update.status_code == 409

    restored = await client.post(
        f"/api/v1/projects/{project_id}/restore",
        headers=headers,
        json={"expected_revision": 4},
    )
    assert restored.status_code == 200
    assert restored.json()["data"]["status"] == "active"


@pytest.mark.asyncio
async def test_project_lists_are_bounded_and_cross_workspace_is_hidden(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await register_project_owner(client)
    for name in ("Beta", "Alpha"):
        response = await client.post(
            "/api/v1/projects",
            headers=headers,
            json=project_payload(workspace_id, name),
        )
        assert response.status_code == 201
    listed = await client.get(
        "/api/v1/projects",
        headers=headers,
        params={"workspace_id": workspace_id, "sort": "name", "order": "asc", "limit": 1},
    )
    assert listed.status_code == 200
    assert listed.json()["data"]["total"] == 2
    assert [item["name"] for item in listed.json()["data"]["items"]] == ["Alpha"]

    project_id = listed.json()["data"]["items"][0]["id"]
    other_headers, _ = await register_project_owner(client, email="other-owner@example.com")
    hidden = await client.get(f"/api/v1/projects/{project_id}", headers=other_headers)
    assert hidden.status_code == 404


@pytest.mark.asyncio
async def test_project_permissions_and_workspace_archive_gate_existing_writes(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    owner_headers, workspace_id = await register_project_owner(client)
    created = await client.post(
        "/api/v1/projects",
        headers=owner_headers,
        json=project_payload(workspace_id),
    )
    project_id = created.json()["data"]["id"]

    actors: dict[str, tuple[UUID, str]] = {}
    for role in ("editor", "viewer"):
        registered = await client.post(
            "/api/v1/auth/register",
            json={
                "email": f"{role}@example.com",
                "password": f"a-secure-{role}-password",
                "display_name": role,
            },
        )
        actor = registered.json()["data"]
        actors[role] = UUID(actor["user"]["id"]), actor["access_token"]

    async with session_factory() as session:
        async with session.begin():
            session.add_all(
                Membership(
                    workspace_id=UUID(workspace_id),
                    user_id=actors[role][0],
                    role=role,
                )
                for role in ("editor", "viewer")
            )

    editor_headers = {"authorization": f"Bearer {actors['editor'][1]}"}
    viewer_headers = {"authorization": f"Bearer {actors['viewer'][1]}"}
    edited = await client.patch(
        f"/api/v1/projects/{project_id}",
        headers=editor_headers,
        json={"name": "编辑者可修改", "expected_revision": 1},
    )
    assert edited.status_code == 200
    assert (
        await client.post(
            f"/api/v1/projects/{project_id}/budget-limit",
            headers=editor_headers,
            json={"amount": "1", "currency": "CNY", "expected_revision": 2},
        )
    ).status_code == 403
    viewer_read = await client.get(f"/api/v1/projects/{project_id}", headers=viewer_headers)
    assert viewer_read.status_code == 200
    assert (
        await client.patch(
            f"/api/v1/projects/{project_id}",
            headers=viewer_headers,
            json={"name": "只读用户不可修改", "expected_revision": 2},
        )
    ).status_code == 403

    archived = await client.post(
        f"/api/v1/workspaces/{workspace_id}/archive",
        headers=owner_headers,
        json={"expected_revision": 1},
    )
    assert archived.status_code == 200
    assert (
        await client.post(
            "/api/v1/projects",
            headers=owner_headers,
            json=project_payload(workspace_id, "不应创建"),
        )
    ).status_code == 403
    assert (
        await client.patch(
            f"/api/v1/projects/{project_id}",
            headers=owner_headers,
            json={"name": "不应修改", "expected_revision": 2},
        )
    ).status_code == 403
    archived_read = await client.get(f"/api/v1/projects/{project_id}", headers=owner_headers)
    assert archived_read.status_code == 200

    restored = await client.post(
        f"/api/v1/workspaces/{workspace_id}/restore",
        headers=owner_headers,
        json={"expected_revision": 2},
    )
    assert restored.status_code == 200
    resumed = await client.patch(
        f"/api/v1/projects/{project_id}",
        headers=owner_headers,
        json={"name": "恢复后可修改", "expected_revision": 2},
    )
    assert resumed.status_code == 200


@pytest.mark.asyncio
async def test_empty_project_can_be_preflighted_and_deleted(
    client: httpx.AsyncClient,
) -> None:
    headers, workspace_id = await register_project_owner(client)
    created = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(workspace_id),
    )
    project_id = created.json()["data"]["id"]

    preflight = await client.post(
        f"/api/v1/projects/{project_id}/delete-preflight",
        headers=headers,
    )
    assert preflight.status_code == 200
    assert preflight.json()["data"] == {"allowed": True, "blockers": []}

    deleted = await client.delete(
        f"/api/v1/projects/{project_id}",
        headers=headers,
        params={"expected_revision": 1},
    )
    assert deleted.status_code == 200
    assert deleted.json()["data"]["deleted"] is True
    assert (await client.get(f"/api/v1/projects/{project_id}", headers=headers)).status_code == 404
