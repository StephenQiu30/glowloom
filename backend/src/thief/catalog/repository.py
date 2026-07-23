from __future__ import annotations

import json
from datetime import datetime
from typing import Any, cast
from uuid import UUID

from sqlalchemy import text
from sqlalchemy.orm import Session

from thief.catalog.import_repository import SqlAlchemyCatalogImportRepository
from thief.catalog.model import (
    Category,
    GenerationExample,
    PromptTemplate,
    SourceAttribution,
    TemplateStatus,
)


class SqlAlchemyCatalogRepository(SqlAlchemyCatalogImportRepository):
    def __init__(self, session: Session) -> None:
        self._session = session

    def add_category(self, category: Category) -> None:
        self._session.execute(
            text(
                "INSERT INTO catalog.categories (id, slug, name) "
                "VALUES (:id, :slug, :name)"
            ),
            {"id": category.id, "slug": category.slug, "name": category.name},
        )

    def add_template(self, template: PromptTemplate) -> None:
        self._session.execute(
            text(
                "INSERT INTO catalog.prompt_templates "
                "(id, slug, title, prompt, negative_prompt, source_model, "
                "aspect_ratio, parameters, category_id, source_name, source_url, "
                "source_object_id, source_revision, source_license, collected_at, "
                "content_hash, status, published_at) VALUES "
                "(:id, :slug, :title, :prompt, :negative_prompt, :source_model, "
                ":aspect_ratio, CAST(:parameters AS JSONB), :category_id, "
                ":source_name, :source_url, "
                ":source_object_id, :source_revision, :source_license, :collected_at, "
                ":content_hash, :status, :published_at)"
            ),
            {
                "id": template.id,
                "slug": template.slug,
                "title": template.title,
                "prompt": template.prompt,
                "negative_prompt": template.negative_prompt,
                "source_model": template.source_model,
                "aspect_ratio": template.aspect_ratio,
                "parameters": json.dumps(template.parameters),
                "category_id": template.category_id,
                "source_name": template.source.name,
                "source_url": template.source.url,
                "source_object_id": template.source.object_id,
                "source_revision": template.source.revision,
                "source_license": template.source.license,
                "collected_at": template.source.collected_at,
                "content_hash": template.content_hash,
                "status": template.status.value,
                "published_at": template.published_at,
            },
        )

    def add_example(self, example: GenerationExample) -> None:
        self._session.execute(
            text(
                "INSERT INTO catalog.generation_examples "
                "(id, template_id, asset_id, alt_text, position) "
                "VALUES (:id, :template_id, :asset_id, :alt_text, :position)"
            ),
            {
                "id": example.id,
                "template_id": example.template_id,
                "asset_id": example.asset_id,
                "alt_text": example.alt_text,
                "position": example.position,
            },
        )

    def find_template(self, template_id: UUID) -> PromptTemplate | None:
        row = (
            self._session.execute(
                text(
                    "SELECT id, slug, title, prompt, negative_prompt, source_model, "
                    "aspect_ratio, parameters, category_id, source_name, source_url, "
                    "source_object_id, source_revision, source_license, collected_at, "
                    "content_hash, status, published_at "
                    "FROM catalog.prompt_templates WHERE id = :id"
                ),
                {"id": template_id},
            )
            .mappings()
            .one_or_none()
        )
        return _template_from_row(row)


def _template_from_row(row: Any) -> PromptTemplate | None:
    if row is None:
        return None
    return PromptTemplate(
        id=cast(UUID, row["id"]),
        slug=str(row["slug"]),
        title=str(row["title"]),
        prompt=str(row["prompt"]),
        negative_prompt=cast(str | None, row["negative_prompt"]),
        source_model=str(row["source_model"]),
        aspect_ratio=str(row["aspect_ratio"]),
        parameters=cast(dict[str, int | float | str], row["parameters"]),
        category_id=cast(UUID, row["category_id"]),
        source=SourceAttribution(
            name=str(row["source_name"]),
            url=str(row["source_url"]),
            object_id=str(row["source_object_id"]),
            revision=str(row["source_revision"]),
            license=str(row["source_license"]),
            collected_at=cast(datetime, row["collected_at"]),
        ),
        content_hash=str(row["content_hash"]),
        status=TemplateStatus(str(row["status"])),
        published_at=cast(datetime | None, row["published_at"]),
    )
