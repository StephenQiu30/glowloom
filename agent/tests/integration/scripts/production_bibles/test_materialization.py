from datetime import UTC, datetime
from hashlib import sha256
from typing import Any
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.core.auth import AccessTokenClaims, decode_access_token
from app.core.config import Settings
from app.core.errors import ApiError, ErrorCode
from app.modules.assets.models import Asset, AssetNameRevision, AssetState, AssetVersion
from app.modules.governance.audit.models import AuditEvent
from app.modules.scripts.production_bibles.materialization import (
    confirm_and_materialize_production_bible,
    plan_production_bible_materialization,
)
from app.modules.scripts.production_bibles.models import (
    ProductionBible,
    ProductionBibleEntity,
    ProductionBibleEntityState,
)
from app.modules.scripts.production_bibles.schemas import ProductionBibleConfirmRequest
from tests.support.project_builders import project_payload, register_project_owner


def _evidence(text: str, anchor: str, *, episode_number: int = 1) -> dict[str, object]:
    source_start = text.index(anchor)
    return {
        "source_start": source_start,
        "source_end": source_start + len(anchor),
        "text_hash": sha256(anchor.encode()).hexdigest(),
        "exact_anchor": anchor,
        "episode_number": episode_number,
    }


async def _project_revision(
    client: httpx.AsyncClient,
    test_settings: Settings,
    *,
    email: str,
    text: str,
) -> tuple[dict[str, str], AccessTokenClaims, UUID, UUID, dict[str, Any]]:
    headers, workspace_id = await register_project_owner(client, email=email)
    token = headers["authorization"].removeprefix("Bearer ")
    claims = decode_access_token(token, test_settings)
    assert claims is not None
    created_project = await client.post(
        "/api/v1/projects",
        headers=headers,
        json=project_payload(workspace_id, name="Production Bible Materialization"),
    )
    assert created_project.status_code == 201
    project_id = UUID(created_project.json()["data"]["id"])
    imported = await client.post(
        f"/api/v1/projects/{project_id}/script-imports",
        headers=headers,
        json={
            "input_type": "text",
            "title": "Whole Script",
            "text": text,
            "language": "en-US",
            "rights_declaration": "Test fixture rights are confirmed.",
            "idempotency_key": f"materialization-script:{email}",
        },
    )
    assert imported.status_code == 201
    revision = imported.json()["data"]["revision"]
    return headers, claims, UUID(workspace_id), project_id, revision


def _bible(
    *,
    bible_id: UUID,
    workspace_id: UUID,
    project_id: UUID,
    document_revision_id: UUID,
    input_hash: str,
    actor_id: UUID,
    result_hash: str = "b" * 64,
) -> ProductionBible:
    now = datetime.now(UTC)
    return ProductionBible(
        id=bible_id,
        workspace_id=workspace_id,
        project_id=project_id,
        document_revision_id=document_revision_id,
        task_id=None,
        status="needs_review",
        input_hash=input_hash,
        result_hash=result_hash,
        engine_version="test-engine",
        model_name="codex-local",
        prompt_version="test-prompt",
        schema_version="test-schema",
        harness_version="test-harness",
        checkpoint=None,
        checkpoint_revision=0,
        checkpoint_updated_at=None,
        run_token=None,
        lease_expires_at=None,
        review_issues=[],
        revision=2,
        idempotency_key=f"bible:{bible_id}",
        confirm_idempotency_key=None,
        confirm_command_hash=None,
        confirm_result={},
        confirmed_at=None,
        confirmed_by=None,
        created_by=actor_id,
        created_at=now,
        updated_at=now,
    )


