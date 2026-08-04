from copy import deepcopy
from datetime import UTC, datetime
from decimal import Decimal
from typing import Any, cast
from uuid import UUID

import httpx
import pytest
from fastapi import FastAPI
from sqlalchemy import func, select
from sqlalchemy.exc import IntegrityError
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker
from uuid6 import uuid7

from app.modules.caching.contracts import (
    CacheUnavailableError,
    HighCostGuardRequest,
    HighCostGuardResult,
)
from app.modules.caching.dependencies import get_high_cost_guard
from app.modules.messaging.models import OutboxEvent
from app.modules.production import generation
from app.modules.production.models import (
    CostEntry,
    GenerationRequest,
    ModelCapability,
    Reservation,
    Task,
)
from tests.integration.storyboards.test_storyboards_api import (
    create_episode_with_confirmed_structure,
    create_ready_location_asset,
    shot_creation_payload,
    shot_spec_payload,
)


async def create_ready_generation_shot(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> tuple[dict[str, str], dict[str, Any], dict[str, UUID], dict[str, Any]]:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="generation-request-owner@example.com",
    )
    project_response = await client.get(
        f"/api/v1/projects/{episode['project_id']}",
        headers=headers,
    )
    assert project_response.status_code == 200
    budget = await client.post(
        f"/api/v1/projects/{episode['project_id']}/budget-limit",
        headers=headers,
        json={
            "amount": "100.000000",
            "currency": "CNY",
            "expected_revision": project_response.json()["data"]["revision"],
        },
    )
    assert budget.status_code == 200
    location_version, _ = await create_ready_location_asset(
        client,
        session_factory,
        headers=headers,
        project_id=UUID(episode["project_id"]),
        refs=refs,
    )
    created = await client.post(
        f"/api/v1/episodes/{episode['id']}/shots",
        headers=headers,
        json=shot_creation_payload(
            refs,
            title="生成事实验收镜头",
            creation_key="generation-fact-shot",
        ),
    )
    assert created.status_code == 201
    shot = created.json()["data"]
    spec_payload = deepcopy(shot_spec_payload(refs, purpose="验证生成事实而非模型结果"))
    visual = dict(cast(dict[str, object], spec_payload["visual"]))
    visual["subject_placements"] = []
    spec_payload["visual"] = visual
    spec_payload["dialogue_or_narration"] = []
    saved = await client.post(
        f"/api/v1/shots/{shot['id']}/spec-versions",
        headers=headers,
        json={
            "expected_current_spec_version_id": None,
            "spec": spec_payload,
            "asset_references": [
                {
                    "slot_key": "location-main",
                    "role": "location",
                    "asset_version_id": location_version["id"],
                    "subject_key": None,
                }
            ],
        },
    )
    assert saved.status_code == 201
    return (
        headers,
        episode,
        refs,
        {
            "shot": shot,
            "spec_version": saved.json()["data"]["version"],
        },
    )


async def seed_active_capability(
    session_factory: async_sessionmaker[AsyncSession],
    *,
    amount: str = "12.500000",
    high_cost_threshold: str = "10.000000",
) -> ModelCapability:
    capability = ModelCapability(
        id=uuid7(),
        provider="verified_contract_fixture",
        model="verified-image-contract-v1",
        kind="image",
        config_version=1,
        input_types=["shot_spec"],
        parameter_schema={
            "type": "object",
            "properties": {
                "resolution": {"type": "string", "enum": ["720p", "1080p"]},
            },
            "required": ["resolution"],
            "additionalProperties": False,
        },
        limits={"outputs": 1},
        pricing={
            "unit": "per_request",
            "amount": amount,
            "currency": "CNY",
            "high_cost_threshold": high_cost_threshold,
        },
        status="active",
        unavailable_reason=None,
    )
    async with session_factory() as session, session.begin():
        session.add(capability)
    return capability


class UnavailableHighCostGuard:
    def __init__(self) -> None:
        self.calls = 0

    async def authorize_high_cost(
        self,
        request: HighCostGuardRequest,
    ) -> HighCostGuardResult:
        _ = request
        self.calls += 1
        raise CacheUnavailableError("forced high cost guard outage")


class RejectingHighCostGuard:
    def __init__(self, outcome: str) -> None:
        self.outcome = outcome

    async def authorize_high_cost(
        self,
        request: HighCostGuardRequest,
    ) -> HighCostGuardResult:
        _ = request
        return HighCostGuardResult(
            allowed=False,
            outcome=cast(Any, self.outcome),
            retry_after_seconds=30,
        )


