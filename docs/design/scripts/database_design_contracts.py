from __future__ import annotations

import json
import re
from collections import Counter
from pathlib import Path

JSONB_COLUMN_COUNT = 22
FIELD_PATTERN = re.compile(
    r"^(?P<name>[a-z][a-z0-9_]*)\s+(?P<type>v\([1-9][0-9]*\)|[utibjzh])"
    r"(?P<nullable>\?)?(?:\s+PK|=.+)?$"
)
SCHEMA_FRAGMENT_PATTERN = re.compile(
    r"(?<![A-Za-z0-9_.-])([a-z][a-z0-9-]*-v[1-9][0-9]*\.schema\.json"
    r"#/\$defs/[A-Za-z][A-Za-z0-9_-]*)"
)


def fail(message: str) -> None:
    raise AssertionError(message)


def load(path: Path) -> dict:
    return json.loads(path.read_text(encoding="utf-8"))


def resolve_pointer(document: dict, pointer: str) -> object:
    node: object = document
    for part in pointer.removeprefix("#/").split("/"):
        part = part.replace("~1", "/").replace("~0", "~")
        if not isinstance(node, dict) or part not in node:
            fail(f"unresolved pointer {pointer}")
        node = node[part]
    return node


def walk_refs(value: object) -> list[str]:
    if isinstance(value, dict):
        refs = [value["$ref"]] if str(value.get("$ref", "")).startswith("#/") else []
        return refs + [ref for item in value.values() for ref in walk_refs(item)]
    if isinstance(value, list):
        return [ref for item in value for ref in walk_refs(item)]
    return []


def section_between(document: str, start: str, end: str) -> str:
    if start not in document or end not in document.split(start, 1)[1]:
        fail(f"missing document section boundary: {start!r} -> {end!r}")
    return document.split(start, 1)[1].split(end, 1)[0]


def parse_columns(design: str) -> dict[str, dict[str, str]]:
    section = section_between(design, "## 3. 20 张表数据字典", "### 3.1")
    result: dict[str, dict[str, str]] = {}
    for table, fields in re.findall(
        r"^\| `([a-z_]+)` \| `([^`]+)` \|", section, re.MULTILINE
    ):
        if table in result:
            fail(f"duplicate data-dictionary table {table}")
        parsed: dict[str, str] = {}
        for field in fields.split(","):
            field = field.strip()
            match = FIELD_PATTERN.fullmatch(field)
            if match is None:
                fail(f"invalid field declaration {table}.{field}")
            name = match.group("name")
            if name in parsed:
                fail(f"duplicate data-dictionary column {table}.{name}")
            parsed[name] = match.group("type") + (match.group("nullable") or "")
        result[table] = parsed
    return result


def unique(values: list[object], label: str) -> None:
    encoded = [json.dumps(value, sort_keys=True) for value in values]
    if len(encoded) != len(set(encoded)):
        fail(f"duplicate {label}")


def require_exact_set(actual: set[object], expected: set[object], label: str) -> None:
    if actual != expected:
        missing, extra = sorted(expected - actual), sorted(actual - expected)
        fail(f"{label} exact-set mismatch: missing={missing!r}, extra={extra!r}")


def validate_jsonb_mappings(
    raw_mappings: list[object],
    columns: dict[str, dict[str, str]],
    documents: dict[str, dict],
    design: str,
) -> None:
    unique(raw_mappings, "JSONB mapping")
    mappings: list[tuple[str, str, str, str]] = []
    for raw_mapping in raw_mappings:
        valid = isinstance(raw_mapping, list) and len(raw_mapping) == 4
        if not valid or not all(
            isinstance(value, str) and value for value in raw_mapping
        ):
            fail(f"invalid JSONB mapping {raw_mapping!r}")
        table, column, filename, definition = raw_mapping
        mappings.append((table, column, filename, definition))

    mapped = [(table, column) for table, column, _filename, _definition in mappings]
    unique([list(item) for item in mapped], "JSONB column mapping")
    declared = [
        (table, column)
        for table, table_columns in columns.items()
        for column, field_type in table_columns.items()
        if field_type in {"j", "j?"}
    ]
    for label, values in (
        ("data dictionary j/j?", declared),
        ("manifest JSONB", mapped),
    ):
        if len(values) != JSONB_COLUMN_COUNT:
            fail(
                f"{label} columns must total {JSONB_COLUMN_COUNT}, found {len(values)}"
            )
    require_exact_set(
        set(mapped), set(declared), "manifest/data-dictionary JSONB column"
    )

    manifest_fragments: list[str] = []
    for table, column, filename, definition in mappings:
        if filename not in documents:
            fail(f"unknown JSON Schema document {filename} for {table}.{column}")
        fragment = f"{filename}#/$defs/{definition}"
        resolve_pointer(documents[filename], f"#/$defs/{definition}")
        manifest_fragments.append(fragment)
    section = section_between(
        design, "## 4. JSONB 严格 Schema exact-set", "### 4.1 哈希输入 exact-set"
    )
    actual = Counter(SCHEMA_FRAGMENT_PATTERN.findall(section))
    expected = Counter(manifest_fragments)
    if actual != expected:
        missing = list((expected - actual).elements())
        extra = list((actual - expected).elements())
        fail(
            f"document/manifest Schema fragment mismatch: missing={missing!r}, extra={extra!r}"
        )
