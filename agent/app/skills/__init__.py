"""Explicit, read-only Skill registrations used by the Agent runtime."""

from app.skills.catalog import SkillCatalog, SkillRegistration, SkillUnavailable

__all__ = ["SkillCatalog", "SkillRegistration", "SkillUnavailable"]