@pytest.mark.asyncio
async def test_unverified_builtin_capabilities_are_visible_but_never_submit_ready(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, refs, shot_facts = await create_ready_generation_shot(client, session_factory)

    catalog = await client.get(
        "/api/v1/model-capabilities",
        headers=headers,
        params={"workspace_id": str(refs["workspace_id"])},
    )
    assert catalog.status_code == 200
    items = catalog.json()["data"]
    assert [item["model"] for item in items] == [
        "doubao-seedream-5-0-lite-260128",
        "doubao-seedance-2-0-260128",
    ]
    assert all(item["status"] == "unavailable" for item in items)
    assert all(item["unavailable_reason"] == "provider_contract_unverified" for item in items)
    assert all(item["pricing"] is None for item in items)
    assert "endpoint" not in str(items).lower()
    assert "api_key" not in str(items).lower()

    before = await generation_fact_counts(session_factory)
    preflight = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-preflight",
        headers=headers,
        json={
            "workspace_id": str(refs["workspace_id"]),
            "shot_spec_version_id": shot_facts["spec_version"]["id"],
            "capability_id": items[0]["id"],
            "parameters": {},
        },
    )
    assert preflight.status_code == 200
    result = preflight.json()["data"]
    assert result["status"] == "blocked"
    assert result["ready"] is False
    assert result["estimated_cost"] is None
    assert [item["code"] for item in result["blocking_reasons"]] == ["CAPABILITY_UNAVAILABLE"]
    assert await generation_fact_counts(session_factory) == before

    unknown_parameter = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-preflight",
        headers=headers,
        json={
            "workspace_id": str(refs["workspace_id"]),
            "shot_spec_version_id": shot_facts["spec_version"]["id"],
            "capability_id": items[0]["id"],
            "parameters": {"provider_payload": "must-not-pass"},
        },
    )
    assert unknown_parameter.status_code == 422
    assert unknown_parameter.json()["error"]["code"] == "validation_failed"
    assert await generation_fact_counts(session_factory) == before


@pytest.mark.asyncio
async def test_preflight_and_submission_are_side_effect_safe_atomic_and_idempotent(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    high_cost_guard: Any,
) -> None:
    headers, episode, refs, shot_facts = await create_ready_generation_shot(client, session_factory)
    capability = await seed_active_capability(session_factory)
    preflight_payload = {
        "workspace_id": str(refs["workspace_id"]),
        "shot_spec_version_id": shot_facts["spec_version"]["id"],
        "capability_id": str(capability.id),
        "parameters": {"resolution": "1080p"},
    }
    before = await generation_fact_counts(session_factory)
    preflight = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-preflight",
        headers=headers,
        json=preflight_payload,
    )
    assert preflight.status_code == 200
    result = preflight.json()["data"]
    assert result["status"] == "ready"
    assert result["ready"] is True
    assert result["estimated_cost"] == {
        "amount": "12.500000",
        "currency": "CNY",
        "pricing_version": 1,
        "unit": "per_request",
    }
    assert result["confirmation_requirements"] == [
        {
            "code": "ACKNOWLEDGE_WARNINGS",
            "warning_codes": ["STYLE_REFERENCE_MISSING"],
        },
        {"code": "CONFIRM_HIGH_COST", "warning_codes": []},
    ]
    assert len(result["preflight_hash"]) == 64
    assert await generation_fact_counts(session_factory) == before

    submission_payload = {
        **preflight_payload,
        "preflight_hash": result["preflight_hash"],
        "preflight_expires_at": result["expires_at"],
        "warning_acknowledgements": ["STYLE_REFERENCE_MISSING"],
        "high_cost_confirmed": False,
        "idempotency_key": "generate-shot-image-001",
    }
    missing_confirmation = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-requests",
        headers=headers,
        json=submission_payload,
    )
    assert missing_confirmation.status_code == 409
    assert missing_confirmation.json()["error"]["next_action"] == "confirm_generation_cost"
    assert await generation_fact_counts(session_factory) == before

    created = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-requests",
        headers=headers,
        json={**submission_payload, "high_cost_confirmed": True},
    )
    assert created.status_code == 201
    data = created.json()["data"]
    assert data["replayed"] is False
    assert data["request"]["workspace_id"] == str(refs["workspace_id"])
    assert data["request"]["shot_id"] == shot_facts["shot"]["id"]
    assert data["request"]["shot_spec_version_id"] == shot_facts["spec_version"]["id"]
    assert data["request"]["parameter_snapshot"] == {"resolution": "1080p"}
    assert data["task"]["task_type"] == "image_generation"
    assert data["task"]["request_type"] == "generation_request"
    assert data["task"]["status"] == "queued"
    assert data["reservation"]["estimated_amount"] == "12.500000"
    assert data["reservation"]["reserved_amount"] == "12.500000"
    assert data["initial_cost_entry"]["entry_type"] == "reserve"
    assert data["initial_cost_entry"]["amount"] == "12.500000"
    assert data["outbox_event_id"]

    repeated = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-requests",
        headers=headers,
        json={**submission_payload, "high_cost_confirmed": True},
    )
    assert repeated.status_code == 201
    replay = repeated.json()["data"]
    assert replay["replayed"] is True
    for key in ("request", "task", "reservation", "initial_cost_entry"):
        assert replay[key]["id"] == data[key]["id"]
    assert await generation_fact_counts(session_factory) == (1, 1, 1, 1, 1)

    changed_preflight = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-preflight",
        headers=headers,
        json={**preflight_payload, "parameters": {"resolution": "720p"}},
    )
    assert changed_preflight.status_code == 200
    conflict = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-requests",
        headers=headers,
        json={
            **submission_payload,
            "parameters": {"resolution": "720p"},
            "preflight_hash": changed_preflight.json()["data"]["preflight_hash"],
            "preflight_expires_at": changed_preflight.json()["data"]["expires_at"],
            "high_cost_confirmed": True,
        },
    )
    assert conflict.status_code == 409
    assert await generation_fact_counts(session_factory) == (1, 1, 1, 1, 1)
    assert len(high_cost_guard.calls) == 1
    assert high_cost_guard.calls[0].idempotency_digest != "generate-shot-image-001"
    assert len(high_cost_guard.calls[0].idempotency_digest) == 64
    assert high_cost_guard.calls[0].request_hash == data["request"]["input_hash"]

    costs = await client.get(
        "/api/v1/costs",
        headers=headers,
        params={
            "workspace_id": str(refs["workspace_id"]),
            "project_id": episode["project_id"],
        },
    )
    assert costs.status_code == 200
    cost_data = costs.json()["data"]
    assert cost_data["currency"] == "CNY"
    assert cost_data["summary"] == {
        "reserved": "12.500000",
        "settled": "0.000000",
        "released": "0.000000",
        "adjustments": "0.000000",
        "remaining_reserved": "12.500000",
    }
    assert len(cost_data["items"]) == 1
    assert cost_data["items"][0]["task_id"] == data["task"]["id"]

    with pytest.raises(IntegrityError):
        async with session_factory() as session, session.begin():
            session.add(
                CostEntry(
                    id=uuid7(),
                    workspace_id=uuid7(),
                    reservation_id=UUID(data["reservation"]["id"]),
                    attempt_id=None,
                    entry_type="reserve",
                    amount=Decimal("1.000000"),
                    currency="CNY",
                    provider_bill_ref=None,
                    idempotency_key="cross-workspace-reserve",
                    created_at=datetime.now(UTC),
                )
            )


