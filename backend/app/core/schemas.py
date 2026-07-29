from typing import Generic, Literal, TypeVar

from pydantic import BaseModel

T = TypeVar("T")


class ApiResponse(BaseModel, Generic[T]):
    data: T


class HealthResponse(BaseModel):
    status: Literal["ok"] = "ok"


class DependencyStatus(BaseModel):
    critical: bool
    status: Literal["available", "degraded", "unavailable"]
    reason: str | None = None


class ReadinessResponse(BaseModel):
    status: Literal["ready", "degraded", "unavailable"]
    dependencies: dict[str, DependencyStatus]
