import json
from pathlib import Path

import pytest

from app.harness.skills import (
    SkillRegistry,
    SkillRegistryError,
    SkillVersionUnavailable,
    package_hash,
)


def write_skill(root: Path, version: str = "1.0.0") -> Path:
    package = root / "mock.echo" / version
    package.mkdir(parents=True)
    (package / "SKILL.md").write_text(
        "---\n"
        "key: mock.echo\n"
        f"version: {version}\n"
        "description: 回传测试结构化输入\n"
        "default_model: mock.structured\n"
        "max_input_tokens: 100\n"
        "max_output_tokens: 40\n"
        "max_repair_rounds: 2\n"
        "timeout_s: 30\n"
        "tools: []\n"
        "validators: [schema]\n"
        "---\n"
        "只返回符合输出 schema 的 JSON。\n",
        encoding="utf-8",
    )
    (package / "input.schema.json").write_text(
        json.dumps(
            {"type": "object", "required": ["value"], "properties": {"value": {"type": "string"}}}
        ),
        encoding="utf-8",
    )
    (package / "output.schema.json").write_text(
        json.dumps(
            {"type": "object", "required": ["value"], "properties": {"value": {"type": "string"}}}
        ),
        encoding="utf-8",
    )
    return package


def test_registry_loads_versioned_skill_and_rejects_missing_version(tmp_path: Path) -> None:
    first = write_skill(tmp_path)
    second = write_skill(tmp_path, "1.1.0")
    (tmp_path / "index.json").write_text(
        json.dumps({"mock.echo": {"1.0.0": package_hash(first), "1.1.0": package_hash(second)}}),
        encoding="utf-8",
    )

    registry = SkillRegistry.load(tmp_path)
    assert registry.get("mock.echo", "1.0.0").metadata.version == "1.0.0"
    assert registry.get("mock.echo", "1.1.0").metadata.version == "1.1.0"
    with pytest.raises(SkillVersionUnavailable):
        registry.get("mock.echo", "2.0.0")


def test_registry_refuses_tampered_package(tmp_path: Path) -> None:
    package = write_skill(tmp_path)
    (tmp_path / "index.json").write_text(
        json.dumps({"mock.echo": {"1.0.0": package_hash(package)}}), encoding="utf-8"
    )
    (package / "SKILL.md").write_text("changed after release", encoding="utf-8")

    with pytest.raises(SkillRegistryError, match="hash"):
        SkillRegistry.load(tmp_path)


def test_registry_refuses_invalid_output_schema(tmp_path: Path) -> None:
    package = write_skill(tmp_path)
    (package / "output.schema.json").write_text(json.dumps({"type": "unknown"}), encoding="utf-8")
    (tmp_path / "index.json").write_text(
        json.dumps({"mock.echo": {"1.0.0": package_hash(package)}}), encoding="utf-8"
    )

    with pytest.raises(SkillRegistryError, match="schema"):
        SkillRegistry.load(tmp_path)
