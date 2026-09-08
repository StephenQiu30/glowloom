"""Freeze a bounded text plan with its execution policy; never regenerate history."""

from typing import Annotated, Any, Literal

from psycopg import AsyncConnection
from psycopg.types.json import Jsonb
from pydantic import Field

from app.creation.contract import Command, ContentHash, Identifier, StrictModel
from app.protocol.canonical import canonical_hash
from app.text_contract.task import Stage


class ManifestConflict(RuntimeError):
    pass


class StageDescriptor(StrictModel):
    step_key: Stage
    title: str
    scope: Literal["run", "episode_collection", "scene_collection"]
    review_required: Literal[True] = True
    review_key: str
    instance_count: None = None


class PlanTemplate(StrictModel):
    version: Literal[1] = 1
    flow_type: Literal["lanverse.creation.text-storyboard.production"] = (
        "lanverse.creation.text-storyboard.production"
    )
    stages: Annotated[list[StageDescriptor], Field(min_length=4, max_length=4)]


class ExecutionManifest(StrictModel):
    version: Literal[1] = 1
    command_id: Identifier
    run_id: Identifier
    payload_hash: ContentHash
    source_revision_id: Identifier
    source_content_hash: ContentHash
    release_hash: ContentHash
    call_limit: Annotated[int, Field(ge=1, le=1000)]
    template: PlanTemplate
    template_hash: ContentHash


def build_template() -> PlanTemplate:
    return PlanTemplate(
        stages=[
            StageDescriptor(
                step_key="map_manuscript",
                title="分集确认",
                scope="run",
                review_key="manuscript_review",
            ),
            StageDescriptor(
                step_key="analyze_episode",
                title="逐集解析",
                scope="episode_collection",
                review_key="episode_review",
            ),
            StageDescriptor(
                step_key="build_world", title="制作总册", scope="run", review_key="world_review"
            ),
            StageDescriptor(
                step_key="direct_scene",
                title="逐场导演",
                scope="scene_collection",
                review_key="scene_review",
            ),
        ]
    )


async def save_manifest(
    conn: AsyncConnection[dict[str, Any]], command_id: str, release_hash: str, call_limit: int
) -> None:
    row = await (
        await conn.execute(
            "SELECT command, payload_hash FROM creation_commands WHERE command_id = %s",
            (command_id,),
        )
    ).fetchone()
    if row is None:
        raise ManifestConflict("creation_manifest_conflict")
    command = Command.model_validate(row["command"])
    if command.command_id != command_id or command.payload_hash != row["payload_hash"]:
        raise ManifestConflict("creation_manifest_conflict")
    template = build_template()
    manifest = ExecutionManifest(
        command_id=command_id,
        run_id=command.run_id,
        payload_hash=command.payload_hash,
        source_revision_id=command.source.revision_id,
        source_content_hash=command.source.content_hash,
        release_hash=release_hash,
        call_limit=call_limit,
        template=template,
        template_hash=canonical_hash(template.model_dump(mode="json")),
    ).model_dump(mode="json")
    await conn.execute(
        "INSERT INTO creation_manifests (command_id, manifest, manifest_hash) VALUES (%s, %s, %s)",
        (command_id, Jsonb(manifest), canonical_hash(manifest)),
    )


async def read_manifest(
    conn: AsyncConnection[dict[str, Any]], command_id: str
) -> dict[str, Any] | None:
    # A single SELECT gives a consistent view of command, policy and immutable plan.
    row = await (
        await conn.execute(
            """SELECT c.command, c.payload_hash, e.command_id AS execution_id,
               e.release_hash, e.call_limit, m.manifest, m.manifest_hash
               FROM creation_commands c
               LEFT JOIN creation_executions e USING (command_id)
               LEFT JOIN creation_manifests m USING (command_id)
               WHERE c.command_id = %s""",
            (command_id,),
        )
    ).fetchone()
    if row is None:
        return None
    try:
        command = Command.model_validate(row["command"])
        if command.command_id != command_id or command.payload_hash != row["payload_hash"]:
            raise ValueError("command identity drift")
        if row["manifest"] is not None:
            plan = ExecutionManifest.model_validate(row["manifest"])
            if (
                canonical_hash(row["manifest"]) != row["manifest_hash"]
                or canonical_hash(plan.template.model_dump(mode="json")) != plan.template_hash
                or plan.command_id != command_id
                or plan.run_id != command.run_id
                or plan.payload_hash != command.payload_hash
                or plan.source_revision_id != command.source.revision_id
                or plan.source_content_hash != command.source.content_hash
                or plan.release_hash != row["release_hash"]
                or plan.call_limit != row["call_limit"]
            ):
                raise ValueError("manifest identity drift")
    except ValueError as error:
        raise ManifestConflict("creation_manifest_conflict") from error
    return {
        "schema": "creation-manifest-production",
        "command_id": command_id,
        "run_id": command.run_id,
        "availability": (
            "recorded"
            if row["manifest"] is not None
            else "unavailable"
            if row["execution_id"] is not None
            else "not_frozen"
        ),
        "manifest": row["manifest"],
        "manifest_hash": row["manifest_hash"],
    }
