from __future__ import annotations

from datetime import datetime
from typing import Literal
from uuid import UUID

from pydantic import BaseModel, ConfigDict, Field, model_validator

from app.modules.production import TaskResponse


class ScheduleScopeResponse(BaseModel):
    usage_type: Literal["upload_session", "workspace", "media_location"]
    usage_id: UUID


class ScheduleResponse(BaseModel):
    id: UUID
    workspace_id: UUID
    schedule_key: str
    handler_name: Literal[
        "expire_upload_session",
        "cleanup_expired_uploads",
        "retire_media_location",
        "unregistered",
    ]
    scope: ScheduleScopeResponse
    kind: Literal["one_off", "interval", "cron"]
    rule: (
        ScheduleOneOffRuleResponse
        | ScheduleIntervalRuleResponse
        | ScheduleCronRuleResponse
        | UnknownScheduleRuleResponse
    )
    timezone: str
    status: Literal["active", "paused", "completed", "manual_attention"]
    next_fire_at: datetime | None
    next_attempt_at: datetime | None
    misfire_policy: Literal["skip", "run_once", "catch_up"]
    max_catch_up: int
    failure_count: int
    last_error: str | None
    revision: int


class PaginatedSchedules(BaseModel):
    items: list[ScheduleResponse]
    total: int
    limit: int
    offset: int


class ScheduleStateRequest(BaseModel):
    model_config = ConfigDict(extra="forbid")

    expected_revision: int = Field(ge=1)


class ScheduleResumeRequest(ScheduleStateRequest):
    resume_from: datetime
    misfire_policy: Literal["skip", "run_once", "catch_up"]
    max_catch_up: int = Field(default=0, ge=0, le=20)

    @model_validator(mode="after")
    def validate_catch_up(self) -> ScheduleResumeRequest:
        if self.misfire_policy == "catch_up" and self.max_catch_up == 0:
            raise ValueError("max_catch_up is required for catch_up")
        if self.misfire_policy != "catch_up" and self.max_catch_up != 0:
            raise ValueError("max_catch_up is only valid for catch_up")
        return self


class ScheduleConfigurationRequest(ScheduleStateRequest):
    effective_from: datetime
    kind: Literal["interval", "cron"]
    interval_seconds: int | None = Field(default=None, ge=60, le=86400)
    cron_expression: str | None = Field(default=None, min_length=9, max_length=100)
    timezone: str = Field(default="UTC", min_length=1, max_length=80)
    misfire_policy: Literal["skip", "run_once", "catch_up"]
    max_catch_up: int = Field(default=0, ge=0, le=20)
    misfire_grace_seconds: int = Field(default=30, ge=0, le=3600)

    @model_validator(mode="after")
    def validate_configuration(self) -> ScheduleConfigurationRequest:
        if self.kind == "interval":
            if self.interval_seconds is None or self.cron_expression is not None:
                raise ValueError("interval configuration requires only interval_seconds")
            if self.timezone != "UTC":
                raise ValueError("interval schedules use UTC")
        elif self.cron_expression is None or self.interval_seconds is not None:
            raise ValueError("cron configuration requires only cron_expression")
        if self.misfire_policy == "catch_up" and self.max_catch_up == 0:
            raise ValueError("max_catch_up is required for catch_up")
        if self.misfire_policy != "catch_up" and self.max_catch_up != 0:
            raise ValueError("max_catch_up is only valid for catch_up")
        return self


class ScheduleTriggerRequest(ScheduleStateRequest):
    idempotency_key: str = Field(min_length=1, max_length=200)


class ScheduleFireResponse(BaseModel):
    id: UUID
    schedule_id: UUID
    scheduled_for: datetime
    trigger_kind: Literal["scheduled", "manual"]
    task: TaskResponse


class ScheduleOneOffRuleResponse(BaseModel):
    kind: Literal["one_off"] = "one_off"
    at: datetime
    misfire_grace_seconds: int


class ScheduleIntervalRuleResponse(BaseModel):
    kind: Literal["interval"] = "interval"
    seconds: int
    misfire_grace_seconds: int


class ScheduleCronRuleResponse(BaseModel):
    kind: Literal["cron"] = "cron"
    expression: str
    misfire_grace_seconds: int


class UnknownScheduleRuleResponse(BaseModel):
    kind: Literal["unknown"] = "unknown"
