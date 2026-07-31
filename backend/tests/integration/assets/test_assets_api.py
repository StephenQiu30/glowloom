import asyncio
from datetime import UTC, datetime, timedelta
from typing import Any, cast
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app.modules.assets.models import Asset, AssetMediaReference, AssetVersion
from tests.support.identity_builders import register_identity_response
from tests.support.media_builders import seed_ready_media_version
from tests.support.project_builders import project_payload


async def _identity_project(
    client: httpx.AsyncClient,
    *,
    email: str,
) -> tuple[dict[str, str], dict[str, Any], dict[str, Any]]:
    registered = await register_identity_response(client, email=email)
    assert registered.status_code == 201
    identity = registered.json()["data"]
    headers = {"authorization": f"Bearer {identity['access_token']}"}
    created = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(identity["workspace"]["id"], "资产圣经验收项目"),
    )
    assert created.status_code == 201
    return headers, identity, created.json()["data"]


def _character_version_payload(
    media_version_id: UUID,
    *,
    expected_current_version_id: str | None,
    appearance: str = "黑发、冷峻轮廓",
) -> dict[str, object]:
    return {
        "spec": {
            "kind": "character",
            "identity": "林澈",
            "appearance": appearance,
            "age_impression": "28 岁",
            "temperament": ["克制", "果断"],
        },
        "prompt_description": "固定主角外观，保持跨镜头一致",
        "media_references": [
            {
                "media_version_id": str(media_version_id),
                "purpose": "portrait",
                "position": 1,
            }
        ],
        "source_type": "manual",
        "source_id": None,
        "expected_current_version_id": expected_current_version_id,
        "set_as_current": True,
    }


def _consent_payload(
    workspace_id: UUID,
    asset_version_id: UUID,
    proof_id: UUID,
) -> dict[str, object]:
    now = datetime.now(UTC)
    return {
        "workspace_id": str(workspace_id),
        "subject_identity": {
            "reference": "synthetic-character-lin-che",
            "kind": "fictional_adult",
        },
        "scope": {
            "type": "media_usage",
            "subject_type": "ASSET_VERSION",
            "subject_id": str(asset_version_id),
            "rights_holder_role": "synthetic_creator",
            "rights_types": ["copyright", "image"],
            "authorized_purposes": ["ai_short_drama_generation"],
            "channels": ["lanverse_preview"],
            "regions": ["CN"],
            "valid_from": (now - timedelta(days=1)).isoformat(),
            "valid_to": (now + timedelta(days=365)).isoformat(),
        },
        "proof_media_version_ids": [str(proof_id)],
        "reason": "合成角色资产授权验收",
        "idempotency_key": f"asset-consent-{asset_version_id}",
    }


@pytest.mark.asyncio
async def test_asset_identity_lifecycle_duplicate_hint_and_safe_delete(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, project = await _identity_project(
        client,
        email="asset-lifecycle@example.com",
    )
    endpoint = f"/api/v1/projects/{project['id']}/assets"
    payload = {
        "kind": "character",
        "name": "林澈",
        "aliases": ["阿澈"],
        "tags": ["主角"],
    }
    first = await client.post(endpoint, headers=headers, json=payload)
    assert first.status_code == 201
    asset = first.json()["data"]
    assert asset["kind"] == "character"
    assert asset["status"] == "active"
    assert asset["revision"] == 1
    assert asset["warnings"] == []

    duplicate = await client.post(endpoint, headers=headers, json=payload)
    assert duplicate.status_code == 201
    assert duplicate.json()["data"]["id"] != asset["id"]
    assert duplicate.json()["data"]["warnings"] == ["duplicate_name"]

    listed = await client.get(
        endpoint,
        headers=headers,
        params={"kind": "character", "query": "林澈"},
    )
    assert listed.status_code == 200
    assert listed.json()["data"]["total"] == 2

    changed_kind = await client.patch(
        f"/api/v1/assets/{asset['id']}",
        headers=headers,
        json={"expected_revision": 1, "kind": "voice"},
    )
    assert changed_kind.status_code == 422

    archived = await client.post(
        f"/api/v1/assets/{asset['id']}/archive",
        headers=headers,
        json={"expected_revision": 1},
    )
    assert archived.status_code == 200
    assert archived.json()["data"]["status"] == "archived"
    restored = await client.post(
        f"/api/v1/assets/{asset['id']}/restore",
        headers=headers,
        json={"expected_revision": 2},
    )
    assert restored.status_code == 200
    assert restored.json()["data"]["status"] == "active"

    preflight = await client.get(
        f"/api/v1/assets/{asset['id']}/delete-preflight",
        headers=headers,
    )
    assert preflight.status_code == 200
    assert preflight.json()["data"] == {"allowed": True, "blockers": []}
    deleted = await client.delete(
        f"/api/v1/assets/{asset['id']}",
        headers=headers,
        params={"expected_revision": 3},
    )
    assert deleted.status_code == 200
    assert deleted.json()["data"] == {"deleted": True}

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(Asset)) == 1