@pytest.mark.asyncio
async def test_high_cost_submission_fails_closed_before_any_business_fact(
    app: FastAPI,
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, refs, shot_facts = await create_ready_generation_shot(client, session_factory)
    capability = await seed_active_capability(session_factory)
    preflight_payload = {
        "workspace_id": str(refs["workspace_id"]),
        "shot_spec_version_id": shot_facts["spec_version"]["id"],
        "capability_id": str(capability.id),
        "parameters": {"resolution": "1080p"},
    }
    preflight = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-preflight",
        headers=headers,
        json=preflight_payload,
    )
    assert preflight.status_code == 200
    result = preflight.json()["data"]
    submission = {
        **preflight_payload,
        "preflight_hash": result["preflight_hash"],
        "preflight_expires_at": result["expires_at"],
        "warning_acknowledgements": ["STYLE_REFERENCE_MISSING"],
        "high_cost_confirmed": True,
        "idempotency_key": "high-cost-fail-closed",
    }

    unavailable = UnavailableHighCostGuard()
    app.dependency_overrides[get_high_cost_guard] = lambda: unavailable
    dependency_failure = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-requests",
        headers=headers,
        json=submission,
    )
    assert dependency_failure.status_code == 503
    assert dependency_failure.json()["error"] == {
        "code": "dependency_unavailable",
        "message": "High cost protection is unavailable",
        "request_id": dependency_failure.json()["error"]["request_id"],
        "next_action": "retry_when_high_cost_protection_recovers",
        "details": {},
    }
    assert unavailable.calls == 1
    assert await generation_fact_counts(session_factory) == (0, 0, 0, 0, 0)

    workspace_limit = RejectingHighCostGuard("workspace_limit")
    app.dependency_overrides[get_high_cost_guard] = lambda: workspace_limit
    limited = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-requests",
        headers=headers,
        json=submission,
    )
    assert limited.status_code == 422
    assert limited.json()["error"]["code"] == "quota_insufficient"
    assert limited.json()["error"]["next_action"] == "retry_after_high_cost_window"
    assert limited.json()["error"]["details"] == {
        "scope": "workspace",
        "retry_after_seconds": 30,
    }
    assert await generation_fact_counts(session_factory) == (0, 0, 0, 0, 0)

    changed_preflight = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-preflight",
        headers=headers,
        json={**preflight_payload, "parameters": {"resolution": "720p"}},
    )
    assert changed_preflight.status_code == 200
    changed_result = changed_preflight.json()["data"]
    conflict = RejectingHighCostGuard("idempotency_conflict")
    app.dependency_overrides[get_high_cost_guard] = lambda: conflict
    reused = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-requests",
        headers=headers,
        json={
            **submission,
            "parameters": {"resolution": "720p"},
            "preflight_hash": changed_result["preflight_hash"],
            "preflight_expires_at": changed_result["expires_at"],
        },
    )
    assert reused.status_code == 409
    assert reused.json()["error"]["code"] == "state_conflict"
    assert reused.json()["error"]["next_action"] == "use_new_idempotency_key"
    assert await generation_fact_counts(session_factory) == (0, 0, 0, 0, 0)


