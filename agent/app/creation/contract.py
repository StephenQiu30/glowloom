from __future__ import annotations

import json
from datetime import datetime
from typing import Annotated, Any, Literal, Self, cast
from uuid import UUID

from pydantic import AfterValidator, AwareDatetime, BaseModel, ConfigDict, Field, model_validator

from app.protocol.canonical import production_canonical_hash

FLOW_TYPE = "lanverse.creation.text-storyboard.production"


def canonical_uuid(value: str) -> str:
    parsed = UUID(value)
    if str(parsed) != value or parsed.int == 0:
        raise ValueError("expected a non-nil canonical UUID")
    return value


Identifier = Annotated[str, AfterValidator(canonical_uuid)]
ContentHash = Annotated[str, Field(pattern=r"^[0-9a-f]{64}$")]


class StrictModel(BaseModel):
    model_config = ConfigDict(extra="forbid", strict=True, frozen=True)


class Source(StrictModel):
    document_id: Identifier
    revision_id: Identifier
    revision: Annotated[int, Field(ge=1, le=2**63 - 1)]
    content_hash: ContentHash
    span_index_id: Identifier


class Command(StrictModel):
    schema_: Literal["creation-command-production"] = Field(alias="schema")
    command_id: Identifier
    run_id: Identifier
    workspace_id: Identifier
    project_id: Identifier
    actor_id: Identifier
    source: Source
    flow_type: Literal["lanverse.creation.text-storyboard.production"]
    workflow_id: str

    @model_validator(mode="after")
    def check_identity(self) -> Self:
        if self.command_id != self.run_id or self.workflow_id != f"lanverse:creation:{self.run_id}":
            raise ValueError("creation command identity mismatch")
        return self

    @property
    def payload_hash(self) -> str:
        return production_canonical_hash(self.model_dump(mode="json", by_alias=True))


class Acceptance(StrictModel):
    schema_: Literal["creation-acceptance-production"] = Field(
        default="creation-acceptance-production", alias="schema"
    )
    command_id: Identifier
    run_id: Identifier
    payload_hash: ContentHash
    flow_type: Literal["lanverse.creation.text-storyboard.production"]
    workflow_id: str
    receipt_id: Identifier
    accepted_at: AwareDatetime

    @classmethod
    def for_command(cls, command: Command, receipt_id: str, accepted_at: datetime) -> Acceptance:
        return cls(
            command_id=command.command_id,
            run_id=command.run_id,
            payload_hash=command.payload_hash,
            flow_type=command.flow_type,
            workflow_id=command.workflow_id,
            receipt_id=receipt_id,
            accepted_at=accepted_at,
        )


def decode_object(raw: bytes) -> dict[str, Any]:
    def pairs(items: list[tuple[str, Any]]) -> dict[str, Any]:
        result: dict[str, Any] = {}
        for key, value in items:
            if key in result:
                raise ValueError("duplicate JSON key")
            result[key] = value
        return result

    def invalid_constant(value: str) -> Any:
        raise ValueError("invalid JSON constant")

    value: Any = json.loads(raw, object_pairs_hook=pairs, parse_constant=invalid_constant)
    if not isinstance(value, dict):
        raise ValueError("expected JSON object")
    return cast(dict[str, Any], value)
