import asyncio
from datetime import UTC, datetime
from typing import Any
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.core.auth import decode_access_token
from app.core.config import Settings
from app.core.errors import ApiError, ErrorCode
from app.modules.governance.contracts import RightsUsage, SubjectReference, SubjectType
from app.modules.governance.models import Consent, ConsentProof, ConsentRevision
from app.modules.governance.service import check_rights
from app.modules.identity.models import Membership
from app.modules.media.models import MediaLocation, MediaObject, MediaVersion
from tests.support.identity_builders import register_identity_response
from tests.support.project_builders import project_payload


async def _identity(
    client: httpx.AsyncClient,
    *,
    email: str = "governance-owner@example.com",
) -> tuple[dict[str, str], dict[str, Any]]:
    response = await register_identity_response(client, email=email)
    assert response.status_code == 201
    data = response.json()["data"]
    return {"authorization": f"Bearer {data['access_token']}"}, data


async def _seed_accessible_media(
    session_factory: async_sessionmaker[AsyncSession],
    *,
    workspace_id: UUID,
    actor_id: UUID,
    filename: str,
) -> UUID:
    object_id = uuid7()
    version_id = uuid7()
    async with session_factory() as session, session.begin():
        session.add(
            MediaObject(
                id=object_id,
                workspace_id=workspace_id,
                kind="image",
                source_type="upload",
                status="active",
                current_version_id=version_id,
                revision=1,
            )
        )
        session.add(
            MediaVersion(
                id=version_id,
                workspace_id=workspace_id,
                media_object_id=object_id,
                version_no=1,
                filename=filename,
                sha256="a" * 64,
                size_bytes=64,
                mime_type="image/png",
                probe_status="ready",
                probe_attempt=1,
                width=1,
                height=1,
                created_by=actor_id,
            )
        )
        session.add(
            MediaLocation(
                workspace_id=workspace_id,
                media_version_id=version_id,
                storage_profile="test-private",
                bucket="lanverse-test",
                object_key=f"governance/{version_id}/{filename}",
                status="active",
                verified_at=datetime.now(UTC),
            )
        )
    return version_id


def _scope(subject_id: UUID, **overrides: object) -> dict[str, object]:
    scope: dict[str, object] = {
        "type": "media_usage",
        "subject_type": "MEDIA_VERSION",
        "subject_id": str(subject_id),
        "rights_holder_role": "synthetic_creator",
        "rights_types": ["copyright", "image", "voice"],
        "authorized_purposes": [
            "ai_short_drama_generation",
            "public_distribution",
        ],
        "channels": ["lanverse_preview", "lanverse_download"],
        "regions": ["CN"],
        "valid_from": "2026-07-01T00:00:00Z",
        "valid_to": "2027-07-01T00:00:00Z",
    }
    scope.update(overrides)
    return scope


def _create_payload(
    workspace_id: UUID,
    subject_id: UUID,
    proof_id: UUID,
) -> dict[str, object]:
    return {
        "workspace_id": str(workspace_id),
        "subject_identity": {
            "reference": "synthetic-subject-adult-a",
            "kind": "fictional_adult",
        },
        "scope": _scope(subject_id),
        "proof_media_version_ids": [str(proof_id)],
        "reason": "Synthetic fixture for governance acceptance",
        "idempotency_key": "consent-register-001",
    }


