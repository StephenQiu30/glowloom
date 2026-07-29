from collections.abc import Mapping
from enum import StrEnum
from typing import cast

import httpx


class TextProviderFailureKind(StrEnum):
    INPUT_REJECTED = "input_rejected"
    CONFIGURATION = "configuration"
    RATE_LIMITED = "rate_limited"
    TIMEOUT = "timeout"
    TEMPORARY_UNAVAILABLE = "temporary_unavailable"
    INVALID_RESPONSE = "invalid_response"
    UNEXPECTED_STATUS = "unexpected_status"


class ProviderDeliveryState(StrEnum):
    RECEIVED = "received"
    UNKNOWN = "unknown"


class TextProviderFailure(RuntimeError):
    def __init__(
        self,
        *,
        kind: TextProviderFailureKind,
        retryable: bool,
        delivery_state: ProviderDeliveryState,
        status_code: int | None = None,
        request_id: str | None = None,
    ) -> None:
        self.kind = kind
        self.retryable = retryable
        self.delivery_state = delivery_state
        self.status_code = status_code
        self.request_id = request_id
        super().__init__(
            "text provider failure: "
            f"kind={kind.value} retryable={retryable} "
            f"delivery_state={delivery_state.value} "
            f"status_code={status_code} request_id={request_id or 'missing'}"
        )


def parse_text_provider_response(response: httpx.Response) -> Mapping[str, object]:
    request_id = response.headers.get("x-request-id") or None
    if not response.is_success:
        raise _failure_for_status(response.status_code, request_id=request_id)

    try:
        payload: object = response.json()
    except ValueError as error:
        raise _invalid_response(request_id=request_id) from error
    if not isinstance(payload, dict):
        raise _invalid_response(request_id=request_id)
    return cast(Mapping[str, object], payload)


def translate_text_provider_request_error(error: httpx.RequestError) -> TextProviderFailure:
    kind = (
        TextProviderFailureKind.TIMEOUT
        if isinstance(error, httpx.TimeoutException)
        else TextProviderFailureKind.TEMPORARY_UNAVAILABLE
    )
    return TextProviderFailure(
        kind=kind,
        retryable=True,
        delivery_state=ProviderDeliveryState.UNKNOWN,
    )


def _failure_for_status(status_code: int, *, request_id: str | None) -> TextProviderFailure:
    if status_code == 400:
        kind = TextProviderFailureKind.INPUT_REJECTED
        retryable = False
    elif status_code in {401, 403, 404}:
        kind = TextProviderFailureKind.CONFIGURATION
        retryable = False
    elif status_code == 429:
        kind = TextProviderFailureKind.RATE_LIMITED
        retryable = True
    elif status_code >= 500:
        kind = TextProviderFailureKind.TEMPORARY_UNAVAILABLE
        retryable = True
    else:
        kind = TextProviderFailureKind.UNEXPECTED_STATUS
        retryable = False
    return TextProviderFailure(
        kind=kind,
        retryable=retryable,
        delivery_state=ProviderDeliveryState.RECEIVED,
        status_code=status_code,
        request_id=request_id,
    )


def _invalid_response(*, request_id: str | None) -> TextProviderFailure:
    return TextProviderFailure(
        kind=TextProviderFailureKind.INVALID_RESPONSE,
        retryable=False,
        delivery_state=ProviderDeliveryState.RECEIVED,
        status_code=200,
        request_id=request_id,
    )
