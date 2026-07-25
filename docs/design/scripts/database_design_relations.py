from __future__ import annotations

import re
from itertools import product

Relation = tuple[str, tuple[str, ...], str, tuple[str, ...]]
Key = tuple[str, tuple[str, ...]]
Partial = tuple[str, str, tuple[str, ...], str]


def _fail(message: str) -> None:
    raise AssertionError(message)


def _section(document: str, start: str, end: str) -> str:
    if start not in document or end not in document.split(start, 1)[1]:
        _fail(f"missing document section boundary: {start!r} -> {end!r}")
    return document.split(start, 1)[1].split(end, 1)[0]


def _refs(expression: str) -> list[Key]:
    if "(" not in expression and "/" in expression:
        return [item for part in expression.split("/") for item in _refs(part)]
    if "(" in expression:
        tables, raw_columns = expression.split("(", 1)
        if not raw_columns.endswith(")"):
            _fail(f"invalid relation reference {expression}")
        column_groups = [column.split("/") for column in raw_columns[:-1].split(",")]
    else:
        if expression.count(".") != 1:
            _fail(f"invalid relation reference {expression}")
        tables, column = expression.split(".", 1)
        column_groups = [[column]]
    return [
        (table, tuple(columns))
        for table in tables.split("/")
        for columns in product(*column_groups)
    ]


def parse_foreign_keys(
    design: str, columns: dict[str, dict[str, str]]
) -> list[Relation]:
    section = _section(design, "### 3.1 FK 与候选键 exact-set", "复合 FK 所需")
    table = section.split("| 分组 | FK exact-set |", 1)[-1]
    expressions = [value for value in re.findall(r"`([^`]+)`", table) if "→" in value]
    result: list[Relation] = []
    current_child = ""
    for expression in expressions:
        left, right = expression.split("→", 1)
        parents = _refs(right)
        if not left:
            if current_child != "delivery_versions" or len(parents) != 1:
                _fail(f"unsupported implicit FK {expression}")
            parent, remote = parents[0]
            locals_ = [
                name
                for name in columns[current_child]
                if name.endswith("_media_version_id")
            ]
            if len(locals_) != 3:
                _fail(
                    "implicit delivery artifact FK columns are not an exact set of three"
                )
            result.extend(
                (current_child, (local,), parent, remote) for local in locals_
            )
            continue
        children = _refs(left)
        current_child = children[0][0]
        if len(children) == len(parents):
            pairs = zip(children, parents)
        elif len(parents) == 1:
            pairs = ((child, parents[0]) for child in children)
        else:
            _fail(f"ambiguous FK shorthand {expression}")
        result.extend(
            (child, local, parent, remote) for (child, local), (parent, remote) in pairs
        )
    return result


def parse_candidate_uniques(design: str) -> list[Key]:
    section = _section(design, "复合 FK 所需 15 组候选 UQ exact-set 为：", "。Episode")
    result: list[Key] = []
    for expression in re.findall(r"`([^`]+)`", section):
        references = _refs(expression)
        if len(references) != 1:
            _fail(f"invalid candidate UQ {expression}")
        result.append(references[0])
    return result


def parse_partials(
    design: str, tables: set[str]
) -> tuple[list[Partial], list[Partial]]:
    section = _section(
        design, "### 3.2 Partial 与普通索引 exact-set", "### 3.3 Enum/CHECK exact-set"
    )
    uniques: list[Partial] = []
    indexes: list[Partial] = []
    pattern = re.compile(r"((?:uq|ix)_[a-z0-9_]+)\(([a-z0-9_,]+)\) WHERE (.+)")
    for expression in re.findall(r"`([^`]+)`", section):
        if " WHERE " not in expression:
            continue
        match = pattern.fullmatch(expression)
        if match is None:
            _fail(f"invalid partial definition {expression}")
        name, raw_keys, predicate = match.groups()
        prefix = name.split("_", 1)[0]
        owners = [table for table in tables if name.startswith(f"{prefix}_{table}_")]
        if len(owners) != 1:
            _fail(f"cannot infer one table for partial {name}")
        item = (name, owners[0], tuple(raw_keys.split(",")), predicate)
        (uniques if prefix == "uq" else indexes).append(item)
    return uniques, indexes


def _unique(values: list[object], label: str) -> None:
    if len(values) != len(set(values)):
        _fail(f"duplicate {label}")


def _exact(actual: list[object], expected: list[object], label: str) -> None:
    _unique(actual, f"documented {label}")
    _unique(expected, f"manifest {label}")
    if set(actual) != set(expected):
        missing = sorted(set(expected) - set(actual))
        extra = sorted(set(actual) - set(expected))
        _fail(f"{label} exact-set mismatch: missing={missing!r}, extra={extra!r}")


def validate_relational_exact_sets(
    manifest: dict, design: str, columns: dict[str, dict[str, str]], tables: list[str]
) -> None:
    manifest_fks = [
        (child, tuple(local), parent, tuple(remote))
        for child, local, parent, remote in manifest["foreign_keys"]
    ]
    manifest_uqs = [
        (table, tuple(keys)) for table, keys in manifest["candidate_uniques"]
    ]
    manifest_pu = [
        (name, table, tuple(keys), predicate)
        for name, table, keys, predicate in manifest["partial_uniques"]
    ]
    manifest_pi = [
        (name, table, tuple(keys), predicate)
        for name, table, keys, predicate in manifest["partial_indexes"]
    ]
    partials = manifest_pu + manifest_pi
    _unique([item[0] for item in partials], "manifest partial name")
    _unique([item[1:] for item in partials], "manifest partial definition")
    for child, local, parent, remote in manifest_fks:
        valid = child in columns and parent in columns
        valid = (
            valid
            and set(local) <= set(columns[child])
            and set(remote) <= set(columns[parent])
        )
        if len(local) != len(remote) or not valid:
            _fail(f"invalid FK {child}{local}->{parent}{remote}")
    for table, keys in manifest_uqs:
        if table not in columns or not set(keys) <= set(columns[table]):
            _fail(f"invalid candidate UQ {table}{keys}")
    for name, table, keys, _predicate in partials:
        if table not in columns or not set(keys) <= set(columns[table]):
            _fail(f"invalid partial {name}")
    documented_pu, documented_pi = parse_partials(design, set(tables))
    _exact(parse_foreign_keys(design, columns), manifest_fks, "foreign key")
    _exact(parse_candidate_uniques(design), manifest_uqs, "candidate unique")
    _exact(documented_pu, manifest_pu, "partial unique")
    _exact(documented_pi, manifest_pi, "partial index")