@pytest.mark.asyncio
async def test_confirmation_materializes_one_identity_with_multiple_asset_states(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    test_settings: Settings,
) -> None:
    text = (
        "EPISODE 1\nAurelia appears in her imperial robe.\n"
        "EPISODE 2\nAurelia returns wearing travel clothes."
    )
    headers, claims, workspace_id, project_id, revision = await _project_revision(
        client,
        test_settings,
        email="bible-materialization@example.com",
        text=text,
    )
    bible_id = uuid7()
    entity_id = uuid7()
    base_state_id = uuid7()
    travel_state_id = uuid7()
    entity_evidence = _evidence(text, "Aurelia")
    base_evidence = _evidence(text, "imperial robe")
    travel_evidence = _evidence(text, "travel clothes", episode_number=2)
    async with session_factory() as session:
        async with session.begin():
            session.add(
                _bible(
                    bible_id=bible_id,
                    workspace_id=workspace_id,
                    project_id=project_id,
                    document_revision_id=UUID(revision["id"]),
                    input_hash=str(revision["normalized_hash"]),
                    actor_id=claims.sub,
                )
            )
            session.add(
                ProductionBibleEntity(
                    id=entity_id,
                    workspace_id=workspace_id,
                    project_id=project_id,
                    bible_id=bible_id,
                    entity_key="character:aurelia",
                    kind="character",
                    canonical_name="Aurelia",
                    normalized_name="aurelia",
                    aliases=["Empress", "Mother"],
                    stable_spec={
                        "identity": "Aurelia",
                        "temperament": ["resolute"],
                    },
                    episode_numbers=[1, 2],
                    evidence=[entity_evidence],
                    asset_id=None,
                )
            )
            session.add_all(
                [
                    ProductionBibleEntityState(
                        id=base_state_id,
                        workspace_id=workspace_id,
                        project_id=project_id,
                        bible_id=bible_id,
                        entity_id=entity_id,
                        state_key="base",
                        label="Imperial presentation",
                        state_spec={"appearance": "imperial robe"},
                        episode_numbers=[1],
                        evidence=[base_evidence],
                        asset_state_id=None,
                        asset_version_id=None,
                    ),
                    ProductionBibleEntityState(
                        id=travel_state_id,
                        workspace_id=workspace_id,
                        project_id=project_id,
                        bible_id=bible_id,
                        entity_id=entity_id,
                        state_key="travel",
                        label="Travel presentation",
                        state_spec={"appearance": "travel clothes"},
                        episode_numbers=[2],
                        evidence=[travel_evidence],
                        asset_state_id=None,
                        asset_version_id=None,
                    ),
                ]
            )

    async with session_factory() as session:
        plan = await plan_production_bible_materialization(session, bible_id)
        assert plan.confirmable
        assert plan.issues == ()
        assert len(plan.entities) == 1
        assert plan.entities[0].action == "create"
        assert [state.state_key for state in plan.entities[0].states] == [
            "base",
            "travel",
        ]

    request = ProductionBibleConfirmRequest(
        expected_revision=2,
        expected_result_hash="b" * 64,
        idempotency_key="confirm-bible-materialization",
    )
    async with session_factory() as session:
        result = await confirm_and_materialize_production_bible(
            session,
            claims,
            bible_id,
            request,
            trace_id="trace-confirm-bible-materialization",
        )
    assert result.status == "confirmed"
    assert result.revision == 3
    assert result.replayed is False
    assert set(result.entity_asset_ids) == {"character:aurelia"}
    assert set(result.state_bindings) == {
        "character:aurelia:base",
        "character:aurelia:travel",
    }

    async with session_factory() as session:
        replay = await confirm_and_materialize_production_bible(
            session,
            claims,
            bible_id,
            request,
            trace_id="trace-confirm-bible-materialization-replay",
        )
    assert replay.replayed is True
    assert replay.entity_asset_ids == result.entity_asset_ids
    assert replay.state_bindings == result.state_bindings

    api_replay = await client.post(
        f"/api/v1/production-bibles/{bible_id}/confirm",
        headers=headers,
        json=request.model_dump(mode="json"),
    )
    assert api_replay.status_code == 200
    confirmed = api_replay.json()["data"]
    assert confirmed["status"] == "confirmed"
    assert confirmed["entities"][0]["asset_id"] == str(result.entity_asset_ids["character:aurelia"])
    assert {state["state_key"] for state in confirmed["entities"][0]["states"]} == {
        "base",
        "travel",
    }

    async with session_factory() as session:
        assets = list(await session.scalars(select(Asset)))
        states = list(await session.scalars(select(AssetState).order_by(AssetState.state_key)))
        versions = list(
            await session.scalars(select(AssetVersion).order_by(AssetVersion.version_no))
        )
        names = list(await session.scalars(select(AssetNameRevision)))
        stored_entity = await session.get(ProductionBibleEntity, entity_id)
        stored_base = await session.get(ProductionBibleEntityState, base_state_id)
        stored_travel = await session.get(ProductionBibleEntityState, travel_state_id)
        stored_bible = await session.get(ProductionBible, bible_id)
        assert len(assets) == 1
        assert assets[0].name == "Aurelia"
        assert assets[0].aliases == ["Empress", "Mother"]
        assert len(names) == 1
        assert len(states) == 2
        assert len(versions) == 2
        assert {version.source_type for version in versions} == {"production_bible_state"}
        assert {version.source_id for version in versions} == {
            base_state_id,
            travel_state_id,
        }
        assert versions[0].spec["identity"] == "Aurelia"
        assert [version.spec["appearance"] for version in versions] == [
            "imperial robe",
            "travel clothes",
        ]
        assert stored_entity is not None
        assert stored_entity.asset_id == assets[0].id
        assert stored_base is not None
        assert stored_base.asset_state_id is not None
        assert stored_base.asset_version_id is not None
        assert stored_base.evidence == [base_evidence]
        assert stored_travel is not None
        assert stored_travel.asset_state_id is not None
        assert stored_travel.asset_version_id is not None
        assert stored_travel.evidence == [travel_evidence]
        assert stored_bible is not None
        assert stored_bible.status == "confirmed"
        assert stored_bible.confirm_idempotency_key == request.idempotency_key
        assert stored_bible.confirmed_by == claims.sub
        assert (
            await session.scalar(
                select(func.count())
                .select_from(AuditEvent)
                .where(AuditEvent.action == "asset.created")
            )
            == 1
        )
        assert (
            await session.scalar(
                select(func.count())
                .select_from(AuditEvent)
                .where(AuditEvent.action == "asset.state_created")
            )
            == 1
        )
        assert (
            await session.scalar(
                select(func.count())
                .select_from(AuditEvent)
                .where(AuditEvent.action == "asset.version_created")
            )
            == 2
        )
        assert (
            await session.scalar(
                select(func.count())
                .select_from(AuditEvent)
                .where(AuditEvent.action == "script.production_bible_confirmed")
            )
            == 1
        )

    version_response = await client.get(
        f"/api/v1/asset-versions/{versions[0].id}",
        headers=headers,
    )
    assert version_response.status_code == 200
    assert version_response.json()["data"]["source_type"] == "production_bible_state"
    assert version_response.json()["data"]["source_id"] in {
        str(base_state_id),
        str(travel_state_id),
    }


