from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from app.core.auth import AccessTokenClaims
from app.core.errors import ApiError, ErrorCode
from app.modules.identity import Capability, actor_context


def resource_not_found(resource: str) -> ApiError:
    return ApiError(ErrorCode.NOT_FOUND, f"{resource} not found", status_code=404)


async def require_resource_access(
    session: AsyncSession,
    claims: AccessTokenClaims,
    workspace_id: UUID,
    resource: str,
) -> None:
    try:
        await actor_context(
            session,
            claims,
            workspace_id,
            Capability.CONTENT_READ,
        )
    except ApiError as error:
        if error.code == ErrorCode.NOT_FOUND:
            raise resource_not_found(resource) from error
        raise
