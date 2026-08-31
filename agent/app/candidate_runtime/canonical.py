from __future__ import annotations

import hashlib
import json
import unicodedata
from typing import Any, cast


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
    return json.dumps(
        normalized,
        ensure_ascii=False,
        separators=(",", ":"),
        sort_keys=True,
        allow_nan=False,
    ).encode("utf-8")


def production_canonical_hash(value: Any) -> str:
    return hashlib.sha256(production_canonical_json(value)).hexdigest()


def _normalize_production_value(value: Any) -> Any:
    if value is None or isinstance(value, bool):
        return value
    if isinstance(value, int) and not isinstance(value, bool):
        return value
    if isinstance(value, float):
        raise ValueError("production canonical JSON only permits integers")
    if isinstance(value, str):
        return unicodedata.normalize("NFC", value)
    if isinstance(value, list):
        return [_normalize_production_value(item) for item in cast(list[Any], value)]
    if isinstance(value, dict):
        normalized: dict[str, Any] = {}
        for key, item in cast(dict[Any, Any], value).items():
            if not isinstance(key, str):
                raise ValueError("production canonical JSON keys must be strings")
            normalized_key = unicodedata.normalize("NFC", key)
            if normalized_key in normalized:
                raise ValueError("production canonical JSON contains duplicate normalized keys")
            normalized[normalized_key] = _normalize_production_value(item)
        return normalized
    raise ValueError("production canonical JSON contains an unsupported value")
