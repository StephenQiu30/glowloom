from __future__ import annotations

from dataclasses import dataclass
from typing import Protocol
from uuid import UUID

from thief.catalog.model import Category, GenerationExample, PromptTemplate


class InvalidCatalogQuery(ValueError):
    pass


@dataclass(frozen=True, slots=True)
class CatalogFilter:
    query: str | None = None
    category: str | None = None
    model: str | None = None
    aspect_ratio: str | None = None
    source: str | None = None
    cursor: str | None = None
    limit: int = 20


@dataclass(frozen=True, slots=True)
class CatalogTemplate:
    template: PromptTemplate
    examples: tuple[GenerationExample, ...] = ()


@dataclass(frozen=True, slots=True)
class CatalogPage:
    items: tuple[CatalogTemplate, ...]
    next_cursor: str | None


@dataclass(frozen=True, slots=True)
class CategorySummary:
    category: Category
    template_count: int


class CatalogQueries(Protocol):
    def list_templates(self, filters: CatalogFilter) -> CatalogPage: ...

    def get_template(self, template_id: UUID) -> CatalogTemplate | None: ...

    def list_categories(self) -> tuple[CategorySummary, ...]: ...
