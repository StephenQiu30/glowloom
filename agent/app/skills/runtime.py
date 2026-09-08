"""Read-only Skill runtime facade owned by the Agent application."""

from __future__ import annotations

from dataclasses import dataclass

from app.skills.catalog import SkillCatalog


@dataclass(frozen=True)
class SkillRuntime:
    """Resolve only the releases declared by one application-owned catalog."""

    catalog: SkillCatalog

    def verify_all(self) -> dict[str, str]:
        return self.catalog.verify_all()

    def load(self, key: str) -> object:
        return self.catalog.load(key)
