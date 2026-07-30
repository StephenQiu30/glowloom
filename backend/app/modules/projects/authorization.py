from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.identity import ActorContext, Capability, actor_context
from app.modules.projects import repository
from app.modules.projects.models import Project


def project_not_found() -> ApiError:
    return ApiError(ErrorCode.NOT_FOUND, "Project not found", status_code=404)


def require_project_revision(project: Project, expected: int) -> None:
    if project.revision != expected:
        raise ApiError(
            ErrorCode.VERSION_CONFLICT,
            "Project has changed",
            status_code=409,
            details={"current_revision": project.revision},
        )


def require_project_state(project: Project, capability: Capability) -> None:
    if project.status == "archived" and capability != Capability.CONTENT_READ:
        raise ApiError(
            ErrorCode.STATE_CONFLICT,
            "Project is archived",
            status_code=409,
            next_action="restore_project",
        )


async def owned_project(
    session: AsyncSession,
    claims: AccessTokenClaims,
    project_id: UUID,
    capability: Capability,
    *,
    for_update: bool = False,
    allow_archived_command: bool = False,
) -> tuple[Project, ActorContext]:
    project = await repository.find_project(session, project_id, for_update=for_update)
    if project is None:
        raise project_not_found()
    try:
        actor = await actor_context(session, claims, project.workspace_id, capability)
    except ApiError as error:
        if error.code == ErrorCode.NOT_FOUND:
            raise project_not_found() from error
        raise
    if not allow_archived_command:
        require_project_state(project, capability)
    return project, actor
