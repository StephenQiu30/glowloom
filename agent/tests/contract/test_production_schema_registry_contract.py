from __future__ import annotations

import json
from pathlib import Path
from typing import Any, cast

from app.protocol.canonical import production_canonical_hash

FIXTURE = (
    Path(__file__).resolve().parents[3]
    / "backend"
    / "tests"
    / "fixtures"
    / "storygraph"
    / "production-schema-registry.json"
)


def test_backend_production_schema_registry_hashes_are_recomputed_by_agent() -> None:
    fixture = cast(dict[str, Any], json.loads(FIXTURE.read_text(encoding="utf-8")))
    manifest = cast(dict[str, Any], fixture["manifest"])
    payloads = cast(list[dict[str, Any]], fixture["payload_contracts"])

    assert production_canonical_hash(manifest) == fixture["schema_hash"]
    assert len(manifest["node_definitions"]) == 31
    assert len(manifest["payload_union_definitions"]) == 32
    assert len(manifest["edge_alias_definitions"]) == 7
    assert len(manifest["edge_matrix_definitions"]) == 65
    assert len(manifest["owner_collection_definitions"]) == 13
    assert len(manifest["coverage_rules"]) == 4
    assert len(manifest["checkpoint_owner_family_definitions"]) == 13
    assert len(manifest["exclusion_definitions"]) == 22
    assert len(payloads) == 25

    hashes: dict[str, str] = {}
    for payload in payloads:
        definition = cast(dict[str, Any], payload["definition"])
        contract_id = cast(str, definition["payload_contract_id"])
        actual_hash = production_canonical_hash(definition)
        assert actual_hash == payload["payload_contract_hash"]
        assert contract_id not in hashes
        hashes[contract_id] = actual_hash

    for union in cast(list[dict[str, Any]], manifest["payload_union_definitions"]):
        contract_id = cast(str, union["payload_contract_id"])
        assert union["payload_contract_hash"] == hashes[contract_id]
