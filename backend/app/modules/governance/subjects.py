from uuid import UUID

from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy.ext.asyncio import AsyncSession

from app.core.errors import ApiError, ErrorCode
from app.modules.governance.contracts import SubjectReference, SubjectType


def _dependency_unavailable(subject_type: SubjectType, *, retryable: bool) -> ApiError:
    return ApiError(
        ErrorCode.DEPENDENCY_UNAVAILABLE,
        "Subject resolver is unavailable",
        status_code=503,
        next_action="retry" if retryable else None,
        details={"subject_type": subject_type, "retryable": retryable},
    )


async def resolve_subject(
    session: AsyncSession,
    workspace_id: UUID,
    subject: SubjectReference,
) -> None:
    try:
        if subject.subject_type is SubjectType.MEDIA_VERSION:
            from app.modules.media import media_version_exists

            exists = await media_version_exists(
                session, workspace_id, subject.subject_id
            )
        elif subject.subject_type is SubjectType.SCRIPT_VERSION:
            from app.modules.scripts import script_version_exists

            exists = await script_version_exists(
                session, workspace_id, subject.subject_id
            )
        elif subject.subject_type is SubjectType.ASSET_VERSION:
            # Resolve lazily to avoid an initialization cycle: assets consumes
            # the public RightsGate while governance resolves asset subjects.
            from app.modules.assets import asset_version_exists

            exists = await asset_version_exists(
                session, workspace_id, subject.subject_id
            )
        else:
            raise _dependency_unavailable(subject.subject_type, retryable=False)
    except ApiError:
        raise
    except SQLAlchemyError as error:
        raise _dependency_unavailable(subject.subject_type, retryable=True) from error

    if not exists:
        raise ApiError(ErrorCode.NOT_FOUND, "Subject not found", status_code=404)
