from __future__ import annotations

import base64
import json
from datetime import datetime
from typing import Any, cast
from uuid import UUID

from sqlalchemy import text
from sqlalchemy.orm import Session, sessionmaker

from thief.catalog.model import Category, GenerationExample
from thief.catalog.query import (
    CatalogFilter,
    CatalogPage,
    CatalogTemplate,
    CategorySummary,
    InvalidCatalogQuery,
)
from thief.catalog.repository import template_from_row


_TEMPLATE_FIELDS = (
    "t.id, t.slug, t.title, t.prompt, t.negative_prompt, t.source_model, "
    "t.aspect_ratio, t.parameters, t.category_id, t.source_name, t.source_url, "
    "t.source_object_id, t.source_revision, t.source_license, t.collected_at, "
    "t.content_hash, t.status, t.published_at"
)


class SqlAlchemyCatalogQueryRepository:
    def __init__(self, factory: sessionmaker[Session]) -> None:
        self._factory = factory

    def list_templates(self, filters: CatalogFilter) -> CatalogPage:
        if not 1 <= filters.limit <= 50:
            raise InvalidCatalogQuery("limit must be between 1 and 50")
        clauses = ["t.status = 'published'", "t.published_at IS NOT NULL"]
        parameters: dict[str, object] = {"fetch_limit": filters.limit + 1}
        join_search = bool(filters.query)
        exact_filters = {
            "category": ("c.slug", filters.category),
            "model": ("t.source_model", filters.model),
            "aspect_ratio": ("t.aspect_ratio", filters.aspect_ratio),
            "source": ("t.source_name", filters.source),
        }
        for name, (column, value) in exact_filters.items():
            if value is not None:
                clauses.append(f"{column} = :{name}")
                parameters[name] = value
        if filters.query:
            clauses.append(
                "d.search_vector @@ websearch_to_tsquery('simple', :query)"
            )
            parameters["query"] = filters.query
        if filters.cursor:
            published_at, template_id = _decode_cursor(filters.cursor)
            clauses.append("(t.published_at, t.id) < (:published_at, :template_id)")
            parameters.update(
                {"published_at": published_at, "template_id": template_id}
            )
        joins = "JOIN catalog.categories c ON c.id = t.category_id "
        if join_search:
            joins += "JOIN catalog.search_documents d ON d.template_id = t.id "
        statement = (
            f"SELECT {_TEMPLATE_FIELDS} FROM catalog.prompt_templates t {joins}"
            f"WHERE {' AND '.join(clauses)} "
            "ORDER BY t.published_at DESC, t.id DESC LIMIT :fetch_limit"
        )
        with self._factory() as session:
            rows = session.execute(text(statement), parameters).mappings().all()
        has_more = len(rows) > filters.limit
        selected = rows[: filters.limit]
        items = tuple(
            CatalogTemplate(template=cast(Any, template_from_row(row)))
            for row in selected
        )
        next_cursor = None
        if has_more and selected:
            last = selected[-1]
            next_cursor = _encode_cursor(
                cast(datetime, last["published_at"]), cast(UUID, last["id"])
            )
        return CatalogPage(items=items, next_cursor=next_cursor)

    def get_template(self, template_id: UUID) -> CatalogTemplate | None:
        with self._factory() as session:
            row = (
                session.execute(
                    text(
                        f"SELECT {_TEMPLATE_FIELDS} "
                        "FROM catalog.prompt_templates t "
                        "WHERE t.id = :id AND t.status = 'published' "
                        "AND t.published_at IS NOT NULL"
                    ),
                    {"id": template_id},
                )
                .mappings()
                .one_or_none()
            )
            if row is None:
                return None
            examples = session.execute(
                text(
                    "SELECT id, template_id, asset_id, alt_text, position "
                    "FROM catalog.generation_examples WHERE template_id = :id "
                    "ORDER BY position, id"
                ),
                {"id": template_id},
            ).mappings()
            return CatalogTemplate(
                template=cast(Any, template_from_row(row)),
                examples=tuple(_example_from_row(example) for example in examples),
            )

    def list_categories(self) -> tuple[CategorySummary, ...]:
        with self._factory() as session:
            rows = session.execute(
                text(
                    "SELECT c.id, c.slug, c.name, count(*) AS template_count "
                    "FROM catalog.categories c JOIN catalog.prompt_templates t "
                    "ON t.category_id = c.id WHERE t.status = 'published' "
                    "AND t.published_at IS NOT NULL GROUP BY c.id, c.slug, c.name "
                    "ORDER BY c.slug"
                )
            ).mappings()
            return tuple(
                CategorySummary(
                    category=Category(
                        cast(UUID, row["id"]), str(row["slug"]), str(row["name"])
                    ),
                    template_count=int(row["template_count"]),
                )
                for row in rows
            )


def _encode_cursor(published_at: datetime, template_id: UUID) -> str:
    raw = json.dumps(
        {"published_at": published_at.isoformat(), "id": str(template_id)},
        separators=(",", ":"),
    ).encode()
    return base64.urlsafe_b64encode(raw).decode().rstrip("=")


def _decode_cursor(cursor: str) -> tuple[datetime, UUID]:
    try:
        padding = "=" * (-len(cursor) % 4)
        payload = json.loads(base64.urlsafe_b64decode(cursor + padding))
        published_at = datetime.fromisoformat(payload["published_at"])
        template_id = UUID(payload["id"])
        if published_at.tzinfo is None or set(payload) != {"published_at", "id"}:
            raise ValueError
        return published_at, template_id
    except (KeyError, TypeError, ValueError, json.JSONDecodeError) as error:
        raise InvalidCatalogQuery("cursor is invalid") from error


def _example_from_row(row: Any) -> GenerationExample:
    return GenerationExample(
        id=cast(UUID, row["id"]),
        template_id=cast(UUID, row["template_id"]),
        asset_id=cast(UUID, row["asset_id"]),
        alt_text=str(row["alt_text"]),
        position=int(row["position"]),
    )
