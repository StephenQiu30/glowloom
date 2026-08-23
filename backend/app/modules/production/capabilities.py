from dataclasses import dataclass
from decimal import Decimal, InvalidOperation
from typing import Any, Literal, cast
from uuid import UUID

from app.core.config import Settings

CapabilityKind = Literal["image", "video"]
CapabilityStatus = Literal["active", "inactive", "unavailable"]


@dataclass(frozen=True, slots=True)
class CapabilityDefinition:
    id: UUID
    provider: str
    model: str
    kind: CapabilityKind
    config_version: int
    input_types: tuple[str, ...]
    parameter_schema: dict[str, Any]
    limits: dict[str, Any]
    pricing: dict[str, Any] | None
    status: CapabilityStatus
    unavailable_reason: str | None


_CLOSED_PARAMETER_SCHEMA: dict[str, Any] = {
    "type": "object",
    "properties": {},
    "additionalProperties": False,
}

_BUILTIN_UNAVAILABLE_CAPABILITIES = (
    CapabilityDefinition(
        id=UUID("8bdbff1c-b543-54e7-a3a2-5337d47edcee"),
        provider="volcengine_ark",
        model="doubao-seedream-5-0-lite-260128",
        kind="image",
        config_version=1,
        input_types=("shot_spec",),
        parameter_schema=_CLOSED_PARAMETER_SCHEMA,
        limits={},
        pricing=None,
        status="unavailable",
        unavailable_reason="provider_contract_unverified",
    ),
    CapabilityDefinition(
        id=UUID("c628d073-6b46-5d62-bc2e-2082a43a87fa"),
        provider="volcengine_ark",
        model="doubao-seedance-2-0-260128",
        kind="video",
        config_version=1,
        input_types=("shot_spec",),
        parameter_schema=_CLOSED_PARAMETER_SCHEMA,
        limits={},
        pricing=None,
        status="unavailable",
        unavailable_reason="provider_contract_unverified",
    ),
)

_ROOT_SCHEMA_KEYS = frozenset({"type", "properties", "required", "additionalProperties"})
_FIELD_SCHEMA_KEYS = frozenset({"type", "enum", "minimum", "maximum", "minLength", "maxLength"})
_SUPPORTED_PARAMETER_TYPES = frozenset({"string", "integer", "number", "boolean"})
_PRICING_KEYS = frozenset({"unit", "amount", "currency", "high_cost_threshold"})


def builtin_unavailable_capabilities(settings: Settings) -> tuple[CapabilityDefinition, ...]:
    """Return stable public identities; an API key never activates an unverified contract."""
    _ = settings
    return _BUILTIN_UNAVAILABLE_CAPABILITIES


def builtin_capability(capability_id: UUID) -> CapabilityDefinition | None:
    return next(
        (item for item in _BUILTIN_UNAVAILABLE_CAPABILITIES if item.id == capability_id),
        None,
    )


def validate_capability_parameters(
    schema: dict[str, Any],
    parameters: dict[str, Any],
) -> dict[str, Any]:
    unsupported_root = set(schema) - _ROOT_SCHEMA_KEYS
    if unsupported_root:
        raise ValueError(f"unsupported schema keyword: {sorted(unsupported_root)[0]}")
    if schema.get("type") != "object":
        raise ValueError("capability parameter schema must be an object")
    if schema.get("additionalProperties") is not False:
        raise ValueError("capability parameter schema must reject additional properties")
    properties_value = schema.get("properties")
    if not isinstance(properties_value, dict):
        raise ValueError("capability parameter properties are invalid")
    properties = cast(dict[str, object], properties_value)
    required_value = schema.get("required", [])
    if not isinstance(required_value, list):
        raise ValueError("capability required parameters are invalid")
    required_items = cast(list[object], required_value)
    if not all(isinstance(item, str) for item in required_items):
        raise ValueError("capability required parameters are invalid")
    required = cast(list[str], required_value)
    unknown_required = set(required) - set(properties)
    if unknown_required:
        raise ValueError(f"required parameter is not declared: {sorted(unknown_required)[0]}")
    validated_properties: dict[str, tuple[str, dict[str, object]]] = {}
    for name, definition_value in properties.items():
        if not isinstance(definition_value, dict):
            raise ValueError(f"parameter schema is invalid: {name}")
        definition = cast(dict[str, object], definition_value)
        unsupported_field = set(definition) - _FIELD_SCHEMA_KEYS
        if unsupported_field:
            raise ValueError(f"unsupported schema keyword: {sorted(unsupported_field)[0]}")
        expected_type_value = definition.get("type")
        if not isinstance(expected_type_value, str) or expected_type_value not in (
            _SUPPORTED_PARAMETER_TYPES
        ):
            raise ValueError(f"parameter type is unsupported: {name}")
        _validate_parameter_definition(name, expected_type_value, definition)
        validated_properties[name] = (expected_type_value, definition)
    unknown = set(parameters) - set(properties)
    if unknown:
        raise ValueError(f"unknown parameter: {sorted(unknown)[0]}")
    missing = set(required) - set(parameters)
    if missing:
        raise ValueError(f"required parameter is missing: {sorted(missing)[0]}")

    normalized: dict[str, Any] = {}
    for name in sorted(parameters):
        expected_type, definition = validated_properties[name]
        value = parameters[name]
        _validate_parameter_type(name, expected_type, value)
        enum_value = definition.get("enum")
        if enum_value is not None and (
            not isinstance(enum_value, list) or value not in cast(list[object], enum_value)
        ):
            raise ValueError(f"parameter is outside the allowed values: {name}")
        _validate_parameter_bounds(name, expected_type, value, definition)
        normalized[name] = value
    return normalized