@pytest.mark.asyncio
async def test_same_kind_alias_collision_blocks_confirmation_without_partial_assets(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    test_settings: Settings,
) -> None:
    text = "Aurelia is called the Empress. Valerie impersonates the Empress."
    _, claims, workspace_id, project_id, revision = await _project_revision(
        client,
        test_settings,
        email="bible-alias-collision@example.com",
        text=text,
    )
    bible_id = uuid7()
    now = datetime.now(UTC)
    entities: list[ProductionBibleEntity] = []
    states: list[ProductionBibleEntityState] = []
    for key, name, anchor in (
        ("character:aurelia", "Aurelia", "Aurelia"),
        ("character:valerie", "Valerie", "Valerie"),
    ):
        entity_id = uuid7()
        entities.append(
            ProductionBibleEntity(
                id=entity_id,
                workspace_id=workspace_id,
                project_id=project_id,
                bible_id=bible_id,
                entity_key=key,
                kind="character",
                canonical_name=name,
                normalized_name=name.casefold(),
                aliases=["Empress"],
                stable_spec={"identity": name},
                episode_numbers=[1],
                evidence=[_evidence(text, anchor)],
                asset_id=None,
                created_at=now,
                updated_at=now,
            )
        )
        states.append(
            ProductionBibleEntityState(
                id=uuid7(),
                workspace_id=workspace_id,
                project_id=project_id,
                bible_id=bible_id,
                entity_id=entity_id,
                state_key="base",
                label="Base",
                state_spec={"appearance": name},
                episode_numbers=[1],
                evidence=[_evidence(text, anchor)],
                asset_state_id=None,
                asset_version_id=None,
                created_at=now,
                updated_at=now,
            )
        )
    async with session_factory() as session:
        async with session.begin():
            session.add(
                _bible(
                    bible_id=bible_id,
                    workspace_id=workspace_id,
                    project_id=project_id,
                    document_revision_id=UUID(revision["id"]),
                    input_hash=str(revision["normalized_hash"]),
                    actor_id=claims.sub,
                )
            )
            session.add_all(entities)
            session.add_all(states)

    async with session_factory() as session:
        plan = await plan_production_bible_materialization(session, bible_id)
    assert not plan.confirmable
    assert "identity_token_collision" in {issue.code for issue in plan.issues}

    with pytest.raises(ApiError) as blocked:
        async with session_factory() as session:
            await confirm_and_materialize_production_bible(
                session,
                claims,
                bible_id,
                ProductionBibleConfirmRequest(
                    expected_revision=2,
                    expected_result_hash="b" * 64,
                    idempotency_key="confirm-colliding-bible",
                ),
                trace_id="trace-confirm-colliding-bible",
            )
    assert blocked.value.code == ErrorCode.STATE_CONFLICT
    assert blocked.value.next_action == "resolve_production_bible_issues"
    assert "identity_token_collision" in {
        issue["code"] for issue in blocked.value.details["issues"]
    }

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(Asset)) == 0
        stored_bible = await session.get(ProductionBible, bible_id)
        assert stored_bible is not None
        assert stored_bible.status == "needs_review"
        assert stored_bible.confirmed_at is None


