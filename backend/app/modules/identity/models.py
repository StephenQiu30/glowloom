from datetime import UTC, datetime
from uuid import UUID

from sqlalchemy import (
    CheckConstraint,
    DateTime,
    ForeignKey,
    Integer,
    String,
    Text,
    UniqueConstraint,
    Uuid,
)
from sqlalchemy.orm import Mapped, mapped_column
from uuid6 import uuid7

from app.core.database import Base


def _utc_now() -> datetime:
    return datetime.now(UTC)


class UserAccount(Base):
    __tablename__ = "idn_user_accounts"
    __table_args__ = (
        CheckConstraint("status IN ('active', 'deactivated')", name="ck_idn_user_status"),
        CheckConstraint("token_version >= 1", name="ck_idn_user_token_version"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    email_normalized: Mapped[str] = mapped_column(String(320), unique=True)
    password_hash: Mapped[str] = mapped_column(Text)
    token_version: Mapped[int] = mapped_column(Integer, default=1)
    display_name: Mapped[str] = mapped_column(String(80))
    avatar_url: Mapped[str | None] = mapped_column(Text, nullable=True)
    status: Mapped[str] = mapped_column(String(20), default="active", index=True)
    last_login_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class Workspace(Base):
    __tablename__ = "idn_workspaces"
    __table_args__ = (
        CheckConstraint("status IN ('active', 'archived')", name="ck_idn_workspace_status"),
        CheckConstraint("revision >= 1", name="ck_idn_workspace_revision"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    name: Mapped[str] = mapped_column(String(120))
    status: Mapped[str] = mapped_column(String(20), default="active", index=True)
    revision: Mapped[int] = mapped_column(Integer, default=1)
    archived_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True), default=_utc_now, onupdate=_utc_now
    )


class Membership(Base):
    __tablename__ = "idn_memberships"
    __table_args__ = (
        CheckConstraint("role IN ('owner', 'editor', 'viewer')", name="ck_idn_membership_role"),
        CheckConstraint("status IN ('active', 'removed')", name="ck_idn_membership_status"),
        UniqueConstraint("workspace_id", "user_id", name="uq_idn_membership_workspace_user"),
    )

    id: Mapped[UUID] = mapped_column(Uuid, primary_key=True, default=uuid7)
    workspace_id: Mapped[UUID] = mapped_column(
        Uuid, ForeignKey("idn_workspaces.id"), nullable=False
    )
    user_id: Mapped[UUID] = mapped_column(Uuid, ForeignKey("idn_user_accounts.id"), nullable=False)
    role: Mapped[str] = mapped_column(String(20), default="owner")
    status: Mapped[str] = mapped_column(String(20), default="active")
    joined_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=_utc_now)
    removed_at: Mapped[datetime | None] = mapped_column(DateTime(timezone=True), nullable=True)
