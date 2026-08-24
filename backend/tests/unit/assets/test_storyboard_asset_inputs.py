from datetime import UTC, datetime
from typing import Any, cast

import pytest
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.modules.assets import service
from app.modules.assets.contracts import AssetVersionReadinessReference
from app.modules.assets.models import Asset, AssetOccurrenceDecision, AssetState


@pytest.mark.asyncio
async def test_storyboard_planning_accepts_semantic_version_before_media_is_ready(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    workspace_id = uuid7()
    project_id = uuid7()
    actor_id = uuid7()
    asset_id = uuid7()
    state_id = uuid7()
    version_id = uuid7()
    now = datetime.now(UTC)
    asset = Asset(
        id=asset_id,
        workspace_id=workspace_id,
        project_id=project_id,
        kind="character",
        name="Mara",
        normalized_name="mara",
        aliases=[],
        tags=["production_bible"],
        status="active",
        availability="enabled",
        name_revision=1,
        revision=1,
        command_receipts={},
        created_by=actor_id,
        created_at=now,
        updated_at=now,
    )
    state = AssetState(
        id=state_id,
        workspace_id=workspace_id,
        asset_id=asset_id,
        state_key="base",
        label="Base",
        description="",
        status="active",
        current_version_id=version_id,
        revision=1,
        creation_key="base",
        command_receipts={},
        created_by=actor_id,
        created_at=now,
        updated_at=now,
    )

    async def find_state_scopes(*_: object, **__: object) -> list[tuple[AssetState, Asset]]:
        return [(state, asset)]

    async def readiness(*_: object, **__: object) -> dict[Any, Any]:
        return {
            version_id: AssetVersionReadinessReference(
                id=version_id,
                asset_id=asset_id,
                asset_state_id=state_id,
                kind="character",
                asset_status="active",
                asset_availability="enabled",
                asset_state_status="active",
                status="draft",
                blocker_codes=("asset_media_missing",),
                media_version_ids=(),
                consent_ids=(),
                evaluation_hash="b" * 64,
            )
        }

    monkeypatch.setattr(service.repository, "find_state_scopes", find_state_scopes)
    monkeypatch.setattr(service, "resolve_asset_versions_readiness", readiness)

    result = await service.resolve_storyboard_planning_assets(
        cast(AsyncSession, object()),
        workspace_id,
        project_id,
        [state_id],
    )

    assert len(result) == 1
    assert result[0].asset_state_id == state_id
    assert result[0].asset_version_id == version_id
    assert result[0].readiness_hash == "b" * 64


@pytest.mark.asyncio
async def test_episode_storyboard_assets_follow_current_occurrence_decisions(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    workspace_id = uuid7()
    episode_id = uuid7()
    state_id = uuid7()
    unit_id = uuid7()
    old_version_id = uuid7()
    current_version_id = uuid7()
    actor_id = uuid7()
    now = datetime.now(UTC)
    rows = [
        AssetOccurrenceDecision(
            id=uuid7(),
            workspace_id=workspace_id,
            asset_state_id=state_id,
            episode_id=episode_id,
            narrative_unit_id=unit_id,
            narrative_unit_version_id=old_version_id,
            sequence=1,
            decision="link",
            origin="script_candidate",
            evidence_hash="a" * 64,
            idempotency_key="old-link",
            created_by=actor_id,
            created_at=now,
        ),
        AssetOccurrenceDecision(
            id=uuid7(),
            workspace_id=workspace_id,
            asset_state_id=state_id,
            episode_id=episode_id,
            narrative_unit_id=unit_id,
            narrative_unit_version_id=current_version_id,
            sequence=2,
            decision="link",
            origin="script_candidate",
            evidence_hash="b" * 64,
            idempotency_key="current-link",
            created_by=actor_id,
            created_at=now,
        ),
    ]

    async def list_occurrences(*_: object, **__: object) -> list[AssetOccurrenceDecision]:
        return rows

    monkeypatch.setattr(
        service.repository,
        "list_episode_occurrence_decisions",
        list_occurrences,
    )

    result = await service.resolve_episode_storyboard_asset_state_ids(
        cast(AsyncSession, object()),
        episode_id,
        narrative_unit_version_ids={current_version_id},
    )
    units = await service.resolve_episode_storyboard_asset_units(
        cast(AsyncSession, object()),
        episode_id,
        narrative_unit_version_ids={current_version_id},
    )

    assert result == (state_id,)
    assert units == {state_id: (current_version_id,)}
