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


_ROLE_CAPABILITIES: dict[str, frozenset[Capability]] = {
    "owner": frozenset(Capability),
    "editor": frozenset(
        {
            Capability.CONTENT_READ,
            Capability.CONTENT_WRITE,
            Capability.GENERATION_SUBMIT,
            Capability.REVIEW_DECIDE,
        }
    ),
    "viewer": frozenset({Capability.CONTENT_READ}),
}


@dataclass(frozen=True, slots=True)
class ActorContext:
    user_id: UUID
    workspace_id: UUID
    membership_id: UUID
    role: str
    workspace_status: str


def require_capability(role: str, capability: Capability) -> None:
    if capability not in _ROLE_CAPABILITIES.get(role, frozenset()):
        raise PermissionError(capability)


def require_workspace_capability(
    role: str,
    workspace_status: str,
    capability: Capability,
) -> None:
    require_capability(role, capability)
    if workspace_status == "archived" and capability != Capability.CONTENT_READ:
        raise PermissionError(capability)