@pytest.mark.asyncio
async def test_asset_version_is_typed_immutable_concurrent_and_rights_gated(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, identity, project = await _identity_project(
        client,
        email="asset-version@example.com",
    )
    workspace_id = UUID(identity["workspace"]["id"])
    actor_id = UUID(identity["user"]["id"])
    portrait_id = await seed_ready_media_version(
        session_factory,
        workspace_id=workspace_id,
        actor_id=actor_id,
        kind="image",
        filename="lin-che.png",
        mime_type="image/png",
    )
    created_asset = await client.post(
        f"/api/v1/projects/{project['id']}/assets",
        headers=headers,
        json={
            "kind": "character",
            "name": "林澈",
            "aliases": [],
            "tags": ["主角"],
        },
    )
    assert created_asset.status_code == 201
    asset = created_asset.json()["data"]

    version_response = await client.post(
        f"/api/v1/assets/{asset['id']}/versions",
        headers=headers,
        json=_character_version_payload(
            portrait_id,
            expected_current_version_id=None,
        ),
    )
    assert version_response.status_code == 201
    result = version_response.json()["data"]
    version = result["version"]
    assert version["version_no"] == 1
    assert version["spec"]["kind"] == "character"
    assert result["asset"]["current_version_id"] == version["id"]
    assert result["readiness"]["status"] == "blocked"
    assert [item["code"] for item in result["readiness"]["blockers"]] == [
        "consent_missing"
    ]

    invalid_payload = _character_version_payload(
        portrait_id,
        expected_current_version_id=version["id"],
    )
    invalid_spec = dict(cast(dict[str, object], invalid_payload["spec"]))
    invalid_spec["provider_model"] = "forbidden"
    invalid_payload["spec"] = invalid_spec
    unknown_field = await client.post(
        f"/api/v1/assets/{asset['id']}/versions",
        headers=headers,
        json=invalid_payload,
    )
    assert unknown_field.status_code == 422

    consent = await client.post(
        "/api/v1/consents",
        headers=headers,
        json=_consent_payload(workspace_id, UUID(version["id"]), portrait_id),
    )
    assert consent.status_code == 201
    consent_data = consent.json()["data"]
    readiness = await client.get(
        f"/api/v1/asset-versions/{version['id']}/readiness",
        headers=headers,
        params={
            "purpose": "ai_short_drama_generation",
            "channel": "lanverse_preview",
            "region": "CN",
        },
    )
    assert readiness.status_code == 200
    assert readiness.json()["data"]["status"] == "ready"
    assert readiness.json()["data"]["dependency_snapshot"]["consent_ids"] == [
        consent_data["id"]
    ]

    endpoint = f"/api/v1/assets/{asset['id']}/versions"
    first, second = await asyncio.gather(
        client.post(
            endpoint,
            headers=headers,
            json=_character_version_payload(
                portrait_id,
                expected_current_version_id=version["id"],
                appearance="黑发、雨夜造型",
            ),
        ),
        client.post(
            endpoint,
            headers=headers,
            json=_character_version_payload(
                portrait_id,
                expected_current_version_id=version["id"],
                appearance="黑发、室内造型",
            ),
        ),
    )
    assert sorted([first.status_code, second.status_code]) == [201, 409]
    versions = await client.get(endpoint, headers=headers)
    assert versions.status_code == 200
    assert versions.json()["data"]["total"] == 2

    revoked = await client.post(
        f"/api/v1/consents/{consent_data['id']}/revoke",
        headers=headers,
        json={"expected_revision": 1, "reason": "撤回资产授权"},
    )
    assert revoked.status_code == 200
    blocked = await client.get(
        f"/api/v1/asset-versions/{version['id']}/readiness",
        headers=headers,
        params={
            "purpose": "ai_short_drama_generation",
            "channel": "lanverse_preview",
            "region": "CN",
        },
    )
    assert blocked.status_code == 200
    assert blocked.json()["data"]["status"] == "blocked"
    assert blocked.json()["data"]["blockers"][0]["code"] == "consent_revoked"

    delete_preflight = await client.get(
        f"/api/v1/assets/{asset['id']}/delete-preflight",
        headers=headers,
    )
    assert delete_preflight.status_code == 200
    assert delete_preflight.json()["data"]["allowed"] is False
    assert delete_preflight.json()["data"]["blockers"][0]["code"] == "asset_has_versions"
    assert delete_preflight.json()["data"]["blockers"][0]["version_count"] == 2

    async with session_factory() as session:
        original = await session.scalar(
            select(AssetVersion).where(AssetVersion.id == UUID(version["id"]))
        )
        assert original is not None
        assert original.spec["appearance"] == "黑发、冷峻轮廓"
        assert (
            await session.scalar(
                select(func.count()).select_from(AssetMediaReference)
            )
            == 2
        )


@pytest.mark.asyncio
async def test_asset_commands_hide_cross_workspace_media_and_assets(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    first_headers, first_identity, first_project = await _identity_project(
        client,
        email="asset-workspace-a@example.com",
    )
    second_headers, second_identity, _ = await _identity_project(
        client,
        email="asset-workspace-b@example.com",
    )
    foreign_media_id = await seed_ready_media_version(
        session_factory,
        workspace_id=UUID(second_identity["workspace"]["id"]),
        actor_id=UUID(second_identity["user"]["id"]),
        kind="image",
        filename="foreign.png",
        mime_type="image/png",
    )
    created = await client.post(
        f"/api/v1/projects/{first_project['id']}/assets",
        headers=first_headers,
        json={
            "kind": "character",
            "name": "跨空间角色",
            "aliases": [],
            "tags": [],
        },
    )
    assert created.status_code == 201
    asset = created.json()["data"]

    foreign_reference = await client.post(
        f"/api/v1/assets/{asset['id']}/versions",
        headers=first_headers,
        json=_character_version_payload(
            foreign_media_id,
            expected_current_version_id=None,
        ),
    )
    assert foreign_reference.status_code == 404
    assert foreign_reference.json()["error"]["code"] == "not_found"

    hidden = await client.get(
        f"/api/v1/assets/{asset['id']}",
        headers=second_headers,
    )
    assert hidden.status_code == 404
    assert hidden.json()["error"]["code"] == "not_found"

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(AssetVersion)) == 0
        assert UUID(first_identity["workspace"]["id"]) != UUID(
            second_identity["workspace"]["id"]
        )
