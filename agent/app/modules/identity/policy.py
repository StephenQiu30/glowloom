from app.modules.identity.contracts import Capability

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
