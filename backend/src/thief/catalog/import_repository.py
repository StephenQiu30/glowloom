from __future__ import annotations

import json
from typing import Any

from sqlalchemy import text
from sqlalchemy.engine import CursorResult
from sqlalchemy.orm import Session

from thief.catalog.importer import CatalogImportBatch, ImportReport
from thief.catalog.model import PromptTemplate


class SqlAlchemyCatalogImportRepository:
    _session: Session

    def import_batch(self, batch: CatalogImportBatch) -> ImportReport:
        manifest = batch.manifest
        manifest_created = _count(
            self._session.execute(
                text(
                    "INSERT INTO catalog.import_manifests "
                    "(id, source_name, source_url, source_revision, source_license, "
                    "collected_at, item_count, checksum) VALUES "
                    "(:id, :name, :url, :revision, :license, :collected_at, "
                    ":item_count, :checksum) ON CONFLICT DO NOTHING"
                ),
                {
                    "id": batch.id,
                    "name": manifest.source_name,
                    "url": manifest.source_url,
                    "revision": manifest.revision,
                    "license": manifest.license,
                    "collected_at": manifest.collected_at,
                    "item_count": manifest.item_count,
                    "checksum": manifest.checksum,
                },
            )
        )
        self._session.execute(
            text(
                "INSERT INTO catalog.categories (id, slug, name) "
                "VALUES (:id, :slug, :name) ON CONFLICT DO NOTHING"
            ),
            {
                "id": batch.category.id,
                "slug": batch.category.slug,
                "name": batch.category.name,
            },
        )
        templates_created = _count(
            self._session.execute(
                text(
                    "INSERT INTO catalog.prompt_templates "
                    "(id, slug, title, prompt, negative_prompt, source_model, "
                    "aspect_ratio, parameters, category_id, source_name, source_url, "
                    "source_object_id, source_revision, source_license, collected_at, "
                    "content_hash, status, published_at) VALUES "
                    "(:id, :slug, :title, :prompt, :negative_prompt, :source_model, "
                    ":aspect_ratio, CAST(:parameters AS JSONB), :category_id, "
                    ":source_name, :source_url, :source_object_id, :source_revision, "
                    ":source_license, :collected_at, :content_hash, :status, "
                    ":published_at) ON CONFLICT DO NOTHING"
                ),
                [_template_values(template) for template in batch.templates],
            )
        )
        examples_created = _count(
            self._session.execute(
                text(
                    "INSERT INTO catalog.generation_examples "
                    "(id, template_id, asset_id, alt_text, position) VALUES "
                    "(:id, :template_id, :asset_id, :alt_text, :position) "
                    "ON CONFLICT DO NOTHING"
                ),
                [
                    {
                        "id": example.id,
                        "template_id": example.template_id,
                        "asset_id": example.asset_id,
                        "alt_text": example.alt_text,
                        "position": example.position,
                    }
                    for example in batch.examples
                ],
            )
        )
        search_documents_created = _count(
            self._session.execute(
                text(
                    "INSERT INTO catalog.search_documents "
                    "(template_id, search_text) VALUES (:template_id, :search_text) "
                    "ON CONFLICT DO NOTHING"
                ),
                [
                    {
                        "template_id": document.template_id,
                        "search_text": document.search_text,
                    }
                    for document in batch.search_documents
                ],
            )
        )
        return ImportReport(
            manifest_created=manifest_created == 1,
            templates_created=templates_created,
            examples_created=examples_created,
            search_documents_created=search_documents_created,
        )


def _template_values(template: PromptTemplate) -> dict[str, object]:
    return {
        "id": template.id,
        "slug": template.slug,
        "title": template.title,
        "prompt": template.prompt,
        "negative_prompt": template.negative_prompt,
        "source_model": template.source_model,
        "aspect_ratio": template.aspect_ratio,
        "parameters": json.dumps(template.parameters, separators=(",", ":")),
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
    }


def _count(result: Any) -> int:
    return int(result.rowcount) if isinstance(result, CursorResult) else 0
