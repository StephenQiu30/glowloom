from typing import Any, cast
from uuid import UUID

import httpx
import pytest
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from tests.integration.storyboards.test_storyboards_api import (
    create_episode_with_confirmed_structure,
    shot_creation_payload,
    shot_spec_payload,
)


async def _shot_with_spec(
    client: httpx.AsyncClient,
    *,
    headers: dict[str, str],
    episode: dict[str, Any],
    refs: dict[str, UUID],
    key: str,
) -> tuple[dict[str, Any], dict[str, Any]]:
    created = await client.post(
        f"/api/v1/episodes/{episode['id']}/shots",
        headers=headers,
        json=shot_creation_payload(refs, title=f"覆盖镜头 {key}", creation_key=key),
    )
    assert created.status_code == 201
    shot = created.json()["data"]
    spec = shot_spec_payload(refs, purpose=f"验证叙事覆盖 {key}")
    cast(dict[str, object], spec["visual"])["subject_placements"] = []
    spec["dialogue_or_narration"] = []
    appended = await client.post(
        f"/api/v1/shots/{shot['id']}/spec-versions",
        headers=headers,
        json={
            "expected_current_spec_version_id": None,
            "spec": spec,
            "asset_references": [],
        },
    )
    assert appended.status_code == 201
    return appended.json()["data"]["shot"], appended.json()["data"]["version"]


def _full_reference(unit: dict[str, Any], *, role: str = "primary") -> dict[str, Any]:
    return {
        "unit_version_id": unit["unit_version_id"],
        "channel": unit["required_channel"],
        "role": role,
        "coverage_mode": "full",
        "segment_start": None,
        "segment_end": None,
        "contribution": "required",
    }


async def _replace_references(
    client: httpx.AsyncClient,
    *,
    headers: dict[str, str],
    shot: dict[str, Any],
    spec: dict[str, Any],
    report: dict[str, Any],
    references: list[dict[str, Any]],
) -> httpx.Response:
    return await client.post(
        f"/api/v1/shots/{shot['id']}/narrative-references",
        headers=headers,
        json={
            "expected_shot_revision": shot["revision"],
            "expected_current_spec_version_id": spec["id"],
            "expected_evaluation_hash": report["evaluation_hash"],
            "references": references,
        },
    )


@pytest.mark.asyncio
async def test_many_to_many_coverage_is_bidirectional_and_gates_readiness(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="coverage-bidirectional@example.com",
    )
    shot, spec = await _shot_with_spec(
        client,
        headers=headers,
        episode=episode,
        refs=refs,
        key="coverage-bidirectional-001",
    )

    initial_response = await client.get(
        f"/api/v1/episodes/{episode['id']}/coverage",
        headers=headers,
    )
    assert initial_response.status_code == 200
    initial = initial_response.json()["data"]
    assert initial["status"] == "blocked"
    assert initial["summary"]["required_total"] > 0
    assert initial["summary"]["uncovered"] == initial["summary"]["required_total"]
    assert initial["summary"]["orphan"] == 1

    readiness = await client.get(
        f"/api/v1/shots/{shot['id']}/readiness",
        headers=headers,
    )
    assert readiness.status_code == 200
    blocker_codes = {
        item["code"] for item in readiness.json()["data"]["blocking_reasons"]
    }
    assert {"COVERAGE_UNACCOUNTED", "SHOT_SOURCE_ORPHAN"} <= blocker_codes

    replaced = await _replace_references(
        client,
        headers=headers,
        shot=shot,
        spec=spec,
        report=initial,
        references=[_full_reference(unit) for unit in initial["units"]],
    )
    assert replaced.status_code == 201, replaced.text
    applied = replaced.json()["data"]
    assert applied["previous_spec_version_id"] == spec["id"]
    assert applied["current_spec_version_id"] != spec["id"]
    assert applied["shot_revision"] == shot["revision"] + 1
    report = applied["report"]
    assert report["status"] == "ready"
    assert report["summary"]["covered"] == report["summary"]["required_total"]
    assert report["summary"]["orphan"] == 0
    assert report["summary"]["stale"] == 0
    assert {item["shot_id"] for item in report["shots"]} == {shot["id"]}
    assert all(item["shot_ids"] == [shot["id"]] for item in report["units"])
    assert set(report["shots"][0]["unit_version_ids"]) == {
        item["unit_version_id"] for item in report["units"]
    }

    readiness_after = await client.get(
        f"/api/v1/shots/{shot['id']}/readiness",
        headers=headers,
    )
    assert readiness_after.status_code == 200
    after_codes = {
        item["code"]
        for item in readiness_after.json()["data"]["blocking_reasons"]
    }
    assert "COVERAGE_UNACCOUNTED" not in after_codes
    assert "SHOT_SOURCE_ORPHAN" not in after_codes


