import asyncio
from typing import Any
from uuid import UUID

import httpx
import pytest
from sqlalchemy import func, select
from sqlalchemy.ext.asyncio import AsyncSession, async_sessionmaker

from app import io_worker
from app.modules.governance.audit.models import AuditEvent
from app.modules.messaging import envelope_from_event
from app.modules.messaging.models import InboxDelivery, OutboxEvent
from app.modules.production.models import Task
from app.modules.projects.models import Episode
from app.modules.scripts.models import ScriptVersion
from tests.support.identity_builders import register_identity_response

SOURCE_BODY = (
    "场景1：海边仓库，暴雨夜\n"
    "林澈冲进仓库，发现两个孩子被困在涨水的地下室。\n"
    "林澈：先救孩子，账以后再算。\n"
    "顾沉带人离开，门外传来锁链落下的声音。\n"
    "林澈摘下旧项链，露出皇室徽记。\n"
)
ADAPTED_BODY = (
    "场景1：海边仓库，暴雨夜\n"
    "海水倒灌。林澈撞开地下室门，两个孩子已经呛水。\n"
    "林澈厉声道：“先救孩子，账以后再算！”\n"
    "顾沉带人撤走，铁链在门外骤然锁死。\n"
    "林澈扯下旧项链，皇室徽记在闪电中亮起。\n"
)


class _RecordingMessage:
    def __init__(self, body: bytes) -> None:
        self.body = body
        self.ack_count = 0
        self.nack_requeues: list[bool] = []

    async def ack(self) -> None:
        self.ack_count += 1

    async def nack(self, *, requeue: bool) -> None:
        self.nack_requeues.append(requeue)


class _RecordingAdapter:
    def __init__(self, result: dict[str, Any] | None = None) -> None:
        self.result = result or {
            "adapted_script_text": ADAPTED_BODY,
            "change_summary": "压缩环境铺陈，前置救援冲突并保留皇室身份钩子。",
            "estimated_duration_ms": 52_000,
        }
        self.inputs: list[dict[str, Any]] = []

    async def adapt(
        self,
        script_body: str,
        *,
        target_duration_ms: int,
        core_plot_points: list[str],
        pacing: str,
        colloquial_dialogue: bool,
    ) -> dict[str, Any]:
        self.inputs.append(
            {
                "script_body": script_body,
                "target_duration_ms": target_duration_ms,
                "core_plot_points": core_plot_points,
                "pacing": pacing,
                "colloquial_dialogue": colloquial_dialogue,
            }
        )
        return self.result


class _SimulatedWorkerExit(BaseException):
    pass


class _InterruptingAdapter(_RecordingAdapter):
    async def adapt(
        self,
        script_body: str,
        *,
        target_duration_ms: int,
        core_plot_points: list[str],
        pacing: str,
        colloquial_dialogue: bool,
    ) -> dict[str, Any]:
        await super().adapt(
            script_body,
            target_duration_ms=target_duration_ms,
            core_plot_points=core_plot_points,
            pacing=pacing,
            colloquial_dialogue=colloquial_dialogue,
        )
        raise _SimulatedWorkerExit


async def _identity(
    client: httpx.AsyncClient,
    *,
    email: str,
) -> tuple[dict[str, str], str]:
    response = await register_identity_response(
        client,
        email=email,
        password="a-secure-adaptation-password",
        display_name="剧本改写负责人",
    )
    assert response.status_code == 201
    data = response.json()["data"]
    return {"authorization": f"Bearer {data['access_token']}"}, data["workspace"]["id"]


async def _published_script(
    client: httpx.AsyncClient,
    *,
    email: str,
    body: str = SOURCE_BODY,
) -> tuple[dict[str, str], dict[str, Any], dict[str, Any], dict[str, Any]]:
    headers, workspace_id = await _identity(client, email=email)
    project_response = await client.post(
        "/api/v1/projects",
        headers=headers,
        json={
            "workspace_id": workspace_id,
            "name": "剧本改写验收项目",
            "aspect_ratio": "9:16",
            "language": "zh-CN",
            "target_duration_ms": 90_000,
        },
    )
    assert project_response.status_code == 201
    project = project_response.json()["data"]
    episode_response = await client.post(
        f"/api/v1/projects/{project['id']}/episodes",
        headers=headers,
        json={"name": "第一集", "target_duration_ms": 90_000},
    )
    assert episode_response.status_code == 201
    episode = episode_response.json()["data"]
    source_response = await client.post(
        f"/api/v1/episodes/{episode['id']}/script-sources",
        headers=headers,
        json={
            "input_type": "text",
            "title": "第一集原稿",
            "body": body,
            "rights_declaration": "确认拥有该原创测试文本的使用权",
            "idempotency_key": f"source:{email}",
        },
    )
    assert source_response.status_code == 201
    source = source_response.json()["data"]["source"]
    published_response = await client.post(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
        json={"body": body, "expected_current_version_id": None},
    )
    assert published_response.status_code == 201
    published = published_response.json()["data"]["version"]
    return headers, episode, source, published