@pytest.mark.asyncio
async def test_consent_registration_read_revision_and_revoke_are_append_only(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    test_settings: Settings,
) -> None:
    headers, identity = await _identity(client)
    workspace_id = UUID(identity["workspace"]["id"])
    actor_id = UUID(identity["user"]["id"])
    subject_id = await _seed_accessible_media(
        session_factory,
        workspace_id=workspace_id,
        actor_id=actor_id,
        filename="subject.png",
    )
    proof_id = await _seed_accessible_media(
        session_factory,
        workspace_id=workspace_id,
        actor_id=actor_id,
        filename="consent-proof.png",
    )

    created = await client.post(
        "/api/v1/consents",
        headers=headers,
        json=_create_payload(workspace_id, subject_id, proof_id),
    )
    assert created.status_code == 201
    consent = created.json()["data"]
    consent_id = consent["id"]
    assert consent["workspace_id"] == str(workspace_id)
    assert consent["status"] == "active"
    assert consent["revision"] == 1
    assert consent["subject_identity"] == {
        "reference": "synthetic-subject-adult-a",
        "kind": "fictional_adult",
    }
    assert consent["current_revision"]["action"] == "register"
    assert consent["current_revision"]["proof_media_version_ids"] == [str(proof_id)]
    assert "filename" not in str(consent)
    assert "object_key" not in str(consent)
    assert "bucket" not in str(consent)

    repeated = await client.post(
        "/api/v1/consents",
        headers=headers,
        json=_create_payload(workspace_id, subject_id, proof_id),
    )
    assert repeated.status_code == 201
    assert repeated.json()["data"] == consent
    conflicting_create = await client.post(
        "/api/v1/consents",
        headers=headers,
        json={
            **_create_payload(workspace_id, subject_id, proof_id),
            "reason": "Same key with different facts",
        },
    )
    assert conflicting_create.status_code == 409
    assert conflicting_create.json()["error"]["code"] == "resource_conflict"

    listed = await client.get(
        "/api/v1/consents",
        headers=headers,
        params={"workspace_id": str(workspace_id)},
    )
    assert listed.status_code == 200
    assert listed.json()["data"]["total"] == 1
    assert listed.json()["data"]["items"][0]["id"] == consent_id
    detail = await client.get(f"/api/v1/consents/{consent_id}", headers=headers)
    assert detail.status_code == 200
    assert [item["action"] for item in detail.json()["data"]["revisions"]] == [
        "register"
    ]

    claims = decode_access_token(identity["access_token"], test_settings)
    assert claims is not None
    async with session_factory() as session:
        allowed = await check_rights(
            session,
            workspace_id=workspace_id,
            subject=SubjectReference(
                subject_type=SubjectType.MEDIA_VERSION,
                subject_id=subject_id,
            ),
            usage=RightsUsage(
                purpose="ai_short_drama_generation",
                channel="lanverse_preview",
                region="CN",
                at_time=datetime.now(UTC),
            ),
        )
    assert allowed.allowed is True
    assert allowed.blockers == ()
    assert allowed.consent_ids == (UUID(consent_id),)

    revision_payload = {
        "expected_revision": 1,
        "scope": _scope(subject_id, channels=["lanverse_preview"]),
        "proof_media_version_ids": [str(proof_id)],
        "reason": "Restrict distribution channel",
    }
    first, second = await asyncio.gather(
        client.post(
            f"/api/v1/consents/{consent_id}/revisions",
            headers=headers,
            json=revision_payload,
        ),
        client.post(
            f"/api/v1/consents/{consent_id}/revisions",
            headers=headers,
            json=revision_payload,
        ),
    )
    assert sorted([first.status_code, second.status_code]) == [201, 409]
    updated = first if first.status_code == 201 else second
    assert updated.json()["data"]["revision"] == 2
    assert [
        item["action"] for item in updated.json()["data"]["revisions"]
    ] == ["register", "update"]
    conflict = second if first.status_code == 201 else first
    assert conflict.json()["error"]["code"] == "version_conflict"
    assert conflict.json()["error"]["details"] == {"current_revision": 2}

    revoked = await client.post(
        f"/api/v1/consents/{consent_id}/revoke",
        headers=headers,
        json={"expected_revision": 2, "reason": "Permission withdrawn"},
    )
    assert revoked.status_code == 200
    revoked_data = revoked.json()["data"]
    assert revoked_data["status"] == "revoked"
    assert revoked_data["revision"] == 3
    assert [item["action"] for item in revoked_data["revisions"]] == [
        "register",
        "update",
        "revoke",
    ]
    assert revoked_data["revisions"][0]["scope"]["channels"] == [
        "lanverse_preview",
        "lanverse_download",
    ]

    async with session_factory() as session:
        blocked = await check_rights(
            session,
            workspace_id=workspace_id,
            subject=SubjectReference(
                subject_type=SubjectType.MEDIA_VERSION,
                subject_id=subject_id,
            ),
            usage=RightsUsage(
                purpose="ai_short_drama_generation",
                channel="lanverse_preview",
                region="CN",
                at_time=datetime.now(UTC),
            ),
        )
        assert await session.scalar(select(func.count()).select_from(Consent)) == 1
        assert (
            await session.scalar(select(func.count()).select_from(ConsentRevision))
            == 3
        )
        assert await session.scalar(select(func.count()).select_from(ConsentProof)) == 3
    assert blocked.allowed is False
    assert [blocker.code for blocker in blocked.blockers] == ["consent_revoked"]

    audited = await client.get(
        "/api/v1/audit-events",
        headers=headers,
        params={"workspace_id": str(workspace_id)},
    )
    assert audited.status_code == 200
    audit_data = audited.json()["data"]
    assert audit_data["total"] == 3
    assert [item["action"] for item in audit_data["items"]] == [
        "consent.revoked",
        "consent.revised",
        "consent.registered",
    ]
    assert all(
        item["actor_id"] == str(actor_id)
        and item["target_type"] == "consent"
        and item["target_id"] == consent_id
        and item["result"] == "succeeded"
        and item["trace_id"]
        and set(item["metadata"]) <= {"revision", "subject_type"}
        for item in audit_data["items"]
    )
    assert "Permission withdrawn" not in str(audit_data)
    assert str(proof_id) not in str(audit_data)

    filtered_audit = await client.get(
        "/api/v1/audit-events",
        headers=headers,
        params={
            "workspace_id": str(workspace_id),
            "action": "consent.revoked",
            "target_type": "consent",
            "target_id": consent_id,
        },
    )
    assert filtered_audit.status_code == 200
    assert filtered_audit.json()["data"]["total"] == 1