@pytest.mark.asyncio
async def test_one_unit_can_support_two_shots_but_primary_is_unique(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="coverage-primary@example.com",
    )
    first_shot, first_spec = await _shot_with_spec(
        client,
        headers=headers,
        episode=episode,
        refs=refs,
        key="coverage-primary-001",
    )
    second_shot, second_spec = await _shot_with_spec(
        client,
        headers=headers,
        episode=episode,
        refs=refs,
        key="coverage-primary-002",
    )
    report = (
        await client.get(
            f"/api/v1/episodes/{episode['id']}/coverage",
            headers=headers,
        )
    ).json()["data"]
    unit = next(item for item in report["units"] if item["required_for_coverage"])
    first = await _replace_references(
        client,
        headers=headers,
        shot=first_shot,
        spec=first_spec,
        report=report,
        references=[_full_reference(unit)],
    )
    assert first.status_code == 201, first.text
    report = first.json()["data"]["report"]
    second = await _replace_references(
        client,
        headers=headers,
        shot=second_shot,
        spec=second_spec,
        report=report,
        references=[_full_reference(unit, role="supporting")],
    )
    assert second.status_code == 201, second.text
    report = second.json()["data"]["report"]
    unit_result = next(
        item for item in report["units"] if item["unit_version_id"] == unit["unit_version_id"]
    )
    assert unit_result["shot_ids"] == [first_shot["id"], second_shot["id"]]

    duplicate_primary = await _replace_references(
        client,
        headers=headers,
        shot=second_shot | {"revision": second.json()["data"]["shot_revision"]},
        spec={"id": second.json()["data"]["current_spec_version_id"]},
        report=report,
        references=[_full_reference(unit)],
    )
    assert duplicate_primary.status_code == 409
    assert duplicate_primary.json()["error"]["code"] == "state_conflict"
    assert duplicate_primary.json()["error"]["details"]["reason"] == (
        "duplicate_primary_reference"
    )