def _create_payload(
    version_id: str,
    *,
    idempotency_key: str,
    target_duration_ms: int = 60_000,
) -> dict[str, Any]:
    return {
        "input_script_version_id": version_id,
        "target_duration_ms": target_duration_ms,
        "core_plot_points": ["孩子必须获救", "结尾揭示林澈的皇室身份"],
        "pacing": "fast",
        "colloquial_dialogue": True,
        "idempotency_key": idempotency_key,
    }


async def _queued_run(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    email: str,
    idempotency_key: str,
) -> tuple[dict[str, str], dict[str, Any], dict[str, Any], bytes]:
    headers, episode, source, current = await _published_script(client, email=email)
    created = await client.post(
        f"/api/v1/episodes/{episode['id']}/adaptation-runs",
        headers=headers,
        json=_create_payload(current["id"], idempotency_key=idempotency_key),
    )
    assert created.status_code == 202
    run = created.json()["data"]
    async with session_factory() as session:
        event = await session.scalar(
            select(OutboxEvent).where(OutboxEvent.aggregate_id == UUID(run["task_id"]))
        )
        assert event is not None
        message_body = envelope_from_event(event).model_dump_json().encode()
    return headers, source, run, message_body


async def _complete_run(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
    *,
    email: str,
    idempotency_key: str,
) -> tuple[dict[str, str], dict[str, Any], dict[str, Any]]:
    headers, source, queued, message_body = await _queued_run(
        client,
        session_factory,
        email=email,
        idempotency_key=idempotency_key,
    )
    message = _RecordingMessage(message_body)
    outcome = await io_worker.process_incoming_message(
        message,
        session_factory,
        adaptation_provider=_RecordingAdapter(),
    )
    assert outcome == "completed"
    assert message.ack_count == 1
    fetched = await client.get(
        f"/api/v1/adaptation-runs/{queued['id']}",
        headers=headers,
    )
    assert fetched.status_code == 200
    return headers, source, fetched.json()["data"]