@pytest.mark.asyncio
async def test_consent_commands_enforce_schema_capabilities_and_workspace_isolation(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    owner_headers, owner = await _identity(client, email="governance-a@example.com")
    other_headers, other = await _identity(client, email="governance-b@example.com")
    workspace_id = UUID(owner["workspace"]["id"])
    other_workspace_id = UUID(other["workspace"]["id"])
    owner_id = UUID(owner["user"]["id"])
    other_id = UUID(other["user"]["id"])
    subject_id = await _seed_accessible_media(
        session_factory,
        workspace_id=workspace_id,
        actor_id=owner_id,
        filename="subject-a.png",
    )
    proof_id = await _seed_accessible_media(
        session_factory,
        workspace_id=workspace_id,
        actor_id=owner_id,
        filename="proof-a.png",
    )
    other_proof_id = await _seed_accessible_media(
        session_factory,
        workspace_id=other_workspace_id,
        actor_id=other_id,
        filename="proof-b.png",
    )
    payload = _create_payload(workspace_id, subject_id, proof_id)

    unknown = await client.post(
        "/api/v1/consents",
        headers=owner_headers,
        json={
            **payload,
            "scope": {**_scope(subject_id), "legal_conclusion": True},
        },
    )
    assert unknown.status_code == 422
    invalid_dates = await client.post(
        "/api/v1/consents",
        headers=owner_headers,
        json={
            **payload,
            "scope": {
                **_scope(subject_id),
                "valid_from": "2027-07-01T00:00:00Z",
                "valid_to": "2026-07-01T00:00:00Z",
            },
        },
    )
    assert invalid_dates.status_code == 422
    cross_workspace_proof = await client.post(
        "/api/v1/consents",
        headers=owner_headers,
        json={**payload, "proof_media_version_ids": [str(other_proof_id)]},
    )
    assert cross_workspace_proof.status_code == 404
    assert cross_workspace_proof.json()["error"]["code"] == "not_found"

    created = await client.post(
        "/api/v1/consents", headers=owner_headers, json=payload
    )
    assert created.status_code == 201
    consent_id = created.json()["data"]["id"]
    assert (
        await client.get(f"/api/v1/consents/{consent_id}", headers=other_headers)
    ).status_code == 404

    async with session_factory() as session, session.begin():
        session.add(
            Membership(
                workspace_id=workspace_id,
                user_id=other_id,
                role="viewer",
                status="active",
            )
        )
    visible = await client.get(
        "/api/v1/consents",
        headers=other_headers,
        params={"workspace_id": str(workspace_id)},
    )
    assert visible.status_code == 200
    forbidden = await client.post(
        f"/api/v1/consents/{consent_id}/revisions",
        headers=other_headers,
        json={
            "expected_revision": 1,
            "scope": _scope(subject_id),
            "proof_media_version_ids": [str(proof_id)],
            "reason": "Viewer may not update consent",
        },
    )
    assert forbidden.status_code == 403
    assert forbidden.json()["error"]["code"] == "forbidden"
    audit_forbidden = await client.get(
        "/api/v1/audit-events",
        headers=other_headers,
        params={"workspace_id": str(workspace_id)},
    )
    assert audit_forbidden.status_code == 403


@pytest.mark.asyncio
async def test_rights_gate_hides_cross_workspace_subjects_and_fails_closed(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    _, owner = await _identity(client, email="rights-a@example.com")
    _, other = await _identity(client, email="rights-b@example.com")
    workspace_id = UUID(owner["workspace"]["id"])
    subject_id = await _seed_accessible_media(
        session_factory,
        workspace_id=workspace_id,
        actor_id=UUID(owner["user"]["id"]),
        filename="rights-subject.png",
    )
    usage = RightsUsage(
        purpose="ai_short_drama_generation",
        channel="lanverse_preview",
        region="CN",
        at_time=datetime(2026, 7, 29, tzinfo=UTC),
    )

    async with session_factory() as session:
        with pytest.raises(ApiError) as hidden:
            await check_rights(
                session,
                workspace_id=UUID(other["workspace"]["id"]),
                subject=SubjectReference(
                    subject_type=SubjectType.MEDIA_VERSION,
                    subject_id=subject_id,
                ),
                usage=usage,
            )
        assert hidden.value.code is ErrorCode.NOT_FOUND
        assert hidden.value.status_code == 404

        with pytest.raises(ApiError) as missing_asset:
            await check_rights(
                session,
                workspace_id=workspace_id,
                subject=SubjectReference(
                    subject_type=SubjectType.ASSET_VERSION,
                    subject_id=uuid7(),
                ),
                usage=usage,
            )
        assert missing_asset.value.code is ErrorCode.NOT_FOUND

        with pytest.raises(ApiError) as unavailable:
            await check_rights(
                session,
                workspace_id=workspace_id,
                subject=SubjectReference(
                    subject_type=SubjectType.SHOT_SPEC_VERSION,
                    subject_id=uuid7(),
                ),
                usage=usage,
            )
        assert unavailable.value.code is ErrorCode.DEPENDENCY_UNAVAILABLE
        assert unavailable.value.status_code == 503

        missing = await check_rights(
            session,
            workspace_id=workspace_id,
            subject=SubjectReference(
                subject_type=SubjectType.MEDIA_VERSION,
                subject_id=subject_id,
            ),
            usage=usage,
        )
    assert missing.allowed is False
    assert [blocker.code for blocker in missing.blockers] == ["consent_missing"]


@pytest.mark.asyncio
async def test_script_version_resolver_uses_the_real_version_boundary(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, identity = await _identity(client, email="script-rights@example.com")
    workspace_id = UUID(identity["workspace"]["id"])
    actor_id = UUID(identity["user"]["id"])
    proof_id = await _seed_accessible_media(
        session_factory,
        workspace_id=workspace_id,
        actor_id=actor_id,
        filename="script-rights-proof.png",
    )
    project_response = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(str(workspace_id), "剧本授权项目"),
    )
    assert project_response.status_code == 201
    project_id = project_response.json()["data"]["id"]
    episode_response = await client.post(
        f"/api/v1/projects/{project_id}/episodes",
        headers=headers,
        json={"name": "第一集", "target_duration_ms": 90000},
    )
    assert episode_response.status_code == 201
    imported_response = await client.post(
        f"/api/v1/episodes/{episode_response.json()['data']['id']}/script-sources",
        headers=headers,
        json={
            "input_type": "text",
            "title": "授权剧本",
            "body": "第一场\n角色走进雨夜。",
            "rights_declaration": "拥有本测试文本的使用权",
            "idempotency_key": "script-rights-import-001",
        },
    )
    assert imported_response.status_code == 201
    script_version_id = UUID(imported_response.json()["data"]["version"]["id"])
    payload = _create_payload(workspace_id, script_version_id, proof_id)
    payload["scope"] = _scope(
        script_version_id, subject_type="SCRIPT_VERSION"
    )
    payload["idempotency_key"] = "script-consent-001"
    created = await client.post(
        "/api/v1/consents", headers=headers, json=payload
    )
    assert created.status_code == 201

    async with session_factory() as session:
        result = await check_rights(
            session,
            workspace_id=workspace_id,
            subject=SubjectReference(
                subject_type=SubjectType.SCRIPT_VERSION,
                subject_id=script_version_id,
            ),
            usage=RightsUsage(
                purpose="ai_short_drama_generation",
                channel="lanverse_preview",
                region="CN",
                at_time=datetime.now(UTC),
            ),
        )
    assert result.allowed is True
    assert result.blockers == ()
    assert result.consent_ids == (UUID(created.json()["data"]["id"]),)
