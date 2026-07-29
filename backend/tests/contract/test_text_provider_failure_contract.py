from collections.abc import Mapping

import httpx
import pytest

from app.integrations.text_provider import (
    ProviderDeliveryState,
    TextProviderFailure,
    TextProviderFailureKind,
    parse_text_provider_response,
    translate_text_provider_request_error,
)


def provider_response(
    status_code: int,
    *,
    content: bytes = b'{"result":"ok"}',
    request_id: str = "request-fixture-001",
) -> httpx.Response:
    request = httpx.Request("POST", "https://provider.invalid/chat/completions")
    return httpx.Response(
        status_code,
        content=content,
        headers={"x-request-id": request_id},
        request=request,
    )


@pytest.mark.parametrize(
    ("status_code", "kind", "retryable"),
    [
        (400, TextProviderFailureKind.INPUT_REJECTED, False),
        (401, TextProviderFailureKind.CONFIGURATION, False),
        (403, TextProviderFailureKind.CONFIGURATION, False),
        (404, TextProviderFailureKind.CONFIGURATION, False),
        (429, TextProviderFailureKind.RATE_LIMITED, True),
        (500, TextProviderFailureKind.TEMPORARY_UNAVAILABLE, True),
        (503, TextProviderFailureKind.TEMPORARY_UNAVAILABLE, True),
    ],
)
def test_http_failure_has_stable_classification(
    status_code: int,
    kind: TextProviderFailureKind,
    retryable: bool,
) -> None:
    with pytest.raises(TextProviderFailure) as captured:
        parse_text_provider_response(provider_response(status_code))

    failure = captured.value
    assert failure.kind is kind
    assert failure.retryable is retryable
    assert failure.delivery_state is ProviderDeliveryState.RECEIVED
    assert failure.status_code == status_code
    assert failure.request_id == "request-fixture-001"
    assert "result" not in str(failure)


def test_timeout_is_retryable_but_has_unknown_delivery_state() -> None:
    request = httpx.Request("POST", "https://provider.invalid/chat/completions")
    error = httpx.ReadTimeout("synthetic timeout with secret-body", request=request)

    failure = translate_text_provider_request_error(error)

    assert failure.kind is TextProviderFailureKind.TIMEOUT
    assert failure.retryable is True
    assert failure.delivery_state is ProviderDeliveryState.UNKNOWN
    assert failure.status_code is None
    assert failure.request_id is None
    assert "secret-body" not in str(failure)


@pytest.mark.parametrize("content", [b"not-json", b"[]", b'"text"'])
def test_invalid_json_object_is_non_retryable_and_does_not_leak_body(content: bytes) -> None:
    with pytest.raises(TextProviderFailure) as captured:
        parse_text_provider_response(provider_response(200, content=content))

    failure = captured.value
    assert failure.kind is TextProviderFailureKind.INVALID_RESPONSE
    assert failure.retryable is False
    assert failure.delivery_state is ProviderDeliveryState.RECEIVED
    assert failure.request_id == "request-fixture-001"
    assert content.decode() not in str(failure)


def test_valid_json_object_is_returned_without_adapter_specific_shape() -> None:
    body = parse_text_provider_response(provider_response(200))

    assert isinstance(body, Mapping)
    assert body == {"result": "ok"}