@pytest.mark.asyncio
async def test_existing_identity_match_requires_explicit_link_instead_of_duplication(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    test_settings: Settings,
) -> None:
    text = "Aurelia is the Empress."
    headers, claims, workspace_id, project_id, revision = await _project_revision(
        client,
        test_settings,
        email="bible-existing-identity@example.com",
        text=text,
    )
    existing = await client.post(
        f"/api/v1/projects/{project_id}/assets",
        headers=headers,
        json={
            "kind": "character",
            "name": "Aurelia",
            "aliases": ["Empress"],
            "tags": [],
        },
    )
    assert existing.status_code == 201
    bible_id = uuid7()
    entity_id = uuid7()
    now = datetime.now(UTC)
    async with session_factory() as session:
        async with session.begin():
            session.add(
                _bible(
                    bible_id=bible_id,
                    workspace_id=workspace_id,
                    project_id=project_id,
                    document_revision_id=UUID(revision["id"]),
                    input_hash=str(revision["normalized_hash"]),
                    actor_id=claims.sub,
                )
            )
            session.add(
                ProductionBibleEntity(
                    id=entity_id,
                    workspace_id=workspace_id,
                    project_id=project_id,
                    bible_id=bible_id,
                    entity_key="character:aurelia",
                    kind="character",
                    canonical_name="Aurelia",
                    normalized_name="aurelia",
                    aliases=["Empress"],
                    stable_spec={"identity": "Aurelia"},
                    episode_numbers=[1],
                    evidence=[_evidence(text, "Aurelia")],
                    asset_id=None,
                    created_at=now,
                    updated_at=now,
                )
            )

    async with session_factory() as session:
        plan = await plan_production_bible_materialization(session, bible_id)
    assert not plan.confirmable
    assert "existing_identity_requires_explicit_link" in {issue.code for issue in plan.issues}
    assert "entity_missing_state" in {issue.code for issue in plan.issues}

    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(Asset)) == 1


