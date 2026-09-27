"""Explicit simulation for local mock-provider workflow tests.

No media is fetched or inspected here. A real provider must replace this adapter
before generated output can be treated as content-safe in production.
"""

from typing import Annotated, Any, Literal

from pydantic import BaseModel, ConfigDict, Field, ValidationError, model_validator
from temporalio import activity
from temporalio.exceptions import ApplicationError


class MockModerationInput(BaseModel):
    model_config = ConfigDict(extra="forbid")

    operation_id: str = Field(min_length=1)
    output_id: str = Field(min_length=1)
    adapter_key: Literal["mock"]
    kind: Literal["image", "video", "audio", "text"]
    asset_id: str | None = None
    media_url: str | None = None
    text: str | None = None
    mock_status: Literal["passed", "rejected"]
    mock_labels: list[Annotated[str, Field(min_length=1)]] = Field(default_factory=list)

    @model_validator(mode="after")
    def require_content_reference(self) -> "MockModerationInput":
        if self.kind == "text":
            if not self.text:
                raise ValueError("text moderation requires text")
        elif not self.asset_id:
            raise ValueError("mock media moderation requires asset_id")
        return self


class MockModerationOutput(BaseModel):
    status: Literal["passed", "rejected"]
    labels: list[str]
    provider: Literal["mock"] = "mock"


class MockModerationActivities:
    @activity.defn(name="moderation.check")
    async def check(self, payload: dict[str, Any]) -> dict[str, Any]:
        try:
            data = MockModerationInput.model_validate(payload, strict=True)
        except ValidationError:
            raise ApplicationError(
                "invalid moderation activity input",
                type="invalid_activity_input",
                non_retryable=True,
            ) from None
        labels = data.mock_labels
        if data.mock_status == "rejected" and not labels:
            labels = ["mock_rejected"]
        return MockModerationOutput(status=data.mock_status, labels=labels).model_dump(mode="json")
