from __future__ import annotations

import asyncio
import time
from typing import Any
from uuid import uuid4

import httpx
import pytest

from app.creation.api import create_app
from app.creation.execution import ExecutionConflict, ExecutionStore
from app.creation.repository import Repository, SchemaMismatch
from app.protocol.canonical import canonical_hash
from tests.creation.test_contract import SECRET, authorization
from tests.creation.test_execution import setup_execution
from tests.creation.test_repository import new_command


async def read_manifest(repository: Repository, command_id: str) -> dict[str, Any]:
    from app.creation.manifest import read_manifest as read

    async with await repository.connect() as conn:
        value = await read(conn, command_id)
        assert value is not None
        return value


async def test_freeze_records_plan_before_any_attempt(repository: Repository) -> None:
    store, run, task = await setup_execution(repository)
    value = await read_manifest(repository, run)
    assert value["availability"] == "recorded"
    manifest = value["manifest"]
    assert value["manifest_hash"] == canonical_hash(manifest)
    assert manifest["command_id"] == manifest["run_id"] == run
    assert manifest["release_hash"] == task.release_hash and manifest["call_limit"] == 2
    assert manifest["source_revision_id"] == task.source.revision_id
    assert manifest["source_content_hash"] == task.source.content_hash
    command = await repository.command(run)
    assert command and manifest["payload_hash"] == command.payload_hash
    template = manifest["template"]
    assert manifest["template_hash"] == canonical_hash(template)
    assert [stage["step_key"] for stage in template["stages"]] == [
        "map_manuscript",
        "analyze_episode",
        "build_world",
        "direct_scene",
    ]
    assert all(stage["review_required"] for stage in template["stages"])
    assert [stage["scope"] for stage in template["stages"]] == [
        "run",
        "episode_collection",
        "run",
        "scene_collection",
    ]
    assert all(stage["instance_count"] is None for stage in template["stages"])
    assert (await store.snapshot(run))["reserved_calls"] == 0
    async with await repository.connect() as conn:
        row = await (await conn.execute("SELECT count(*) AS n FROM creation_steps")).fetchone()
        assert row and row["n"] == 0


async def test_parallel_freeze_and_replay_ignore_new_template(
    repository: Repository, monkeypatch: pytest.MonkeyPatch
) -> None:
    from app.creation import manifest

    command = new_command()
    await repository.accept(command, "text-test")
    store = ExecutionStore(repository)
    await asyncio.gather(*(store.freeze(command.run_id, "a" * 64, 4) for _ in range(8)))
    before = await read_manifest(repository, command.run_id)

    def changed_template() -> dict[str, Any]:
        raise AssertionError("replay must not compile today's template")

    monkeypatch.setattr(manifest, "build_template", changed_template)
    await ExecutionStore(Repository(repository.dsn)).freeze(command.run_id, "a" * 64, 4)
    assert await read_manifest(repository, command.run_id) == before
    with pytest.raises(ExecutionConflict, match="execution_policy_conflict"):
        await store.freeze(command.run_id, "b" * 64, 4)
    async with await repository.connect() as conn:
        row = await (await conn.execute("SELECT count(*) AS n FROM creation_manifests")).fetchone()
        assert row and row["n"] == 1


async def test_manifest_failure_rolls_back_policy(repository: Repository) -> None:
    command = new_command()
    await repository.accept(command, "text-test")
    async with await repository.connect() as conn:
        await conn.execute("ALTER TABLE creation_manifests ADD CONSTRAINT injected CHECK (false)")
    try:
        with pytest.raises(Exception, match="injected"):
            await ExecutionStore(repository).freeze(command.run_id, "a" * 64, 2)
        async with await repository.connect() as conn:
            row = await (
                await conn.execute("SELECT count(*) AS n FROM creation_executions")
            ).fetchone()
            assert row and row["n"] == 0
    finally:
        async with await repository.connect() as conn:
            await conn.execute("ALTER TABLE creation_manifests DROP CONSTRAINT injected")
    await ExecutionStore(repository).freeze(command.run_id, "a" * 64, 2)


async def test_manifest_is_immutable_and_drift_fails_closed(repository: Repository) -> None:
    from app.creation.manifest import ManifestConflict

    _, run, task = await setup_execution(repository)
    with pytest.raises(Exception, match="manifest is immutable"):
        async with await repository.connect() as conn:
            await conn.execute("UPDATE creation_manifests SET manifest_hash = %s", ("b" * 64,))
    with pytest.raises(Exception, match="manifest is immutable"):
        async with await repository.connect() as conn:
            await conn.execute("DELETE FROM creation_manifests")
    # Simulate corrupted storage only in this dedicated test database.
    async with await repository.connect() as conn:
        await conn.execute(
            "ALTER TABLE creation_manifests DISABLE TRIGGER creation_manifest_immutable"
        )
        await conn.execute("UPDATE creation_manifests SET manifest_hash = %s", ("b" * 64,))
        await conn.execute(
            "ALTER TABLE creation_manifests ENABLE TRIGGER creation_manifest_immutable"
        )
    with pytest.raises(ManifestConflict):
        await read_manifest(repository, run)
    with pytest.raises(ManifestConflict):
        await ExecutionStore(repository).freeze(run, task.release_hash, 2)


