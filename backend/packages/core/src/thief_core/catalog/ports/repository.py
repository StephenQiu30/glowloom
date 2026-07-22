from __future__ import annotations

from typing import Protocol
from uuid import UUID

from thief_core.catalog.domain import Category, GenerationExample, PromptTemplate


class CatalogRepository(Protocol):
    def add_category(self, category: Category) -> None: ...

    def add_template(self, template: PromptTemplate) -> None: ...

    def add_example(self, example: GenerationExample) -> None: ...

    def find_template(self, template_id: UUID) -> PromptTemplate | None: ...
