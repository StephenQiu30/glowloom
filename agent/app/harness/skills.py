"""Load immutable, content-addressed Skill packages at startup."""

import json
import re
from dataclasses import dataclass
from hashlib import sha256
from pathlib import Path
from typing import Any

import yaml
from jsonschema import Draft202012Validator
from jsonschema.exceptions import SchemaError
from pydantic import BaseModel, Field, ValidationError

_KEY = re.compile(r"[a-z][a-z0-9_.-]*\Z")
_VERSION = re.compile(r"[0-9]+\.[0-9]+\.[0-9]+\Z")
_SHA256 = re.compile(r"[0-9a-f]{64}\Z")


class SkillRegistryError(Exception):
    pass


class SkillVersionUnavailable(Exception):
    pass


class SkillMetadata(BaseModel):
    key: str
    version: str
    description: str
    default_model: str
    max_input_tokens: int = Field(gt=0)
    max_output_tokens: int = Field(gt=0)
    max_repair_rounds: int = Field(ge=0, le=2)
    timeout_s: int = Field(gt=0)
    tools: list[str] = Field(default_factory=list)
    validators: list[str] = Field(default_factory=lambda: ["schema"])


@dataclass(frozen=True)
class SkillDefinition:
    metadata: SkillMetadata
    content_hash: str
    instruction: str
    input_schema: dict[str, Any]
    output_schema: dict[str, Any]


def package_hash(package: Path) -> str:
    digest = sha256()
    files = sorted(path for path in package.rglob("*") if path.is_file())
    if not files:
        raise SkillRegistryError("skill package is empty")
    for path in files:
        if path.is_symlink():
            raise SkillRegistryError("skill package contains a symlink")
        digest.update(path.relative_to(package).as_posix().encode("utf-8"))
        digest.update(b"\x00")
        digest.update(path.read_bytes())
        digest.update(b"\x00")
    return digest.hexdigest()


def _load_package(package: Path, key: str, version: str, expected_hash: str) -> SkillDefinition:
    if package_hash(package) != expected_hash:
        raise SkillRegistryError(f"skill package hash mismatch: {key}@{version}")
    try:
        raw = (package / "SKILL.md").read_text(encoding="utf-8")
        if not raw.startswith("---\n"):
            raise ValueError("missing frontmatter")
        frontmatter, separator, instruction = raw[4:].partition("\n---\n")
        if not separator or not instruction.strip():
            raise ValueError("missing frontmatter or instruction")
        metadata = SkillMetadata.model_validate(yaml.safe_load(frontmatter))
        if metadata.key != key or metadata.version != version:
            raise ValueError("skill key or version differs from index")
        input_schema = json.loads((package / "input.schema.json").read_text(encoding="utf-8"))
        output_schema = json.loads((package / "output.schema.json").read_text(encoding="utf-8"))
        if not isinstance(input_schema, dict) or not isinstance(output_schema, dict):
            raise ValueError("skill schema must be an object")
        Draft202012Validator.check_schema(input_schema)
        Draft202012Validator.check_schema(output_schema)
    except (OSError, ValueError, ValidationError, SchemaError, yaml.YAMLError) as exc:
        raise SkillRegistryError(f"invalid skill metadata or schema: {key}@{version}") from exc
    return SkillDefinition(
        metadata, expected_hash, instruction.strip(), input_schema, output_schema
    )


class SkillRegistry:
    def __init__(self, skills: dict[tuple[str, str], SkillDefinition]) -> None:
        self._skills = skills.copy()

    @classmethod
    def load(cls, root: Path) -> "SkillRegistry":
        try:
            index = json.loads((root / "index.json").read_text(encoding="utf-8"))
        except (OSError, ValueError) as exc:
            raise SkillRegistryError("invalid skill index") from exc
        if not isinstance(index, dict):
            raise SkillRegistryError("skill index must be an object")
        skills: dict[tuple[str, str], SkillDefinition] = {}
        for key, versions in index.items():
            if (
                not isinstance(key, str)
                or not _KEY.fullmatch(key)
                or not isinstance(versions, dict)
            ):
                raise SkillRegistryError("invalid skill index entry")
            for version, expected_hash in versions.items():
                if (
                    not isinstance(version, str)
                    or not _VERSION.fullmatch(version)
                    or not isinstance(expected_hash, str)
                    or not _SHA256.fullmatch(expected_hash)
                ):
                    raise SkillRegistryError("invalid skill index version or hash")
                package = root / key / version
                skills[(key, version)] = _load_package(package, key, version, expected_hash)
        return cls(skills)

    def get(self, key: str, version: str) -> SkillDefinition:
        try:
            return self._skills[(key, version)]
        except KeyError:
            raise SkillVersionUnavailable(f"skill version unavailable: {key}@{version}") from None
