from __future__ import annotations

import html
import json
import re
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
DESIGN = FIXTURE.parents[4] / "docs" / "design" / "0010-StoryGraph内容图与DAG创作画布设计.md"


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


def test_production_registry_matrix_and_collection_cells_match_design_gfm_text() -> None:
    fixture = cast(dict[str, Any], json.loads(FIXTURE.read_text(encoding="utf-8")))
    manifest = cast(dict[str, Any], fixture["manifest"])
    matrices = cast(list[dict[str, Any]], manifest["edge_matrix_definitions"])
    actual_by_matrix: dict[str, list[dict[str, str]]] = {}
    for definition in matrices:
        actual_by_matrix.setdefault(cast(str, definition["matrix_id"]), []).append(
            cast(dict[str, str], definition["row_payload"])
        )

    matrix_tables: dict[str, tuple[str, tuple[str, ...]]] = {
        "| Edge Type |": (
            "edge-type-matrix-contract",
            ("edge_type", "allowed_source_to_target", "qualifier_and_cardinality"),
        ),
        "| Claim 分支 |": (
            "claim-cardinality-matrix-contract",
            ("claim_branch", "participant_cardinality", "anchor_cardinality", "state_cardinality"),
        ),
        "| Source | Target | `binding_role` |": (
            "materialization-matrix-contract",
            ("source", "target", "binding_role", "target_cardinality"),
        ),
        "| Source | `reference_role` |": (
            "reference-planning-matrix-contract",
            ("source", "reference_role", "target_payload_field"),
        ),
        "| `target_kind` | `target_uniqueness_key` |": (
            "reference-target-input-compatibility-matrix-contract",
            (
                "target_kind",
                "target_uniqueness_key",
                "expected_target_coverage",
                "identity_refs",
                "specification_refs",
                "state_refs",
                "style_refs",
                "scene_refs",
                "occurrence_refs",
                "interaction_refs",
                "target_dependencies",
            ),
        ),
        "| `target_kind` | 唯一允许的结果 |": (
            "reference-target-result-matrix-contract",
            ("target_kind", "unique_allowed_result"),
        ),
        "| Target 激活条件 |": (
            "reference-target-activation-matrix-contract",
            (
                "target_activation_condition",
                "required_cardinality",
                "optional_cardinality",
                "not_generated_cardinality",
            ),
        ),
        "| Target Binding |": (
            "reference-binding-matrix-contract",
            ("target_binding", "source", "reference_role", "target_cardinality"),
        ),
    }
    for header, (matrix_id, fields) in matrix_tables.items():
        _, rows = _design_table(header)
        expected = [dict(zip(fields, row, strict=True)) for row in rows]
        assert actual_by_matrix[matrix_id] == expected

    _, collection_rows = _design_table("| 最低 Coverage |")
    collections = cast(list[dict[str, Any]], manifest["owner_collection_definitions"])
    actual = {cast(str, value["version_family"]): value for value in collections}
    for phase, family_owner, scope, cardinality, empty, completeness in collection_rows:
        family, owner = family_owner.split(" / ", maxsplit=1)
        value = actual[family]
        assert (
            value["minimum_coverage"],
            value["owner_kind"],
            value["scope_kind"],
            value["scope_key_and_collection_cardinality"],
            value["empty_collection_policy"],
            value["member_completeness"],
        ) == (phase, owner, scope, cardinality, empty, completeness)


def _design_table(header_prefix: str) -> tuple[list[str], list[list[str]]]:
    lines = DESIGN.read_text(encoding="utf-8").splitlines()
    start = next(index for index, line in enumerate(lines) if line.startswith(header_prefix))
    headings = _split_gfm_row(lines[start])
    rows: list[list[str]] = []
    for line in lines[start + 2 :]:
        if not line.startswith("|"):
            break
        rows.append(_split_gfm_row(line))
    return headings, rows


def _split_gfm_row(line: str) -> list[str]:
    cells: list[str] = []
    buffer: list[str] = []
    in_code = False
    for character in line.strip()[1:-1]:
        if character == "`":
            in_code = not in_code
            continue
        if character == "|" and not in_code:
            cells.append(_plain_gfm_cell("".join(buffer)))
            buffer = []
            continue
        buffer.append(character)
    cells.append(_plain_gfm_cell("".join(buffer)))
    return cells


def _plain_gfm_cell(value: str) -> str:
    return re.sub(r"[ \t\r\n\f\v]+", " ", html.unescape(value)).strip()