@pytest.mark.asyncio
async def test_review_issue_resolution_repairs_state_and_preserves_idempotent_hash_chain(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    test_settings: Settings,
) -> None:
    text = (
        "EPISODE 32\nThe assignment is still unsigned and signing remains conditional.\n"
        "EPISODE 33\nTristan signs the Eos-9 collateral assignment."
    )
    headers, claims, workspace_id, project_id, revision = await _project_revision(
        client,
        test_settings,
        email="bible-review-resolution@example.com",
        text=text,
    )
    bible_id = uuid7()
    entity_id = uuid7()
    base_state_id = uuid7()
    state_id = uuid7()
    bible = _bible(
        bible_id=bible_id,
        workspace_id=workspace_id,
        project_id=project_id,
        document_revision_id=UUID(revision["id"]),
        input_hash=str(revision["normalized_hash"]),
        actor_id=claims.sub,
    )
    bible.review_issues = [
        {
            "issue_key": "issue:eos_assignment_signed_episode_mapping",
            "code": "STATE_EPISODE_MAPPING_PREMATURE",
            "severity": "blocking",
            "scope": "entity_state",
            "subject_key": "prop:eos_9_collateral_assignment/state:signed",
            "summary": "The signed state starts one episode too early.",
            "repair_hint": "Keep the signed state only from episode 33.",
            "evidence": [
                _evidence(text, "signing remains conditional", episode_number=32),
                _evidence(text, "Tristan signs", episode_number=33),
            ],
        }
    ]
    async with session_factory() as session, session.begin():
        session.add(bible)
        session.add(
            ProductionBibleEntity(
                id=entity_id,
                workspace_id=workspace_id,
                project_id=project_id,
                bible_id=bible_id,
                entity_key="prop:eos_9_collateral_assignment",
                kind="prop",
                canonical_name="Eos-9 collateral assignment",
                normalized_name="eos-9 collateral assignment",
                aliases=[],
                stable_spec={"usage_context": "Fraudulent collateral assignment"},
                episode_numbers=[32, 33],
                evidence=[_evidence(text, "assignment", episode_number=32)],
                asset_id=None,
            )
        )
        session.add_all(
            [
                ProductionBibleEntityState(
                    id=base_state_id,
                    workspace_id=workspace_id,
                    project_id=project_id,
                    bible_id=bible_id,
                    entity_id=entity_id,
                    state_key="base",
                    label="Unsigned",
                    state_spec={"appearance": "Unsigned assignment"},
                    episode_numbers=[32],
                    evidence=[_evidence(text, "still unsigned", episode_number=32)],
                    asset_state_id=None,
                    asset_version_id=None,
                ),
                ProductionBibleEntityState(
                    id=state_id,
                    workspace_id=workspace_id,
                    project_id=project_id,
                    bible_id=bible_id,
                    entity_id=entity_id,
                    state_key="signed",
                    label="Signed",
                    state_spec={"appearance": "Signed by Tristan"},
                    episode_numbers=[32, 33],
                    evidence=[_evidence(text, "Tristan signs", episode_number=33)],
                    asset_state_id=None,
                    asset_version_id=None,
                ),
            ]
        )

    resolution_payload = {
        "expected_revision": 2,
        "expected_result_hash": "b" * 64,
        "idempotency_key": "resolve-signed-state-001",
        "issue_key": "issue:eos_assignment_signed_episode_mapping",
        "resolution_note": "Episode 32 is conditional; the signature happens in episode 33.",
        "correction": {
            "kind": "entity_state_episode_numbers",
            "entity_key": "prop:eos_9_collateral_assignment",
            "state_key": "signed",
            "episode_numbers": [33],
        },
    }
    resolved_response = await client.post(
        f"/api/v1/production-bibles/{bible_id}/review-issue-resolutions",
        headers=headers,
        json=resolution_payload,
    )
    assert resolved_response.status_code == 200
    resolved = resolved_response.json()["data"]
    assert resolved["revision"] == 3
    assert resolved["result_hash"] != "b" * 64
    assert resolved["review_issues"] == []
    resolved_states = {state["state_key"]: state for state in resolved["entities"][0]["states"]}
    assert resolved_states["signed"]["episode_numbers"] == [33]

    replay_response = await client.post(
        f"/api/v1/production-bibles/{bible_id}/review-issue-resolutions",
        headers=headers,
        json=resolution_payload,
    )
    assert replay_response.status_code == 200
    assert replay_response.json()["data"]["revision"] == 3
    assert replay_response.json()["data"]["result_hash"] == resolved["result_hash"]

    conflicting_payload = {
        **resolution_payload,
        "resolution_note": "A different command using the same key.",
    }
    conflict_response = await client.post(
        f"/api/v1/production-bibles/{bible_id}/review-issue-resolutions",
        headers=headers,
        json=conflicting_payload,
    )
    assert conflict_response.status_code == 409

    confirmed_response = await client.post(
        f"/api/v1/production-bibles/{bible_id}/confirm",
        headers=headers,
        json={
            "expected_revision": 3,
            "expected_result_hash": resolved["result_hash"],
            "idempotency_key": "confirm-resolved-bible-001",
        },
    )
    assert confirmed_response.status_code == 200, confirmed_response.text
    assert confirmed_response.json()["data"]["status"] == "confirmed"

    async with session_factory() as session:
        stored_state = await session.get(ProductionBibleEntityState, state_id)
        assert stored_state is not None
        assert stored_state.episode_numbers == [33]
        assert (
            await session.scalar(
                select(func.count())
                .select_from(AuditEvent)
                .where(AuditEvent.action == "script.production_bible_review_issue_resolved")
            )
            == 1
        )
