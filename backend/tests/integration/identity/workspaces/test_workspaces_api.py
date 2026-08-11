import httpx
import pytest

from tests.support.identity_builders import register_identity_response


@pytest.mark.asyncio
async def test_profile_and_workspace_lifecycle_are_versioned_and_isolated(
    client: httpx.AsyncClient,
) -> None:
    registered = await register_identity_response(client)
    token = registered.json()["data"]["access_token"]
    headers = {"authorization": f"Bearer {token}"}

    rejected = await client.patch(
        "/api/v1/me",
        headers=headers,
        json={"email": "takeover@example.com"},
    )
    assert rejected.status_code == 422

    updated_profile = await client.patch(
        "/api/v1/me",
        headers=headers,
        json={"display_name": "新名称", "avatar_url": "https://example.com/avatar.png"},
    )
    assert updated_profile.status_code == 200
    assert updated_profile.json()["data"]["user"]["display_name"] == "新名称"
    profile_audit = await client.get(
        "/api/v1/audit-events",
        headers=headers,
        params={
            "workspace_id": registered.json()["data"]["workspace"]["id"],
            "action": "identity.profile_updated",
        },
    )
    assert profile_audit.status_code == 200
    assert profile_audit.json()["data"]["total"] == 1
    assert profile_audit.json()["data"]["items"][0]["metadata"] == {
        "changed_fields": ["display_name", "avatar_url"]
    }
    assert "新名称" not in str(profile_audit.json()["data"])
    assert "avatar.png" not in str(profile_audit.json()["data"])

    created = await client.post(
        "/api/v1/workspaces",
        headers=headers,
        json={"name": "第二工作空间"},
    )
    assert created.status_code == 201
    workspace = created.json()["data"]
    workspace_id = workspace["id"]
    assert workspace["revision"] == 1

    renamed = await client.patch(
        f"/api/v1/workspaces/{workspace_id}",
        headers=headers,
        json={"name": "正式空间", "expected_revision": 1},
    )
    assert renamed.status_code == 200
    assert renamed.json()["data"]["revision"] == 2

    stale = await client.patch(
        f"/api/v1/workspaces/{workspace_id}",
        headers=headers,
        json={"name": "覆盖空间", "expected_revision": 1},
    )
    assert stale.status_code == 409
    assert stale.json()["error"]["code"] == "version_conflict"

    archived = await client.post(
        f"/api/v1/workspaces/{workspace_id}/archive",
        headers=headers,
        json={"expected_revision": 2},
    )
    assert archived.status_code == 200
    assert archived.json()["data"]["status"] == "archived"
    assert archived.json()["data"]["revision"] == 3

    restored = await client.post(
        f"/api/v1/workspaces/{workspace_id}/restore",
        headers=headers,
        json={"expected_revision": 3},
    )
    assert restored.status_code == 200
    assert restored.json()["data"]["status"] == "active"
    assert restored.json()["data"]["revision"] == 4

    audit = await client.get(
        "/api/v1/audit-events",
        headers=headers,
        params={
            "workspace_id": workspace_id,
            "target_type": "workspace",
            "target_id": workspace_id,
        },
    )
    assert audit.status_code == 200
    assert [item["action"] for item in audit.json()["data"]["items"]] == [
        "workspace.restored",
        "workspace.archived",
        "workspace.updated",
        "workspace.created",
    ]
    assert [item["metadata"]["revision"] for item in audit.json()["data"]["items"]] == [
        4,
        3,
        2,
        1,
    ]
    assert audit.json()["data"]["items"][2]["metadata"]["changed_fields"] == [
        "name"
    ]
    assert "第二工作空间" not in str(audit.json()["data"])
    assert "正式空间" not in str(audit.json()["data"])

    listed = await client.get("/api/v1/workspaces", headers=headers)
    assert listed.status_code == 200
    assert len(listed.json()["data"]) == 2

    other_registration = await register_identity_response(
        client,
        email="other@example.com",
        password="another-secure-password",
        display_name="其他用户",
    )
    other_token = other_registration.json()["data"]["access_token"]
    hidden = await client.get(
        f"/api/v1/workspaces/{workspace_id}",
        headers={"authorization": f"Bearer {other_token}"},
    )
    assert hidden.status_code == 404
