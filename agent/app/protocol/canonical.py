from __future__ import annotations

import hashlib
import json
import unicodedata
from typing import Any, cast

MAXIMUM_SAFE_INTEGER = 9_007_199_254_740_991


class ProductionCanonicalError(ValueError):
    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code


def canonical_json(value: Any) -> bytes:
    return json.dumps(
        value,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
    ).encode("utf-8")


def canonical_hash(value: Any) -> str:
    return hashlib.sha256(canonical_json(value)).hexdigest()


def production_canonical_json(value: Any) -> bytes:
    normalized = _normalize_production_value(value)
    try:
        return json.dumps(
            normalized,
            ensure_ascii=False,
            separators=(",", ":"),
            sort_keys=False,
            allow_nan=False,
        ).encode("utf-8")
    except UnicodeEncodeError as error:
        raise ProductionCanonicalError(
            "invalid_unicode", "Production Canonical JSON contains invalid Unicode"
        ) from error


def production_canonical_hash(value: Any) -> str:
    return hashlib.sha256(production_canonical_json(value)).hexdigest()


def production_canonical_json_text(raw: bytes) -> bytes:
    try:
        text = raw.decode("utf-8")
    except UnicodeDecodeError as error:
        raise ProductionCanonicalError(
            "invalid_unicode", "Production Canonical JSON contains invalid Unicode"
        ) from error
    try:
        value = json.loads(
            text,
            object_pairs_hook=_production_object_pairs,
            parse_float=_reject_production_float,
            parse_int=_parse_production_integer,
            parse_constant=_reject_production_constant,
        )
    except ProductionCanonicalError:
        raise
    except (json.JSONDecodeError, RecursionError) as error:
        raise ProductionCanonicalError(
            "invalid_json", "Production Canonical JSON is invalid"
        ) from error
    return production_canonical_json(value)


def _normalize_production_value(value: Any) -> Any:
    if value is None or isinstance(value, bool):
        return value
    if isinstance(value, int) and not isinstance(value, bool):
        if value < -MAXIMUM_SAFE_INTEGER or value > MAXIMUM_SAFE_INTEGER:
            raise ProductionCanonicalError(
                "integer_out_of_range",
                "Production Canonical JSON integer exceeds the safe range",
            )
        return value
    if isinstance(value, float):
        raise ProductionCanonicalError(
            "integer_required",
            "Production Canonical JSON only permits decimal integers",
        )
    if isinstance(value, str):
        normalized = unicodedata.normalize("NFC", value)
        try:
            normalized.encode("utf-8")
        except UnicodeEncodeError as error:
            raise ProductionCanonicalError(
                "invalid_unicode", "Production Canonical JSON contains invalid Unicode"
            ) from error
        return normalized
    if isinstance(value, list):
        return [_normalize_production_value(item) for item in cast(list[Any], value)]
    if isinstance(value, dict):
        normalized_items: list[tuple[str, Any]] = []
        normalized_keys: set[str] = set()
        for key, item in cast(dict[Any, Any], value).items():
            if not isinstance(key, str):
                raise ProductionCanonicalError(
                    "unsupported_value",
                    "Production Canonical JSON keys must be strings",
                )
            normalized_key = cast(str, _normalize_production_value(key))
            if normalized_key in normalized_keys:
                raise ProductionCanonicalError(
                    "duplicate_normalized_key",
                    "Production Canonical JSON contains duplicate normalized keys",
                )
            normalized_keys.add(normalized_key)
            normalized_items.append((normalized_key, _normalize_production_value(item)))
        normalized_items.sort(key=lambda item: item[0].encode("utf-16-be"))
        return dict(normalized_items)
    raise ProductionCanonicalError(
        "unsupported_value", "Production Canonical JSON contains an unsupported value"
    )


def _production_object_pairs(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    value: dict[str, Any] = {}
    for key, item in pairs:
        if key in value:
            raise ProductionCanonicalError(
                "duplicate_key", "Production Canonical JSON contains duplicate keys"
            )
        value[key] = item
    return value


def _reject_production_float(_: str) -> Any:
    raise ProductionCanonicalError(
        "integer_required", "Production Canonical JSON only permits decimal integers"
    )


def _parse_production_integer(value: str) -> int:
    integer = int(value)
    if integer < -MAXIMUM_SAFE_INTEGER or integer > MAXIMUM_SAFE_INTEGER:
        raise ProductionCanonicalError(
            "integer_out_of_range",
            "Production Canonical JSON integer exceeds the safe range",
        )
    return integer


def _reject_production_constant(_: str) -> Any:
    raise ProductionCanonicalError(
        "invalid_json", "Production Canonical JSON contains a non-JSON constant"
    )
