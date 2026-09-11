from __future__ import annotations

import json
from pathlib import Path
from typing import Any, cast

import pytest

from app.protocol.canonical import (
    ProductionCanonicalError,
    production_canonical_hash,
    production_canonical_json,
    production_canonical_json_text,
)

FIXTURE = (
    Path(__file__).resolve().parents[3]
    / "backend"
    / "tests"
    / "fixtures"
    / "agent"
    / "production-canonical.json"
)


def test_production_canonical_fixture_matches_rfc8785_and_backend_codes() -> None:
    fixture = cast(dict[str, list[dict[str, Any]]], json.loads(FIXTURE.read_text()))
    for case in fixture["success_cases"]:
        assert production_canonical_json(case["input"]).decode() == case["canonical_json"]
        assert production_canonical_hash(case["input"]) == case["sha256"]

    for case in fixture["failure_cases"]:
        raw = (
            bytes.fromhex(case["raw_json_hex"])
            if case.get("raw_json_hex")
            else cast(str, case["raw_json"]).encode()
        )
        with pytest.raises(ProductionCanonicalError) as captured:
            production_canonical_json_text(raw)
        assert captured.value.code == case["error_code"]
