from typing import Literal, cast

from app.modules.identity.models import Membership, Workspace
from app.modules.identity.workspaces.schemas import WorkspaceResponse


def workspace_response(workspace: Workspace, membership: Membership) -> WorkspaceResponse:
    return WorkspaceResponse(
        id=workspace.id,
        name=workspace.name,
        status=cast(Literal["active", "archived"], workspace.status),
        role=cast(Literal["owner", "editor", "viewer"], membership.role),
        revision=workspace.revision,
    )