async def test_signed_manifest_availability_and_scope(repository: Repository) -> None:
    command = new_command()
    await repository.accept(command, "text-test")
    app = create_app(repository, SECRET, "text-test")
    path = f"/internal/creation/commands/{command.run_id}/manifest"

    def headers(target: str = path, body: bytes = b"") -> dict[str, str]:
        return {
            "X-Lanverse-Creation-Authorization": authorization(
                body, path=target, method="GET", expires=int(time.time()) + 60
            )
        }

    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=app), base_url="http://test"
    ) as client:
        before = await client.get(path, headers=headers())
        assert before.status_code == 200
        assert before.json()["availability"] == "not_frozen" and before.json()["manifest"] is None
        await ExecutionStore(repository).freeze(command.run_id, "a" * 64, 2)
        response = await client.get(path, headers=headers())
        assert response.status_code == 200 and response.json()["availability"] == "recorded"
        assert response.json()["schema"] == "creation-manifest-production"
        assert (await client.get(path)).status_code == 401
        assert (await client.get(path + "?x=1", headers=headers())).status_code == 401
        other = f"/internal/creation/commands/{uuid4()}/manifest"
        assert (await client.get(other, headers=headers())).status_code == 401
        assert (await client.get(other, headers=headers(other))).status_code == 404
        invalid = "/internal/creation/commands/invalid/manifest"
        assert (await client.get(invalid, headers=headers(invalid))).status_code == 422
        assert (
            await client.request("GET", path, content=b"{}", headers=headers(body=b"{}"))
        ).status_code == 422
        assert not any(
            key in response.json()["manifest"]
            for key in ("source", "prompt", "result", "candidate")
        )


async def test_upgrade_does_not_invent_legacy_manifests(repository: Repository) -> None:
    store, run, task = await setup_execution(repository)
    async with await repository.connect() as conn:
        checksums = await (
            await conn.execute(
                "SELECT name, checksum FROM creation_schema "
                "WHERE name <> 'text-manifest' ORDER BY name"
            )
        ).fetchall()
        await conn.execute("DROP TABLE creation_manifests")
        await conn.execute("DROP FUNCTION creation_guard_manifest()")
        await conn.execute("DELETE FROM creation_schema WHERE name = 'text-manifest'")
    with pytest.raises(SchemaMismatch, match="manifest migration"):
        await repository.ready()
    await repository.migrate()
    await repository.migrate()
    await repository.ready()
    await store.freeze(run, task.release_hash, 2)
    legacy = await read_manifest(repository, run)
    assert legacy["availability"] == "unavailable" and legacy["manifest"] is None
    async with await repository.connect() as conn:
        assert (
            await (
                await conn.execute(
                    "SELECT name, checksum FROM creation_schema "
                    "WHERE name <> 'text-manifest' ORDER BY name"
                )
            ).fetchall()
            == checksums
        )


async def test_manifest_checksum_drift_is_not_repaired(repository: Repository) -> None:
    from app.creation.manifest_schema import MANIFEST_SCHEMA_HASH

    async with await repository.connect() as conn:
        await conn.execute("UPDATE creation_schema SET checksum='drift' WHERE name='text-manifest'")
    try:
        with pytest.raises(SchemaMismatch, match="manifest migration"):
            await repository.ready()
        with pytest.raises(SchemaMismatch, match="manifest migration"):
            await repository.migrate()
    finally:
        async with await repository.connect() as conn:
            await conn.execute(
                "UPDATE creation_schema SET checksum=%s WHERE name='text-manifest'",
                (MANIFEST_SCHEMA_HASH,),
            )


@pytest.mark.parametrize("field", ["source_revision_id", "call_limit", "template_hash"])
async def test_rehashed_manifest_cannot_change_bound_inputs(
    repository: Repository, field: str
) -> None:
    from psycopg.types.json import Jsonb

    from app.creation.manifest import ManifestConflict

    _, run, _ = await setup_execution(repository)
    saved = (await read_manifest(repository, run))["manifest"]
    saved[field] = (
        3 if field == "call_limit" else str(uuid4()) if field == "source_revision_id" else "b" * 64
    )
    async with await repository.connect() as conn:
        await conn.execute(
            "ALTER TABLE creation_manifests DISABLE TRIGGER creation_manifest_immutable"
        )
        await conn.execute(
            "UPDATE creation_manifests SET manifest=%s, manifest_hash=%s WHERE command_id=%s",
            (Jsonb(saved), canonical_hash(saved), run),
        )
        await conn.execute(
            "ALTER TABLE creation_manifests ENABLE TRIGGER creation_manifest_immutable"
        )
    with pytest.raises(ManifestConflict):
        await read_manifest(repository, run)
    path = f"/internal/creation/commands/{run}/manifest"
    token = authorization(b"", path=path, method="GET", expires=int(time.time()) + 60)
    async with httpx.AsyncClient(
        transport=httpx.ASGITransport(app=create_app(repository, SECRET, "text-test")),
        base_url="http://test",
    ) as client:
        response = await client.get(path, headers={"X-Lanverse-Creation-Authorization": token})
    assert response.status_code == 409
    assert response.json() == {"detail": "creation_manifest_conflict"}
