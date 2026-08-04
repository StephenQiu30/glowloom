from datetime import UTC, datetime, timedelta
from decimal import Decimal
from uuid import UUID

import pytest
from pydantic import SecretStr

from app.core.config import Settings
from app.modules.production.capabilities import (
    builtin_unavailable_capabilities,
    estimate_fixed_request_cost,
    validate_capability_parameters,
)
from app.modules.production.generation import (
    generation_preflight_signature,
    verify_generation_preflight_signature,
)


def test_builtin_ark_capabilities_are_stable_unavailable_and_key_independent() -> None:
    without_key = builtin_unavailable_capabilities(Settings(environment="test"))
    with_key = builtin_unavailable_capabilities(
        Settings(environment="test", ark_api_key=SecretStr("not-used-by-catalog"))
    )

    assert without_key == with_key
    assert [item.model for item in without_key] == [
        "doubao-seedream-5-0-lite-260128",
        "doubao-seedance-2-0-260128",
    ]
    assert all(item.status == "unavailable" for item in without_key)
    assert all(item.unavailable_reason == "provider_contract_unverified" for item in without_key)
    assert all(item.pricing is None for item in without_key)
    assert all(
        item.parameter_schema == {"type": "object", "properties": {}, "additionalProperties": False}
        for item in without_key
    )
    assert len({item.id for item in without_key}) == 2


def test_capability_parameter_schema_is_fail_closed_and_cost_uses_decimal() -> None:
    schema = {
        "type": "object",
        "properties": {
            "resolution": {"type": "string", "enum": ["720p", "1080p"]},
            "duration_seconds": {"type": "integer", "minimum": 1, "maximum": 12},
            "generate_audio": {"type": "boolean"},
        },
        "required": ["resolution", "duration_seconds"],
        "additionalProperties": False,
    }

    assert validate_capability_parameters(
        schema,
        {
            "resolution": "1080p",
            "duration_seconds": 5,
            "generate_audio": False,
        },
    ) == {
        "duration_seconds": 5,
        "generate_audio": False,
        "resolution": "1080p",
    }
    with pytest.raises(ValueError, match="unknown parameter"):
        validate_capability_parameters(
            schema, {"resolution": "1080p", "duration_seconds": 5, "prompt": "secret"}
        )
    with pytest.raises(ValueError, match="required parameter"):
        validate_capability_parameters(schema, {"resolution": "1080p"})
    with pytest.raises(ValueError, match="unsupported schema keyword"):
        validate_capability_parameters({"type": "object", "oneOf": []}, {})
    with pytest.raises(ValueError, match="unsupported schema keyword"):
        validate_capability_parameters(
            {
                "type": "object",
                "properties": {"optional": {"type": "string", "pattern": "^unsafe$"}},
                "additionalProperties": False,
            },
            {},
        )
    with pytest.raises(ValueError, match="invalid type"):
        validate_capability_parameters(
            {
                "type": "object",
                "properties": {
                    "optional": {"type": "integer", "enum": [1, "not-an-integer"]}
                },
                "additionalProperties": False,
            },
            {},
        )
    with pytest.raises(ValueError, match="finite"):
        validate_capability_parameters(
            {
                "type": "object",
                "properties": {"strength": {"type": "number"}},
                "required": ["strength"],
                "additionalProperties": False,
            },
            {"strength": float("nan")},
        )

    amount, currency, high_cost_threshold = estimate_fixed_request_cost(
        {
            "unit": "per_request",
            "amount": "12.500000",
            "currency": "CNY",
            "high_cost_threshold": "10.000000",
        }
    )
    assert amount == Decimal("12.500000")
    assert currency == "CNY"
    assert high_cost_threshold == Decimal("10.000000")
    with pytest.raises(ValueError, match="unsupported pricing unit"):
        estimate_fixed_request_cost({"unit": "per_second", "amount": "1", "currency": "CNY"})


def test_preflight_signature_binds_expiry_and_normalized_facts() -> None:
    settings = Settings(
        environment="test",
        jwt_secret_key=SecretStr("preflight-signing-secret-with-at-least-32-bytes"),
    )
    expires_at = datetime(2026, 8, 4, 12, 5, tzinfo=UTC)
    facts = {
        "workspace_id": str(UUID("00000000-0000-0000-0000-000000000001")),
        "shot_id": str(UUID("00000000-0000-0000-0000-000000000002")),
        "parameters": {"duration_seconds": 5, "resolution": "1080p"},
        "estimated_amount": "12.500000",
    }

    signature = generation_preflight_signature(settings, facts, expires_at)
    assert len(signature) == 64
    assert verify_generation_preflight_signature(
        settings,
        facts,
        expires_at,
        signature,
        now=expires_at - timedelta(seconds=1),
    )
    assert not verify_generation_preflight_signature(
        settings,
        facts,
        expires_at,
        signature,
        now=expires_at + timedelta(microseconds=1),
    )
    assert not verify_generation_preflight_signature(
        settings,
        {**facts, "estimated_amount": "12.500001"},
        expires_at,
        signature,
        now=expires_at - timedelta(seconds=1),
    )
