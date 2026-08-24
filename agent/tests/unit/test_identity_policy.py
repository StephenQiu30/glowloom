import pytest

from app.modules.identity.policy import (
    Capability,
    require_capability,
    require_workspace_capability,
)


@pytest.mark.parametrize(
    ("role", "allowed", "denied"),
    [
        ("owner", Capability.WORKSPACE_MANAGE, None),
        ("editor", Capability.CONTENT_WRITE, Capability.WORKSPACE_MANAGE),
        ("viewer", Capability.CONTENT_READ, Capability.CONTENT_WRITE),
    ],
)
def test_role_capability_matrix(
    role: str,
    allowed: Capability,
    denied: Capability | None,
) -> None:
    require_capability(role, allowed)
    if denied is not None:
        with pytest.raises(PermissionError):
            require_capability(role, denied)


def test_archived_workspace_allows_history_but_rejects_new_writes() -> None:
    require_workspace_capability("owner", "archived", Capability.CONTENT_READ)
    with pytest.raises(PermissionError):
        require_workspace_capability("owner", "archived", Capability.CONTENT_WRITE)
    with pytest.raises(PermissionError):
        require_workspace_capability("owner", "archived", Capability.WORKSPACE_MANAGE)
