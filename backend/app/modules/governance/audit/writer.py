from datetime import UTC, datetime
from typing import Any, Literal
from uuid import UUID

from sqlalchemy.ext.asyncio import AsyncSession

from app.modules.governance.audit.models import AuditEvent

_AUDIT_METADATA_FIELDS: dict[str, frozenset[str]] = {
    "identity.registered": frozenset({"token_version", "workspace_revision"}),
    "identity.login_succeeded": frozenset({"token_version"}),
    "identity.logged_out": frozenset(
        {"previous_token_version", "token_version"}
    ),
    "identity.password_changed": frozenset(
        {"previous_token_version", "token_version"}
    ),
    "identity.profile_updated": frozenset({"changed_fields"}),
    "identity.account_deactivated": frozenset(
        {
            "previous_status",
            "status",
            "previous_token_version",
            "token_version",
        }
    ),
    "workspace.created": frozenset({"revision", "status"}),
    "workspace.updated": frozenset({"revision", "changed_fields"}),
    "workspace.archived": frozenset({"revision", "previous_status", "status"}),
    "workspace.restored": frozenset({"revision", "previous_status", "status"}),
    "consent.registered": frozenset({"revision", "subject_type"}),
    "consent.revised": frozenset({"revision", "subject_type"}),
    "consent.revoked": frozenset({"revision", "subject_type"}),
    "media.version_created": frozenset(
        {"revision", "version_id", "version_no", "kind", "source_type"}
    ),
    "media.current_changed": frozenset(
        {"revision", "previous_version_id", "current_version_id"}
    ),
    "media.archived": frozenset({"revision", "current_version_id"}),
    "media.restored": frozenset({"revision", "current_version_id"}),
    "task.created": frozenset(
        {"revision", "task_type", "request_type", "request_id"}
    ),
}


def append_audit_event(
    session: AsyncSession,
    *,
    workspace_id: UUID,
    actor_id: UUID,
    action: str,
    target_type: str,
    target_id: UUID,
    trace_id: str,
    metadata: dict[str, Any],
    result: Literal["succeeded", "denied", "failed"] = "succeeded",
    occurred_at: datetime | None = None,
) -> AuditEvent:
    allowed_fields = _AUDIT_METADATA_FIELDS.get(action)
    if allowed_fields is None:
        raise ValueError(f"Audit action is not registered: {action}")
    unexpected_fields = metadata.keys() - allowed_fields
    if unexpected_fields:
        raise ValueError(
            f"Audit metadata is not allowed for {action}: "
            f"{', '.join(sorted(unexpected_fields))}"
        )
    event = AuditEvent(
        workspace_id=workspace_id,
        actor_id=actor_id,
        action=action,
        target_type=target_type,
        target_id=target_id,
        result=result,
        trace_id=trace_id,
        event_metadata=metadata,
        occurred_at=occurred_at or datetime.now(UTC),
    )
    session.add(event)
    return event
