from dataclasses import dataclass
from enum import StrEnum
from uuid import UUID


class Capability(StrEnum):
    WORKSPACE_MANAGE = "workspace:manage"
    BUDGET_MANAGE = "budget:manage"
    CONTENT_READ = "content:read"
    CONTENT_WRITE = "content:write"
    GENERATION_SUBMIT = "generation:submit"
    REVIEW_DECIDE = "review:decide"


@dataclass(frozen=True, slots=True)
class AuthenticatedUser:
    id: UUID


@dataclass(frozen=True, slots=True)
class ActorContext:
    user_id: UUID
    workspace_id: UUID
    membership_id: UUID
    role: str
    workspace_status: str