@pytest.mark.asyncio
async def test_create_run_is_current_bound_idempotent_and_private(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, episode, _, current = await _published_script(
        client,
        email="adaptation-create@example.com",
    )
    endpoint = f"/api/v1/episodes/{episode['id']}/adaptation-runs"
    payload = _create_payload(current["id"], idempotency_key="adaptation-create-001")

    created = await client.post(endpoint, headers=headers, json=payload)

    assert created.status_code == 202
    run = created.json()["data"]
    assert run["episode_id"] == episode["id"]
    assert run["input_script_version_id"] == current["id"]
    assert run["input_hash"] == current["content_hash"]
    assert run["status"] == "queued"
    assert run["revision"] == 1
    assert run["task_id"] is not None
    assert run["candidate_body"] is None
    assert run["draft_body"] is None
    assert run["published_script_version_id"] is None
    assert run["constraints"] == {
        "target_duration_ms": 60_000,
        "core_plot_points": ["孩子必须获救", "结尾揭示林澈的皇室身份"],
        "pacing": "fast",
        "colloquial_dialogue": True,
    }

    repeated = await client.post(endpoint, headers=headers, json=payload)
    assert repeated.status_code == 202
    assert repeated.json()["data"] == run
    conflict = await client.post(
        endpoint,
        headers=headers,
        json=_create_payload(
            current["id"],
            idempotency_key="adaptation-create-001",
            target_duration_ms=45_000,
        ),
    )
    assert conflict.status_code == 409
    assert conflict.json()["error"]["code"] == "resource_conflict"

    async with session_factory() as session:
        events = list(
            await session.scalars(
                select(OutboxEvent).where(OutboxEvent.aggregate_id == UUID(run["task_id"]))
            )
        )
        assert len(events) == 1
        assert events[0].event_type == "script_adaptation.requested"
        assert events[0].payload == {"task_id": run["task_id"]}
        task = await session.get(Task, UUID(run["task_id"]))
        assert task is not None
        assert task.task_type == "script_adaptation"
        assert task.request_type == "adaptation_run"
        assert str(task.input_version_id) == current["id"]
        assert task.input_hash == current["content_hash"]
        serialized_private_surfaces = " ".join(
            [
                str(events[0].payload),
                str(task.error_summary),
                *[str(item) for item in await session.scalars(select(AuditEvent.event_metadata))],
            ]
        )
        assert SOURCE_BODY not in serialized_private_surfaces
        assert "adaptation-create-001" not in serialized_private_surfaces


@pytest.mark.asyncio
async def test_worker_persists_one_candidate_without_mutating_input_or_current(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, queued, message_body = await _queued_run(
        client,
        session_factory,
        email="adaptation-success@example.com",
        idempotency_key="adaptation-success-001",
    )
    adapter = _RecordingAdapter()
    message = _RecordingMessage(message_body)

    outcome = await io_worker.process_incoming_message(
        message,
        session_factory,
        adaptation_provider=adapter,
    )

    assert outcome == "completed"
    assert message.ack_count == 1
    assert message.nack_requeues == []
    assert adapter.inputs == [
        {
            "script_body": SOURCE_BODY,
            "target_duration_ms": 60_000,
            "core_plot_points": ["孩子必须获救", "结尾揭示林澈的皇室身份"],
            "pacing": "fast",
            "colloquial_dialogue": True,
        }
    ]
    fetched = await client.get(f"/api/v1/adaptation-runs/{queued['id']}", headers=headers)
    assert fetched.status_code == 200
    run = fetched.json()["data"]
    assert run["status"] == "succeeded"
    assert run["revision"] == 2
    assert run["candidate_body"] == ADAPTED_BODY
    assert run["draft_body"] == ADAPTED_BODY
    assert run["candidate_hash"] == run["draft_hash"]
    assert run["change_summary"] == "压缩环境铺陈，前置救援冲突并保留皇室身份钩子。"
    assert run["estimated_duration_ms"] == 52_000

    duplicate_message = _RecordingMessage(message_body)
    duplicate = await io_worker.process_incoming_message(
        duplicate_message,
        session_factory,
        adaptation_provider=adapter,
    )
    assert duplicate in {"completed", "duplicate"}
    assert len(adapter.inputs) == 1

    async with session_factory() as session:
        input_version = await session.get(ScriptVersion, UUID(run["input_script_version_id"]))
        episode = await session.get(Episode, UUID(run["episode_id"]))
        assert input_version is not None and input_version.body == SOURCE_BODY
        assert episode is not None
        assert episode.current_script_version_id == input_version.id
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 2


@pytest.mark.asyncio
async def test_missing_provider_input_drift_and_invalid_output_fail_closed(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, no_provider, no_provider_body = await _queued_run(
        client,
        session_factory,
        email="adaptation-no-provider@example.com",
        idempotency_key="adaptation-no-provider",
    )
    no_provider_message = _RecordingMessage(no_provider_body)
    assert (
        await io_worker.process_incoming_message(no_provider_message, session_factory)
        == "completed"
    )
    failed = (
        await client.get(f"/api/v1/adaptation-runs/{no_provider['id']}", headers=headers)
    ).json()["data"]
    assert failed["status"] == "failed"
    assert failed["error_code"] == "ai_service_unavailable"
    assert failed["candidate_body"] is None

    drift_headers, drift_source, drift_run, drift_body = await _queued_run(
        client,
        session_factory,
        email="adaptation-drift@example.com",
        idempotency_key="adaptation-drift",
    )
    switched = await client.post(
        f"/api/v1/script-sources/{drift_source['id']}/versions",
        headers=drift_headers,
        json={
            "body": SOURCE_BODY + "\n新的结尾。",
            "expected_current_version_id": drift_run["input_script_version_id"],
        },
    )
    assert switched.status_code == 201
    drift_adapter = _RecordingAdapter()
    assert (
        await io_worker.process_incoming_message(
            _RecordingMessage(drift_body),
            session_factory,
            adaptation_provider=drift_adapter,
        )
        == "completed"
    )
    drifted = (
        await client.get(f"/api/v1/adaptation-runs/{drift_run['id']}", headers=drift_headers)
    ).json()["data"]
    assert drifted["status"] == "failed"
    assert drifted["error_code"] == "input_version_changed"
    assert drifted["candidate_body"] is None
    assert drift_adapter.inputs == []

    invalid_headers, _, invalid_run, invalid_body = await _queued_run(
        client,
        session_factory,
        email="adaptation-invalid-output@example.com",
        idempotency_key="adaptation-invalid-output",
    )
    invalid_adapter = _RecordingAdapter(
        {
            "adapted_script_text": "字" * 20_001,
            "change_summary": "超长非法结果",
            "estimated_duration_ms": 60_000,
        }
    )
    assert (
        await io_worker.process_incoming_message(
            _RecordingMessage(invalid_body),
            session_factory,
            adaptation_provider=invalid_adapter,
        )
        == "completed"
    )
    invalid = (
        await client.get(f"/api/v1/adaptation-runs/{invalid_run['id']}", headers=invalid_headers)
    ).json()["data"]
    assert invalid["status"] == "failed"
    assert invalid["error_code"] == "ai_output_invalid"
    assert invalid["candidate_body"] is None
    assert invalid["draft_body"] is None

    duration_headers, _, duration_run, duration_body = await _queued_run(
        client,
        session_factory,
        email="adaptation-invalid-duration@example.com",
        idempotency_key="adaptation-invalid-duration",
    )
    duration_adapter = _RecordingAdapter(
        {
            "adapted_script_text": ADAPTED_BODY,
            "change_summary": "未达到目标时长的候选",
            "estimated_duration_ms": 15_000,
        }
    )
    assert (
        await io_worker.process_incoming_message(
            _RecordingMessage(duration_body),
            session_factory,
            adaptation_provider=duration_adapter,
        )
        == "completed"
    )
    invalid_duration = (
        await client.get(
            f"/api/v1/adaptation-runs/{duration_run['id']}",
            headers=duration_headers,
        )
    ).json()["data"]
    assert invalid_duration["status"] == "failed"
    assert invalid_duration["error_code"] == "ai_output_invalid"
    assert invalid_duration["candidate_body"] is None


@pytest.mark.asyncio
async def test_interrupted_delivery_becomes_unknown_without_blind_resubmit(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, queued, message_body = await _queued_run(
        client,
        session_factory,
        email="adaptation-unknown@example.com",
        idempotency_key="adaptation-unknown",
    )
    interrupted = _InterruptingAdapter()
    with pytest.raises(_SimulatedWorkerExit):
        await io_worker.process_incoming_message(
            _RecordingMessage(message_body),
            session_factory,
            adaptation_provider=interrupted,
        )
    assert len(interrupted.inputs) == 1

    replacement = _RecordingAdapter()
    redelivery = _RecordingMessage(message_body)
    outcome = await io_worker.process_incoming_message(
        redelivery,
        session_factory,
        adaptation_provider=replacement,
    )

    assert outcome == "completed"
    assert redelivery.ack_count == 1
    assert replacement.inputs == []
    run = (await client.get(f"/api/v1/adaptation-runs/{queued['id']}", headers=headers)).json()[
        "data"
    ]
    assert run["status"] == "unknown"
    assert run["error_code"] == "ai_result_unknown"
    assert run["candidate_body"] is None
    async with session_factory() as session:
        task = await session.get(Task, UUID(queued["task_id"]))
        assert task is not None and task.status == "unknown"
        delivery = await session.scalar(
            select(InboxDelivery).where(InboxDelivery.task_id == task.id)
        )
        assert delivery is not None
        assert delivery.status == "completed"
        assert delivery.attempt_count == 2


@pytest.mark.asyncio
async def test_edit_diff_and_publish_use_run_and_current_compare_and_set(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, source, run = await _complete_run(
        client,
        session_factory,
        email="adaptation-publish@example.com",
        idempotency_key="adaptation-publish",
    )
    edited_body = ADAPTED_BODY.replace("账以后再算", "这笔账，我稍后亲自清算")
    edited = await client.patch(
        f"/api/v1/adaptation-runs/{run['id']}/draft",
        headers=headers,
        json={"body": edited_body, "expected_revision": 2},
    )
    assert edited.status_code == 200
    edited_run = edited.json()["data"]
    assert edited_run["revision"] == 3
    assert edited_run["candidate_body"] == ADAPTED_BODY
    assert edited_run["draft_body"] == edited_body
    assert edited_run["candidate_hash"] != edited_run["draft_hash"]

    stale_edit = await client.patch(
        f"/api/v1/adaptation-runs/{run['id']}/draft",
        headers=headers,
        json={"body": edited_body + "\n陈旧覆盖", "expected_revision": 2},
    )
    assert stale_edit.status_code == 409
    assert stale_edit.json()["error"]["code"] == "version_conflict"

    difference = await client.get(
        f"/api/v1/adaptation-runs/{run['id']}/diff",
        headers=headers,
    )
    assert difference.status_code == 200
    diff = difference.json()["data"]
    assert diff["base_version_id"] == run["input_script_version_id"]
    assert diff["adaptation_run_id"] == run["id"]
    assert diff["added_lines"] >= 1
    assert diff["removed_lines"] >= 1
    assert any("这笔账，我稍后亲自清算" in line for line in diff["diff_lines"])

    publish_payload = {
        "expected_run_revision": 3,
        "expected_current_version_id": run["input_script_version_id"],
        "idempotency_key": "publish-adaptation-001",
    }
    published = await client.post(
        f"/api/v1/adaptation-runs/{run['id']}/publish",
        headers=headers,
        json=publish_payload,
    )
    assert published.status_code == 200
    result = published.json()["data"]
    assert result["run"]["status"] == "published"
    assert result["run"]["revision"] == 4
    assert result["version"]["body"] == edited_body
    assert result["version"]["status"] == "published"
    assert result["run"]["published_script_version_id"] == result["version"]["id"]
    assert result["current"]["current_script_version_id"] == result["version"]["id"]

    replay = await client.post(
        f"/api/v1/adaptation-runs/{run['id']}/publish",
        headers=headers,
        json=publish_payload,
    )
    assert replay.status_code == 200
    assert replay.json()["data"] == result

    history = await client.get(
        f"/api/v1/script-sources/{source['id']}/versions",
        headers=headers,
    )
    assert [item["body"] for item in history.json()["data"]["items"]] == [
        SOURCE_BODY,
        SOURCE_BODY,
        edited_body,
    ]
    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 3


@pytest.mark.asyncio
async def test_concurrent_publish_has_one_winner_and_creates_one_version(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, run = await _complete_run(
        client,
        session_factory,
        email="adaptation-concurrent-publish@example.com",
        idempotency_key="adaptation-concurrent-publish",
    )
    endpoint = f"/api/v1/adaptation-runs/{run['id']}/publish"
    first, second = await asyncio.gather(
        client.post(
            endpoint,
            headers=headers,
            json={
                "expected_run_revision": 2,
                "expected_current_version_id": run["input_script_version_id"],
                "idempotency_key": "concurrent-publish-a",
            },
        ),
        client.post(
            endpoint,
            headers=headers,
            json={
                "expected_run_revision": 2,
                "expected_current_version_id": run["input_script_version_id"],
                "idempotency_key": "concurrent-publish-b",
            },
        ),
    )
    responses = [first, second]
    assert sorted(response.status_code for response in responses) == [200, 409]
    conflict = next(response for response in responses if response.status_code == 409)
    assert conflict.json()["error"]["code"] == "version_conflict"
    async with session_factory() as session:
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 3


@pytest.mark.asyncio
async def test_queued_cancel_is_idempotent_and_prevents_provider_call(
    client: httpx.AsyncClient,
    session_factory: async_sessionmaker[AsyncSession],
) -> None:
    headers, _, run, message_body = await _queued_run(
        client,
        session_factory,
        email="adaptation-cancel@example.com",
        idempotency_key="adaptation-cancel",
    )
    payload = {"expected_revision": 1, "idempotency_key": "cancel-adaptation-001"}
    cancelled = await client.post(
        f"/api/v1/adaptation-runs/{run['id']}/cancel",
        headers=headers,
        json=payload,
    )
    assert cancelled.status_code == 200
    result = cancelled.json()["data"]
    assert result["status"] == "cancelled"
    assert result["revision"] == 2
    replay = await client.post(
        f"/api/v1/adaptation-runs/{run['id']}/cancel",
        headers=headers,
        json=payload,
    )
    assert replay.status_code == 200
    assert replay.json()["data"] == result

    adapter = _RecordingAdapter()
    outcome = await io_worker.process_incoming_message(
        _RecordingMessage(message_body),
        session_factory,
        adaptation_provider=adapter,
    )
    assert outcome == "completed"
    assert adapter.inputs == []
    async with session_factory() as session:
        task = await session.get(Task, UUID(run["task_id"]))
        episode = await session.get(Episode, UUID(run["episode_id"]))
        assert task is not None and task.status == "cancelled"
        assert episode is not None
        assert str(episode.current_script_version_id) == run["input_script_version_id"]
        assert await session.scalar(select(func.count()).select_from(ScriptVersion)) == 2
