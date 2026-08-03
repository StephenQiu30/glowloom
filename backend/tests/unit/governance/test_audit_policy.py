from unittest.mock import Mock

import pytest
from sqlalchemy.ext.asyncio import AsyncSession
from uuid6 import uuid7

from app.modules.governance.audit import append_audit_event


def test_audit_metadata_is_restricted_by_registered_action() -> None:
    session = Mock(spec=AsyncSession)
    common = {
        "session": session,
        "workspace_id": uuid7(),
        "actor_id": uuid7(),
        "target_type": "consent",
        "target_id": uuid7(),
        "trace_id": str(uuid7()),
    }

    with pytest.raises(ValueError, match="Audit action is not registered"):
        append_audit_event(
            **common,
            action="consent.exported",
            metadata={},
        )

    with pytest.raises(ValueError, match="reason"):
        append_audit_event(
            **common,
            action="consent.revoked",
            metadata={
                "revision": 2,
                "subject_type": "MEDIA_VERSION",
                "reason": "must not enter audit metadata",
            },
        )

    event = append_audit_event(
        **common,
        action="consent.revoked",
        metadata={"revision": 2, "subject_type": "MEDIA_VERSION"},
    )
    assert event.event_metadata == {
        "revision": 2,
        "subject_type": "MEDIA_VERSION",
    }
    session.add.assert_called_once_with(event)
