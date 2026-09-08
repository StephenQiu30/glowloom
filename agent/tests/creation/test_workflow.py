from typing import Any
from unittest.mock import AsyncMock, patch

import pytest

from app.creation.activities import CreationActivities
from app.creation.execution import InvocationLease
from app.creation.platform import verify_gate
from app.creation.repository import Repository
from tests.creation.test_repository import new_command


async def test_native_worker_waits_at_every_gate_and_resumes_after_restart(
    repository: Repository,
) -> None:
    import asyncio
    import json
    import time
    from uuid import uuid4

    import httpx
    from temporalio.client import Client, WorkflowExecutionStatus
    from temporalio.worker import Worker

    from app.creation.execution import ExecutionStore
    from app.creation.platform import PlatformUnavailable
    from app.creation.temporal import TemporalStarter
    from app.creation.workflow import TextStoryboardWorkflow
    from app.main import create_app
    from app.modules.text_storyboard.harness import RELEASE_HASH, TextHarness
    from app.text_contract.task import TextTask
    from tests.creation.test_contract import SECRET, authorization
    from tests.creation.test_native_runtime import native_temporal_address
    from tests.unit.text_storyboard_samples import sample

    source, episode_map, analyses, world, direction = sample()
    command = new_command()
    command = command.model_copy(
        update={
            "source": command.source.model_copy(
                update={"revision_id": source.revision_id, "content_hash": source.content_hash}
            )
        }
    )
    await repository.accept(command, "native-placeholder")
    client = await Client.connect(native_temporal_address())
    queue = "lanverse-core-test-" + uuid4().hex
    approved: set[str] = set()
    platform = AsyncMock()
    source_online = True

    async def read_source(_: Any) -> Any:
        if not source_online:
            raise PlatformUnavailable("platform_unavailable")
        return source

    platform.source.side_effect = read_source

    async def gate(_: Any, stage: str, __: Any) -> dict[str, Any]:
        return {
            "status": "accepted" if stage in approved else "pending",
            "gate": stage,
            "selected_scenes": [{"episode_key": "episode-one", "scene_key": "scene"}]
            if stage == "analyze_episode"
            else [],
        }

    platform.gate.side_effect = gate
    calls: list[str] = []

    async def invoke(task: TextTask) -> dict[str, Any]:
        calls.append(task.stage)

        async def reason(*_: Any) -> Any:
            if task.stage == "map_manuscript":
                return episode_map
            if task.stage == "analyze_episode":
                return next(item for item in analyses if item.episode_key == task.episode_key)
            return world if task.stage == "build_world" else direction

        return (await TextHarness(reason).execute(task)).model_dump(mode="json")

    harness = AsyncMock()
    harness.invoke.side_effect = invoke

    def worker() -> Worker:
        service = CreationActivities(
            ExecutionStore(Repository(repository.dsn)), platform, harness, RELEASE_HASH, 8
        )
        return Worker(
            client,
            task_queue=queue,
            workflows=[TextStoryboardWorkflow],
            activities=[service.invoke, service.gate, service.progress],
        )

    store = ExecutionStore(repository)

    async def waiting(stage: str, status: str = "waiting_review") -> None:
        async with asyncio.timeout(25):
            while True:
                try:
                    snapshot = await store.snapshot(command.command_id)
                    if snapshot["stage"] == stage and snapshot["status"] == status:
                        return
                except Exception:
                    pass
                await asyncio.sleep(0.05)

    handle = client.get_workflow_handle(command.workflow_id)
    try:
        async with worker():
            await TemporalStarter(client).ensure_started(command, queue)
            await waiting("map_manuscript")
            assert calls == ["map_manuscript"]
            original = await store.snapshot(command.command_id)
        # A new Worker and repository reconnect replay the same Temporal history and draft.
        async with worker():
            assert (await store.snapshot(command.command_id))["outputs"] == original["outputs"]
            assert calls == ["map_manuscript"]
            # A platform outage after map adoption blocks the same run without charging a call.
            source_online = False
            approved.add("map_manuscript")
            await handle.signal("review_changed")
            await waiting("analyze_episode", "blocked")
            blocked = await store.snapshot(command.command_id)
            assert blocked["can_resume"] and blocked["last_error"] == "platform_unavailable"
            assert blocked["reserved_calls"] == 1 and calls == ["map_manuscript"]
            source_online = True
            endpoint = f"/internal/creation/commands/{command.command_id}/resume"
            body = json.dumps({"payload_hash": command.payload_hash}).encode()
            headers = {
                "X-Lanverse-Creation-Authorization": authorization(
                    body, endpoint, "POST", int(time.time()) + 60
                )
            }
            async with httpx.AsyncClient(
                transport=httpx.ASGITransport(
                    app=create_app(repository, SECRET, queue, temporal=client)
                ),
                base_url="http://test",
            ) as http:
                assert (await http.post(endpoint, content=body)).status_code == 401
                resumed = await http.post(endpoint, content=body, headers=headers)
                assert resumed.status_code == 202
                assert resumed.json()["status"] == "resume_requested"
                await waiting("analyze_episode")
                assert len(calls) == 3
                replay = await http.post(endpoint, content=body, headers=headers)
                assert replay.status_code == 202 and replay.json() == resumed.json()
            for stage, next_stage, expected in [
                ("analyze_episode", "build_world", 4),
                ("build_world", "direct_scene", 5),
            ]:
                approved.add(stage)
                await handle.signal("review_changed")
                await waiting(next_stage)
                assert len(calls) == expected
            approved.add("direct_scene")
            await handle.signal("review_changed")
            async with asyncio.timeout(25):
                assert await handle.result() == {"run_id": command.run_id, "status": "completed"}
            final = await store.snapshot(command.command_id)
            assert final["status"] == "completed" and final["reserved_calls"] == 5
    finally:
        if (await handle.describe()).status == WorkflowExecutionStatus.RUNNING:
            await handle.terminate(reason="synthetic core workflow test cleanup")


