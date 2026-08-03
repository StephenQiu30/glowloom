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

    with pytest.raises(ValueError, match="password_hash"):
        append_audit_event(
            session,
            workspace_id=common["workspace_id"],
            actor_id=common["actor_id"],
            action="identity.account_deactivated",
            target_type="user_account",
            target_id=common["target_id"],
            trace_id=common["trace_id"],
            metadata={
                "previous_status": "active",
                "status": "deactivated",
                "previous_token_version": 1,
                "token_version": 2,
                "password_hash": "must-not-be-recorded",
            },
        )

    with pytest.raises(ValueError, match="body"):
        append_audit_event(
            session,
            workspace_id=common["workspace_id"],
            actor_id=common["actor_id"],
            action="script.version_published",
            target_type="script_version",
            target_id=common["target_id"],
            trace_id=common["trace_id"],
            metadata={
                "source_id": str(uuid7()),
                "episode_id": str(uuid7()),
                "version_no": 2,
                "previous_version_id": None,
                "current_version_id": str(uuid7()),
                "episode_revision": 2,
                "body": "must-not-be-recorded",
            },
        )

    with pytest.raises(ValueError, match="name"):
        append_audit_event(
            session,
            workspace_id=common["workspace_id"],
            actor_id=common["actor_id"],
            action="asset.created",
            target_type="asset",
            target_id=common["target_id"],
            trace_id=common["trace_id"],
            metadata={
                "revision": 1,
                "kind": "character",
                "project_id": str(uuid7()),
                "name": "must-not-be-recorded",
            },
        )

    with pytest.raises(ValueError, match="spec"):
        append_audit_event(
            session,
            workspace_id=common["workspace_id"],
            actor_id=common["actor_id"],
            action="shot.spec_version_created",
            target_type="shot_spec_version",
            target_id=common["target_id"],
            trace_id=common["trace_id"],
            metadata={
                "shot_id": str(uuid7()),
                "episode_id": str(uuid7()),
                "version_no": 1,
                "shot_revision": 2,
                "source": "manual_save",
                "previous_version_id": None,
                "current_version_id": str(uuid7()),
                "spec": {"must": "not be recorded"},
            },
        )

    with pytest.raises(ValueError, match="budget_limit"):
        append_audit_event(
            session,
            workspace_id=common["workspace_id"],
            actor_id=common["actor_id"],
            action="project.budget_updated",
            target_type="project",
            target_id=common["target_id"],
            trace_id=common["trace_id"],
            metadata={
                "revision": 2,
                "changed_fields": ["budget_limit", "currency"],
                "budget_limit": "must-not-be-recorded",
            },
        )

    with pytest.raises(ValueError, match="name"):
        append_audit_event(
            session,
            workspace_id=common["workspace_id"],
            actor_id=common["actor_id"],
            action="episode.updated",
            target_type="episode",
            target_id=common["target_id"],
            trace_id=common["trace_id"],
            metadata={
                "project_id": str(uuid7()),
                "revision": 2,
                "changed_fields": ["name"],
                "name": "must-not-be-recorded",
            },
        )
