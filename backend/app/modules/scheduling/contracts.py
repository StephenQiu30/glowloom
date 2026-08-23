from typing import Any, Literal, cast
from uuid import UUID

from pydantic import BaseModel, ConfigDict, ValidationError


class UnregisteredScheduleHandler(RuntimeError):
    pass


class InvalidSchedulePayload(RuntimeError):
    pass


class UploadExpirationPayload(BaseModel):
    model_config = ConfigDict(extra="forbid")

    upload_session_id: UUID


class UploadCleanupPayload(BaseModel):
    model_config = ConfigDict(extra="forbid")

    workspace_id: UUID


class MediaLocationRetirementPayload(BaseModel):
    model_config = ConfigDict(extra="forbid")

    media_location_id: UUID


ScheduleHandlerName = Literal[
    "expire_upload_session",
    "cleanup_expired_uploads",
    "retire_media_location",
]
SchedulePayload = UploadExpirationPayload | UploadCleanupPayload | MediaLocationRetirementPayload

_PAYLOAD_MODELS: dict[str, type[SchedulePayload]] = {
    "expire_upload_session": UploadExpirationPayload,
    "cleanup_expired_uploads": UploadCleanupPayload,
    "retire_media_location": MediaLocationRetirementPayload,
}


def parse_schedule_payload(handler_name: str, payload: dict[str, Any]) -> SchedulePayload:
    model = _PAYLOAD_MODELS.get(handler_name)
    if model is None:
        raise UnregisteredScheduleHandler("schedule handler is not registered")
    try:
        return model.model_validate(payload)
    except ValidationError as error:
        raise InvalidSchedulePayload("schedule payload does not match its handler") from error


def public_handler_name(
    handler_name: str,
) -> ScheduleHandlerName | Literal["unregistered"]:
    if handler_name in _PAYLOAD_MODELS:
        return cast(ScheduleHandlerName, handler_name)
    return "unregistered"
