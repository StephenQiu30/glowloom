import asyncio
from unittest.mock import patch

from fakeredis.aioredis import FakeRedis
from redis.exceptions import TimeoutError

from app.providers.mock import MockProvider, SubmitRequest, TaskRef


def request(request_key: str, **params: object) -> SubmitRequest:
    return SubmitRequest(
        operation_id="op-1",
        request_key=request_key,
        provider_model_id="mock-video",
        capability="video.generate",
        mode="omni_reference",
        params=params,
        inputs=[],
        output_count=1,
    )


def test_duplicate_submit_and_restart_keep_one_task() -> None:
    async def run() -> None:
        now = [1_000]
        async with FakeRedis(decode_responses=True) as store:
            provider = MockProvider(store, now_ms=lambda: now[0])
            req = request(
                "one",
                mock_delay_ms=500,
                mock_result_urls=["https://example.test/result.mp4"],
            )
            submissions = await asyncio.gather(*(provider.submit(req) for _ in range(8)))
            first = submissions[0]

            assert first.outcome == "accepted"
            assert all(item.outcome == "accepted" for item in submissions)
            assert {item.provider_task_id for item in submissions} == {first.provider_task_id}
            conflict = await provider.submit(req.model_copy(update={"operation_id": "op-2"}))
            assert conflict.outcome == "rejected"
            assert conflict.error_code == "request_key_conflict"

            restarted = MockProvider(store, now_ms=lambda: now[0])
            assert (await restarted.query(TaskRef(request_key="one"))).state == "pending"
            now[0] += 500
            result = await restarted.query(TaskRef(provider_task_id=first.provider_task_id))
            assert result.state == "succeeded"
            assert result.result_urls == ["https://example.test/result.mp4"]

    asyncio.run(run())


def test_unknown_submit_is_reconciled_by_request_key() -> None:
    async def run() -> None:
        async with FakeRedis(decode_responses=True) as store:
            provider = MockProvider(store, now_ms=lambda: 1_000)
            submitted = await provider.submit(request("unknown", mock_outcome="unknown"))

            assert submitted.outcome == "unknown"
            assert submitted.provider_task_id is None
            assert (await provider.query(TaskRef(request_key="unknown"))).state == "succeeded"

            duplicate = await provider.submit(request("unknown", mock_outcome="rejected"))
            assert duplicate.outcome == "accepted"
            assert duplicate.provider_task_id is not None

    asyncio.run(run())


def test_rejected_and_not_submitted_never_create_task() -> None:
    async def run() -> None:
        async with FakeRedis(decode_responses=True) as store:
            provider = MockProvider(store)
            rejected = await provider.submit(request("rejected", mock_outcome="rejected"))
            not_submitted = await provider.submit(
                request("not-submitted", mock_outcome="not_submitted")
            )

            assert rejected.outcome == "rejected"
            assert not_submitted.outcome == "not_submitted"
            assert (await provider.query(TaskRef(request_key="rejected"))).state == "not_found"
            assert (await provider.query(TaskRef(request_key="not-submitted"))).state == "not_found"

    asyncio.run(run())


def test_cancel_is_idempotent_and_does_not_return_result() -> None:
    async def run() -> None:
        async with FakeRedis(decode_responses=True) as store:
            provider = MockProvider(store)
            submitted = await provider.submit(request("cancel", mock_delay_ms=500))
            ref = TaskRef(provider_task_id=submitted.provider_task_id)

            assert (await provider.cancel(ref)).cancelled
            assert (await provider.cancel(ref)).cancelled
            result = await provider.query(ref)
            assert result.state == "failed"
            assert result.error_code == "cancelled"
            assert result.result_urls == []

    asyncio.run(run())


def test_failed_and_expired_results_have_explicit_states() -> None:
    async def run() -> None:
        now = [1_000]
        async with FakeRedis(decode_responses=True) as store:
            provider = MockProvider(store, now_ms=lambda: now[0])
            failed = await provider.submit(request("failed", mock_fail=True))
            await provider.submit(request("expired", mock_result_expire_ms=100))

            failed_result = await provider.query(TaskRef(provider_task_id=failed.provider_task_id))
            assert failed_result.state == "failed"
            assert failed_result.error_code == "mock_failure"
            now[0] += 100
            assert (await provider.query(TaskRef(request_key="expired"))).state == "not_found"

    asyncio.run(run())


def test_ambiguous_store_timeout_returns_unknown_and_can_reconcile() -> None:
    async def run() -> None:
        async with FakeRedis(decode_responses=True) as store:
            provider = MockProvider(store)
            original_set = store.set

            async def saved_then_timed_out(name: str, value: str, *, ex: int, nx: bool) -> None:
                await original_set(name, value, ex=ex, nx=nx)
                raise TimeoutError("response lost after store write")

            with patch.object(store, "set", side_effect=saved_then_timed_out):
                result = await provider.submit(request("ambiguous"))

            assert result.outcome == "unknown"
            assert (await provider.query(TaskRef(request_key="ambiguous"))).state == "succeeded"

    asyncio.run(run())
