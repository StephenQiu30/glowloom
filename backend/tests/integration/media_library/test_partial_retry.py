from __future__ import annotations

import json

import pytest

from db.pool import DatabasePool
from integrations.ai.deterministic_media import (
    DeterministicImageProvider,
    GeneratedMedia,
)
from integrations.ai.registry import create_mvp_registry
from services.media_generation import GenerateMediaCommand, GenerateMediaHandler
from services.storyboards import ConfirmStoryboardCommand, ConfirmStoryboardHandler
from services.tasks import RetryTaskHandler, TaskQueryService
from tests.integration.media_library.support import media_job_handler, run_media_job
from tests.integration.story_development.support import storyboard_draft
from workers.media_provider import RetryableMediaProviderError


class FailThirdImageOnce:
    def __init__(self) -> None:
        self._delegate = DeterministicImageProvider()
        self.input_hashes: list[str] = []
        self.failed = False

    async def generate(self, input_hash: str, output_slot: str) -> GeneratedMedia:
        self.input_hashes.append(input_hash)
        if len(self.input_hashes) == 3 and not self.failed:
            self.failed = True
            raise RetryableMediaProviderError("temporary provider failure")
        return await self._delegate.generate(input_hash, output_slot)


@pytest.mark.asyncio
async def test_third_shot_failure_retries_only_its_slot(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=4)
    await database.start()
    try:
        episode_id, generated = await storyboard_draft(database, "partial:third-shot")
        confirmed = await ConfirmStoryboardHandler(database).execute(
            ConfirmStoryboardCommand(
                generated.storyboard.id, generated.storyboard.resource_version
            )
        )
        shots = confirmed.storyboard.content.shots[:3]
        submit = GenerateMediaHandler(database, release_version="test-release")
        accepted = [
            await submit.execute(
                GenerateMediaCommand(
                    episode_id,
                    "shot_image",
                    shot.shot_id,
                    confirmed.storyboard.id,
                    f"partial:shot:{shot.ordinal}",
                )
            )
            for shot in shots
        ]
        provider = FailThirdImageOnce()
        registry = create_mvp_registry(
            {("image", "mock"): lambda _profile: provider}
        )
        jobs = media_job_handler(database, registry=registry)

        for task in accepted:
            await run_media_job(database, task.task_id, jobs)

        tasks = [
            await TaskQueryService(database).get(task.task_id) for task in accepted
        ]
        assert [task.status for task in tasks] == ["succeeded", "succeeded", "failed"]
        assert tasks[2].error_code == "PROVIDER_TEMPORARY"
        assert tasks[2].error == {
            "retryable": True,
            "summary": "Media provider failed temporarily",
        }

        async with database.transaction() as connection:
            before = await connection.fetch(
                """
                SELECT candidate.task_id,version.sha256
                FROM generation_candidates candidate
                JOIN media_versions version ON version.id=candidate.media_version_id
                WHERE candidate.task_id=ANY($1::uuid[])
                ORDER BY candidate.task_id
                """,
                [task.task_id for task in accepted],
            )
            frozen = await connection.fetch(
                """
                SELECT task.id,snapshot.input_refs_json
                FROM production_tasks task
                JOIN submission_snapshots snapshot ON snapshot.id=task.snapshot_id
                WHERE task.id=ANY($1::uuid[])
                """,
                [task.task_id for task in accepted],
            )
        frozen_hashes = {
            row["id"]: json.loads(row["input_refs_json"])["input_hash"]
            for row in frozen
        }
        assert len(before) == 2

        retry = await RetryTaskHandler(
            database, release_version="test-release"
        ).execute(
            accepted[2].task_id,
            tasks[2].resource_version,
            "partial:retry:third-shot",
        )
        await run_media_job(database, retry.task_id, jobs)

        retried = await TaskQueryService(database).get(retry.task_id)
        assert retried.status == "succeeded"
        async with database.transaction() as connection:
            after = await connection.fetch(
                """
                SELECT candidate.task_id,version.sha256
                FROM generation_candidates candidate
                JOIN media_versions version ON version.id=candidate.media_version_id
                WHERE candidate.episode_id=$1 ORDER BY candidate.task_id
                """,
                episode_id,
            )
            retry_row = await connection.fetchrow(
                """
                SELECT task.retry_of_task_id,snapshot.input_refs_json
                FROM production_tasks task
                JOIN submission_snapshots snapshot ON snapshot.id=task.snapshot_id
                WHERE task.id=$1
                """,
                retry.task_id,
            )
        before_by_task = {row["task_id"]: row["sha256"] for row in before}
        after_by_task = {row["task_id"]: row["sha256"] for row in after}
        assert {key: after_by_task[key] for key in before_by_task} == before_by_task
        assert retry_row["retry_of_task_id"] == accepted[2].task_id
        assert json.loads(retry_row["input_refs_json"])["input_hash"] == frozen_hashes[
            accepted[2].task_id
        ]
        assert provider.input_hashes == [
            frozen_hashes[accepted[0].task_id],
            frozen_hashes[accepted[1].task_id],
            frozen_hashes[accepted[2].task_id],
            frozen_hashes[accepted[2].task_id],
        ]
    finally:
        await database.close()