@pytest.mark.asyncio
async def test_omission_and_invented_approvals_are_append_only_and_become_stale(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="coverage-decisions@example.com",
    )
    sourced_shot, sourced_spec = await _shot_with_spec(
        client,
        headers=headers,
        episode=episode,
        refs=refs,
        key="coverage-decision-001",
    )
    invented_shot, invented_spec = await _shot_with_spec(
        client,
        headers=headers,
        episode=episode,
        refs=refs,
        key="coverage-decision-002",
    )
    report = (
        await client.get(
            f"/api/v1/episodes/{episode['id']}/coverage",
            headers=headers,
        )
    ).json()["data"]
    required_units = [item for item in report["units"] if item["required_for_coverage"]]
    omitted = required_units[-1]
    references = [
        _full_reference(unit)
        for unit in report["units"]
        if unit["unit_version_id"] != omitted["unit_version_id"]
    ]
    mapped = await _replace_references(
        client,
        headers=headers,
        shot=sourced_shot,
        spec=sourced_spec,
        report=report,
        references=references,
    )
    assert mapped.status_code == 201, mapped.text
    mapped_data = mapped.json()["data"]
    report = mapped_data["report"]

    omission_payload = {
        "action": "approve_omission",
        "unit_version_id": omitted["unit_version_id"],
        "shot_spec_version_id": None,
        "reason": "该信息已由上一镜头的动作明确表达",
        "evidence": "导演审核：删去不会破坏因果链",
        "expected_evaluation_hash": report["evaluation_hash"],
        "idempotency_key": "coverage-omit-001",
    }
    omitted_response = await client.post(
        f"/api/v1/episodes/{episode['id']}/coverage-decisions",
        headers=headers,
        json=omission_payload,
    )
    assert omitted_response.status_code == 201, omitted_response.text
    omitted_result = omitted_response.json()["data"]
    assert omitted_result["decision"]["sequence"] == 1
    report = omitted_result["report"]
    assert report["summary"]["approved_omitted"] == 1

    replay = await client.post(
        f"/api/v1/episodes/{episode['id']}/coverage-decisions",
        headers=headers,
        json=omission_payload,
    )
    assert replay.status_code == 201
    assert replay.json()["data"]["decision"] == omitted_result["decision"]

    invented_response = await client.post(
        f"/api/v1/episodes/{episode['id']}/coverage-decisions",
        headers=headers,
        json={
            "action": "approve_invented",
            "unit_version_id": None,
            "shot_spec_version_id": invented_spec["id"],
            "reason": "用环境空镜建立下一场的空间方向",
            "evidence": "导演审核：保留创作性过渡镜头",
            "expected_evaluation_hash": report["evaluation_hash"],
            "idempotency_key": "coverage-invented-001",
        },
    )
    assert invented_response.status_code == 201, invented_response.text
    report = invented_response.json()["data"]["report"]
    assert report["status"] == "ready"
    assert report["summary"]["approved_invented"] == 1
    assert report["summary"]["orphan"] == 0

    changed_references = references.copy()
    changed_references[0] = changed_references[0] | {"role": "supporting"}
    changed = await _replace_references(
        client,
        headers=headers,
        shot=sourced_shot | {"revision": mapped_data["shot_revision"]},
        spec={"id": mapped_data["current_spec_version_id"]},
        report=report,
        references=changed_references,
    )
    assert changed.status_code == 201, changed.text
    stale_report = changed.json()["data"]["report"]
    assert stale_report["status"] == "blocked"
    assert stale_report["summary"]["stale"] == 2
    assert stale_report["summary"]["approved_omitted"] == 0
    assert stale_report["summary"]["approved_invented"] == 0


@pytest.mark.asyncio
async def test_mapping_rejects_stale_cas_and_cross_episode_unit(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="coverage-cas@example.com",
    )
    shot, spec = await _shot_with_spec(
        client,
        headers=headers,
        episode=episode,
        refs=refs,
        key="coverage-cas-001",
    )
    report = (
        await client.get(
            f"/api/v1/episodes/{episode['id']}/coverage",
            headers=headers,
        )
    ).json()["data"]

    other_headers, other_episode, _other_refs = await create_episode_with_confirmed_structure(
        client,
        session_factory,
        email="coverage-cross-episode@example.com",
    )
    other_report = (
        await client.get(
            f"/api/v1/episodes/{other_episode['id']}/coverage",
            headers=other_headers,
        )
    ).json()["data"]
    cross_episode = await _replace_references(
        client,
        headers=headers,
        shot=shot,
        spec=spec,
        report=report,
        references=[_full_reference(other_report["units"][0])],
    )
    assert cross_episode.status_code == 422
    assert cross_episode.json()["error"]["details"]["reason"] == (
        "unit_version_outside_episode"
    )

    applied = await _replace_references(
        client,
        headers=headers,
        shot=shot,
        spec=spec,
        report=report,
        references=[_full_reference(unit) for unit in report["units"]],
    )
    assert applied.status_code == 201, applied.text
    stale = await _replace_references(
        client,
        headers=headers,
        shot=shot,
        spec=spec,
        report=report,
        references=[_full_reference(unit) for unit in report["units"]],
    )
    assert stale.status_code == 409
    assert stale.json()["error"]["code"] == "version_conflict"
