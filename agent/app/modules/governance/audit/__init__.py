"""Append-only governance audit resource."""

from app.modules.governance.audit.writer import append_audit_event

__all__ = ["append_audit_event"]
