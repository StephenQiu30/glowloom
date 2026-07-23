from __future__ import annotations

import re
from dataclasses import dataclass
from datetime import datetime
from enum import StrEnum
from uuid import UUID


_SLUG = re.compile(r"[a-z0-9]+(?:-[a-z0-9]+)*")


class TemplateStatus(StrEnum):
    PUBLISHED = "published"
    SUPPRESSED = "suppressed"
    DELETED = "deleted"


def normalize_slug(value: str) -> str:
    normalized = "-".join(value.strip().lower().split())
    if not normalized:
        raise ValueError("slug is required")
    if not _SLUG.fullmatch(normalized):
        raise ValueError("slug contains unsupported characters")
    return normalized


@dataclass(frozen=True, slots=True)
class SourceAttribution:
    name: str
    url: str
    object_id: str
    revision: str
    license: str
    collected_at: datetime


@dataclass(frozen=True, slots=True)
class Category:
    id: UUID
    slug: str
    name: str


@dataclass(frozen=True, slots=True)
class PromptTemplate:
    id: UUID
    slug: str
    title: str
    prompt: str
    negative_prompt: str | None
    source_model: str
    aspect_ratio: str
    parameters: dict[str, int | float | str]
    category_id: UUID
    source: SourceAttribution
    content_hash: str
    status: TemplateStatus
    published_at: datetime | None

    def is_public(self) -> bool:
        return self.status is TemplateStatus.PUBLISHED


@dataclass(frozen=True, slots=True)
class GenerationExample:
    id: UUID
    template_id: UUID
    asset_id: UUID
    alt_text: str
    position: int
