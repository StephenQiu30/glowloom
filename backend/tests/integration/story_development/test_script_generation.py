from __future__ import annotations

import json
from uuid import UUID

import pytest
from pydantic import ValidationError

from db.pool import DatabasePool
from integrations.ai.deterministic_text import (
    DeterministicTextProvider,
)
from integrations.ai.registry import (
    AiModelProfile,
    AiModelRegistry,
    ProfileNotFound,
)
from services.projects import (
    CreateProjectCommand,
    CreateProjectHandler,
)
from services.sources import (
    ConfirmSourceCommand,
    ConfirmSourceHandler,
    CreateSourceRevisionCommand,
    CreateSourceRevisionHandler,
)
from services.story_generation import (
    GenerateScriptCommand,
    GenerateScriptHandler,
)
from services.story_results import ScriptResultRegistrar


async def confirmed_source(database: DatabasePool) -> tuple[UUID, UUID]:
    project = await CreateProjectHandler(database).execute(
        CreateProjectCommand(title="剧本生成", idempotency_key="script:project:0001")
    )
    source = await CreateSourceRevisionHandler(database).execute(
        CreateSourceRevisionCommand(
            episode_id=project.episode.id,
            content="汉字故事" + "a" * 296,
            rights_basis="original",
            parent_id=None,
            idempotency_key="script:source:00001",
        )
    )
    confirmed = await ConfirmSourceHandler(database).execute(
        ConfirmSourceCommand(source.id, source.resource_version)
    )
    return project.episode.id, confirmed.id


@pytest.mark.asyncio
async def test_generate_script_freezes_input_and_registration_is_idempotent(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=4)
    await database.start()
    try:
        episode_id, source_id = await confirmed_source(database)
        accepted = await GenerateScriptHandler(database, release_version="test-release").execute(
            GenerateScriptCommand(episode_id, "script:generate:001")
        )
        provider = DeterministicTextProvider()
        first_output = await provider.generate_script(source_id, "第一集")
        second_output = await provider.generate_script(source_id, "第一集")
        assert first_output == second_output
        assert provider.call_count == 2

        registrar = ScriptResultRegistrar(database)
        first = await registrar.register(accepted.task_id, first_output)
        replay = await registrar.register(accepted.task_id, second_output)

        assert first.id == replay.id
        assert first.status == "draft"
        assert first.source_revision_id == source_id
        assert len(first.content.scenes) == 6
        assert first.content_hash == replay.content_hash
        async with database.transaction() as connection:
            snapshot = await connection.fetchrow(
                "SELECT * FROM submission_snapshots WHERE id=$1", accepted.snapshot_id
            )
            counts = await connection.fetchrow(
                "SELECT (SELECT count(*) FROM script_versions) scripts, "
                "(SELECT count(*) FROM task_outputs) outputs"
            )
        assert snapshot is not None
        assert json.loads(snapshot["input_refs_json"]) == {"source_revision_id": str(source_id)}
        assert snapshot["model_profile_id"] == "mock-text-v1"
        assert snapshot["schema_version"] == "script-v1"
        assert counts is not None and tuple(counts.values()) == (1, 1)
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_invalid_text_output_does_not_create_a_script_draft(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    try:
        episode_id, _ = await confirmed_source(database)
        accepted = await GenerateScriptHandler(database, release_version="test-release").execute(
            GenerateScriptCommand(episode_id, "script:generate:002")
        )

        with pytest.raises(ValidationError):
            await ScriptResultRegistrar(database).register(
                accepted.task_id, '{"schema_version":"script-v1","unexpected":true}'
            )

        async with database.transaction() as connection:
            assert await connection.fetchval("SELECT count(*) FROM script_versions") == 0
            assert await connection.fetchval("SELECT count(*) FROM task_outputs") == 0
    finally:
        await database.close()


@pytest.mark.asyncio
async def test_script_submission_freezes_the_exact_registry_profile_without_fallback(
    migrated_database_url: str,
) -> None:
    database = DatabasePool(migrated_database_url, min_size=1, max_size=2)
    await database.start()
    registry = AiModelRegistry(
        [
            AiModelProfile(
                profile_id="approved-text-v2",
                capability="text",
                provider_id="approved-provider",
                model_id="approved-model-v2",
                route_version="approved-route-v2",
                schema_versions=frozenset({"script-v1"}),
                parameters={"temperature": 0.2, "response_format": "json"},
                kind="provider",
                credential_env_names=("APPROVED_PROVIDER_API_KEY",),
            )
        ],
        defaults={"text": "approved-text-v2"},
    )
    try:
        episode_id, _ = await confirmed_source(database)
        handler = GenerateScriptHandler(
            database,
            release_version="test-release",
            registry=registry,
        )
        accepted = await handler.execute(
            GenerateScriptCommand(
                episode_id,
                "script:profile:0001",
                model_profile_id="approved-text-v2",
            )
        )
        with pytest.raises(ProfileNotFound):
            await handler.execute(
                GenerateScriptCommand(
                    episode_id,
                    "script:profile:0002",
                    model_profile_id="missing-profile",
                )
            )

        async with database.transaction() as connection:
            snapshot = await connection.fetchrow(
                "SELECT * FROM submission_snapshots WHERE id=$1",
                accepted.snapshot_id,
            )
            task_count = await connection.fetchval("SELECT count(*) FROM production_tasks")
        assert snapshot is not None
        assert snapshot["model_profile_id"] == "approved-text-v2"
        assert snapshot["provider_id"] == "approved-provider"
        assert snapshot["model_id"] == "approved-model-v2"
        assert snapshot["route_version"] == "approved-route-v2"
        assert json.loads(snapshot["parameters_json"]) == {
            "temperature": 0.2,
            "response_format": "json",
        }
        assert task_count == 1
    finally:
        await database.close()
