"""Public governance contracts and rights-gate use cases."""

from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.governance.contracts import (
    RightsBlocker,
    RightsGateResult,
    RightsUsage,
    SubjectReference,
    SubjectType,
)


async def check_rights(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    subject: SubjectReference,
    usage: RightsUsage,
) -> RightsGateResult:
    from app.modules.governance.service import check_rights as implementation

    return await implementation(
        session,
        workspace_id=workspace_id,
        subject=subject,
        usage=usage,
    )


async def check_rights_for_resolved_subjects(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    subjects: list[SubjectReference],
    usage: RightsUsage,
) -> dict[SubjectReference, RightsGateResult]:
    from app.modules.governance.service import (
        check_rights_for_resolved_subjects as implementation,
    )

    return await implementation(
        session,
        workspace_id=workspace_id,
        subjects=subjects,
        usage=usage,
    )


__all__ = [
    "RightsBlocker",
    "RightsGateResult",
    "RightsUsage",
    "SubjectReference",
    "SubjectType",
    "check_rights",
    "check_rights_for_resolved_subjects",
]