@pytest.mark.asyncio
async def test_low_cost_submission_does_not_depend_on_redis_guard(
    app: FastAPI,
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, refs, shot_facts = await create_ready_generation_shot(client, session_factory)
    capability = await seed_active_capability(
        session_factory,
        amount="1.000000",
        high_cost_threshold="10.000000",
    )
    preflight_payload = {
        "workspace_id": str(refs["workspace_id"]),
        "shot_spec_version_id": shot_facts["spec_version"]["id"],
        "capability_id": str(capability.id),
        "parameters": {"resolution": "1080p"},
    }
    preflight = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-preflight",
        headers=headers,
        json=preflight_payload,
    )
    assert preflight.status_code == 200
    result = preflight.json()["data"]
    assert all(
        item["code"] != "CONFIRM_HIGH_COST"
        for item in result["confirmation_requirements"]
    )

    unavailable = UnavailableHighCostGuard()
    app.dependency_overrides[get_high_cost_guard] = lambda: unavailable
    created = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-requests",
        headers=headers,
        json={
            **preflight_payload,
            "preflight_hash": result["preflight_hash"],
            "preflight_expires_at": result["expires_at"],
            "warning_acknowledgements": ["STYLE_REFERENCE_MISSING"],
            "high_cost_confirmed": False,
            "idempotency_key": "low-cost-without-redis",
        },
    )
    assert created.status_code == 201
    assert created.json()["data"]["reservation"]["reserved_amount"] == "1.000000"
    assert unavailable.calls == 0
    assert await generation_fact_counts(session_factory) == (1, 1, 1, 1, 1)


@pytest.mark.asyncio
async def test_submission_rolls_back_every_fact_when_outbox_enqueue_fails(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    headers, _, refs, shot_facts = await create_ready_generation_shot(client, session_factory)
    capability = await seed_active_capability(session_factory)
    preflight_payload = {
        "workspace_id": str(refs["workspace_id"]),
        "shot_spec_version_id": shot_facts["spec_version"]["id"],
        "capability_id": str(capability.id),
        "parameters": {"resolution": "1080p"},
    }
    preflight = await client.post(
        f"/api/v1/shots/{shot_facts['shot']['id']}/generation-preflight",
        headers=headers,
        json=preflight_payload,
    )
    assert preflight.status_code == 200
    result = preflight.json()["data"]

    async def fail_outbox(*args: object, **kwargs: object) -> UUID:
        _ = args, kwargs
        raise RuntimeError("forced outbox failure")

    monkeypatch.setattr(generation, "enqueue_outbox_event", fail_outbox)
    with pytest.raises(RuntimeError, match="forced outbox failure"):
        await client.post(
            f"/api/v1/shots/{shot_facts['shot']['id']}/generation-requests",
            headers=headers,
            json={
                **preflight_payload,
                "preflight_hash": result["preflight_hash"],
                "preflight_expires_at": result["expires_at"],
                "warning_acknowledgements": ["STYLE_REFERENCE_MISSING"],
                "high_cost_confirmed": True,
                "idempotency_key": "generation-outbox-rollback",
            },
        )
    assert await generation_fact_counts(session_factory) == (0, 0, 0, 0, 0)


async def generation_fact_counts(
    session_factory: async_sessionmaker[AsyncSession],
) -> tuple[int, int, int, int, int]:
    async with session_factory() as session:
        requests = await session.scalar(select(func.count()).select_from(GenerationRequest))
        tasks = await session.scalar(
            select(func.count())
            .select_from(Task)
            .where(Task.task_type.in_(("image_generation", "video_generation")))
        )
        reservations = await session.scalar(select(func.count()).select_from(Reservation))
        costs = await session.scalar(select(func.count()).select_from(CostEntry))
        outbox = await session.scalar(
            select(func.count())
            .select_from(OutboxEvent)
            .where(OutboxEvent.event_type == "generation.requested")
        )
    return (
        requests or 0,
        tasks or 0,
        reservations or 0,
        costs or 0,
        outbox or 0,
    )