def test_accepted_gate_requires_exact_formal_receipts() -> None:
    command = new_command()
    draft = {
        "draft_id": command.run_id,
        "step_id": command.run_id,
        "candidate_hash": "a" * 64,
        "result_hash": "b" * 64,
    }
    response: dict[str, Any] = {
        "status": "accepted",
        "gate": "map_manuscript",
        "receipts": [],
        "human_task_ids": [],
        "selected_scenes": [],
    }
    with pytest.raises(ValueError, match="gate_receipt_mismatch"):
        verify_gate(command, "map_manuscript", [draft], response)


async def test_unknown_workflow_reports_blocked_without_restarting_inference() -> None:
    import asyncio

    from temporalio import workflow
    from temporalio.exceptions import ApplicationError

    from app.creation.workflow import TextStoryboardWorkflow

    command = new_command()
    calls = AsyncMock(
        side_effect=[ApplicationError("harness_response_unknown", non_retryable=True), None]
    )
    instance = TextStoryboardWorkflow()
    with (
        patch.object(instance, "_activity", calls),
        patch.object(workflow, "wait_condition", AsyncMock(side_effect=asyncio.CancelledError)),
    ):
        with pytest.raises(asyncio.CancelledError):
            await instance.run(command.model_dump(mode="json", by_alias=True))
    assert calls.await_count == 2
    assert calls.await_args_list[1].args == (
        "creation.progress",
        {
            "command_id": command.command_id,
            "stage": "map_manuscript",
            "status": "blocked",
            "last_error": "harness_response_unknown",
            "can_resume": False,
        },
    )


async def test_transport_loss_marks_unknown_and_never_resubmits(repository: Repository) -> None:
    from app.creation.execution import ExecutionStore
    from tests.creation.test_execution import setup_execution

    store, run, task = await setup_execution(repository)
    platform = AsyncMock()
    platform.source.return_value = task.source
    harness = AsyncMock()
    harness.invoke.side_effect = TimeoutError("synthetic connection lost")
    service = CreationActivities(store, platform, harness, task.release_hash, 2)
    with pytest.raises(Exception, match="harness_response_unknown"):
        await service.invoke({"command_id": run, "stage": "map_manuscript"})
    with pytest.raises(Exception, match="invocation_outcome_unknown"):
        await CreationActivities(
            ExecutionStore(repository), platform, harness, task.release_hash, 2
        ).invoke({"command_id": run, "stage": "map_manuscript"})
    assert harness.invoke.await_count == 1


async def test_episode_cannot_bypass_map_gate(repository: Repository) -> None:
    from tests.creation.test_execution import result_for, setup_execution

    store, run, task = await setup_execution(repository)
    lease = await store.reserve(run, task)
    assert isinstance(lease, InvocationLease)
    await store.finish(lease, await result_for(task))
    platform = AsyncMock()
    platform.source.return_value = task.source
    platform.gate.return_value = {"status": "pending"}
    harness = AsyncMock()
    service = CreationActivities(store, platform, harness, task.release_hash, 2)
    with pytest.raises(Exception, match="upstream_gate_pending"):
        await service.invoke(
            {"command_id": run, "stage": "analyze_episode", "episode_key": "episode-one"}
        )
    harness.invoke.assert_not_awaited()