def _validate_parameter_definition(
    name: str,
    expected_type: str,
    definition: dict[str, object],
) -> None:
    enum_items: list[object] = []
    enum_value = definition.get("enum")
    if enum_value is not None:
        if not isinstance(enum_value, list):
            raise ValueError(f"parameter enum is invalid: {name}")
        enum_items = cast(list[object], enum_value)
        if not enum_items:
            raise ValueError(f"parameter enum is invalid: {name}")
        for item in enum_items:
            _validate_parameter_type(name, expected_type, item)
    if expected_type == "string":
        for key in ("minLength", "maxLength"):
            value = definition.get(key)
            if value is not None and (
                not isinstance(value, int) or isinstance(value, bool) or value < 0
            ):
                raise ValueError(f"parameter length bound is invalid: {name}")
        if "minimum" in definition or "maximum" in definition:
            raise ValueError(f"numeric bound is not allowed for parameter: {name}")
    elif expected_type == "boolean":
        if any(key in definition for key in ("minimum", "maximum", "minLength", "maxLength")):
            raise ValueError(f"bounds are not allowed for parameter: {name}")
    else:
        if "minLength" in definition or "maxLength" in definition:
            raise ValueError(f"length bound is not allowed for parameter: {name}")
        for key in ("minimum", "maximum"):
            value = definition.get(key)
            if value is None:
                continue
            try:
                bound = Decimal(str(value))
            except InvalidOperation as error:
                raise ValueError(f"numeric bound is invalid: {name}") from error
            if not bound.is_finite():
                raise ValueError(f"numeric bound must be finite: {name}")
    for item in enum_items:
        _validate_parameter_bounds(name, expected_type, item, definition)


def _validate_parameter_type(name: str, expected_type: object, value: object) -> None:
    valid = (
        isinstance(value, str)
        if expected_type == "string"
        else isinstance(value, int) and not isinstance(value, bool)
        if expected_type == "integer"
        else isinstance(value, (int, float, Decimal)) and not isinstance(value, bool)
        if expected_type == "number"
        else isinstance(value, bool)
    )
    if not valid:
        raise ValueError(f"parameter has an invalid type: {name}")


def _validate_parameter_bounds(
    name: str,
    expected_type: object,
    value: object,
    definition: dict[str, Any],
) -> None:
    if expected_type == "string":
        length = len(str(value))
        minimum_length = definition.get("minLength")
        maximum_length = definition.get("maxLength")
        if isinstance(minimum_length, int) and length < minimum_length:
            raise ValueError(f"parameter is shorter than allowed: {name}")
        if isinstance(maximum_length, int) and length > maximum_length:
            raise ValueError(f"parameter is longer than allowed: {name}")
        return
    if expected_type == "boolean":
        return
    number = Decimal(str(value))
    if not number.is_finite():
        raise ValueError(f"parameter must be finite: {name}")
    minimum = definition.get("minimum")
    maximum = definition.get("maximum")
    if minimum is not None and number < Decimal(str(minimum)):
        raise ValueError(f"parameter is lower than allowed: {name}")
    if maximum is not None and number > Decimal(str(maximum)):
        raise ValueError(f"parameter is higher than allowed: {name}")


def estimate_fixed_request_cost(
    pricing: dict[str, Any],
) -> tuple[Decimal, str, Decimal | None]:
    unsupported = set(pricing) - _PRICING_KEYS
    if unsupported:
        raise ValueError(f"unsupported pricing field: {sorted(unsupported)[0]}")
    if pricing.get("unit") != "per_request":
        raise ValueError("unsupported pricing unit")
    currency = pricing.get("currency")
    if not isinstance(currency, str) or len(currency) != 3 or not currency.isupper():
        raise ValueError("invalid pricing currency")
    amount = _non_negative_decimal(pricing.get("amount"), "invalid pricing amount")
    threshold_raw = pricing.get("high_cost_threshold")
    threshold = (
        None
        if threshold_raw is None
        else _non_negative_decimal(threshold_raw, "invalid high cost threshold")
    )
    return amount, currency, threshold


def _non_negative_decimal(value: object, message: str) -> Decimal:
    try:
        result = Decimal(str(value))
    except (InvalidOperation, ValueError) as error:
        raise ValueError(message) from error
    if not result.is_finite() or result < 0:
        raise ValueError(message)
    return result.quantize(Decimal("0.000001"))
