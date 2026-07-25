from __future__ import annotations

import hashlib
import json
import re
import sys
from pathlib import Path

from database_design_contracts import (
    fail,
    load,
    parse_columns,
    require_exact_set,
    resolve_pointer,
    unique,
    validate_jsonb_mappings,
    walk_refs,
)
from database_design_relations import validate_relational_exact_sets
from verify_sql_design import validate_sql_artifacts

ROOT = Path(__file__).resolve().parents[3]
DESIGN = ROOT / "docs/design/06-数据库表与迁移设计.md"
SCHEMAS = ROOT / "docs/design/schemas"
MANIFEST = SCHEMAS / "database-exact-set-v1.json"
CONTRACTS = Path(__file__).with_name("database_design_contracts.py")
RELATIONS = Path(__file__).with_name("database_design_relations.py")
SQL_VERIFIER = Path(__file__).with_name("verify_sql_design.py")
SQL_DIR = ROOT / "sql"


def main() -> None:
    manifest = load(MANIFEST)
    design = DESIGN.read_text(encoding="utf-8")
    counts = manifest["counts"]
    tables = [table for owned in manifest["tables"].values() for table in owned]
    columns = parse_columns(design)
    checks = set(re.findall(r"^([a-z_]+) :=", design, re.MULTILINE))

    counted = (
        "foreign_keys",
        "candidate_uniques",
        "partial_uniques",
        "partial_indexes",
        "jsonb_columns",
        "application_invariants",
        "schema_invariants",
    )
    expected = {"tables": len(tables), **{key: len(manifest[key]) for key in counted}}
    if counts != expected:
        fail(f"count mismatch: declared={counts}, actual={expected}")
    unique(tables, "table")
    application_invariants = manifest["application_invariants"]
    schema_invariants = manifest["schema_invariants"]
    unique(application_invariants, "application invariant")
    unique(schema_invariants, "manifest schema invariant")
    unique(application_invariants + schema_invariants, "manifest invariant ID")
    require_exact_set(set(columns), set(tables), "data-dictionary table")
    require_exact_set(checks, set(tables), "CHECK table")
    validate_relational_exact_sets(manifest, design, columns, tables)

    seen_schema_invariants: list[str] = []
    paths = sorted(SCHEMAS.glob("*-v1.schema.json"))
    documents = {path.name: load(path) for path in paths}
    for document in documents.values():
        for ref in walk_refs(document):
            resolve_pointer(document, ref)
        seen_schema_invariants.extend(
            item.split(":", 1)[0] for item in document["x-lanverse-invariants"]
        )
    unique(seen_schema_invariants, "schema invariant")
    require_exact_set(
        set(seen_schema_invariants), set(schema_invariants), "schema invariant"
    )

    documented_application_invariants = re.findall(r"\bINV-APP-[0-9]+\b", design)
    unique(documented_application_invariants, "documented application invariant")
    require_exact_set(
        set(documented_application_invariants),
        set(application_invariants),
        "application invariant",
    )
    validate_jsonb_mappings(manifest["jsonb_columns"], columns, documents, design)
    sql_artifacts = validate_sql_artifacts(SQL_DIR, manifest, columns)

    artifacts = [
        DESIGN,
        MANIFEST,
        Path(__file__),
        CONTRACTS,
        RELATIONS,
        SQL_VERIFIER,
        *sql_artifacts,
        *sorted(SCHEMAS / name for name in documents),
    ]
    digests = {
        str(path.relative_to(ROOT)): hashlib.sha256(path.read_bytes()).hexdigest()
        for path in artifacts
    }
    print(
        json.dumps(
            {"result": "passed", "counts": expected, "sha256": digests},
            ensure_ascii=False,
            sort_keys=True,
        )
    )


if __name__ == "__main__":
    try:
        main()
    except (
        AssertionError,
        KeyError,
        TypeError,
        ValueError,
        json.JSONDecodeError,
    ) as error:
        print(f"database design verification failed: {error}", file=sys.stderr)
        raise SystemExit(1) from error
