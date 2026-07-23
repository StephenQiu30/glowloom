from __future__ import annotations

import os
import sys
import unittest
from datetime import UTC, datetime, timedelta
from pathlib import Path
from uuid import UUID

from sqlalchemy import create_engine, text
from sqlalchemy.engine import Engine
from sqlalchemy.orm import sessionmaker


BACKEND = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(BACKEND / "src"))

from thief.catalog.model import (  # noqa: E402
    Category,
    GenerationExample,
    PromptTemplate,
    SourceAttribution,
    TemplateStatus,
)
from thief.catalog.query import CatalogFilter  # noqa: E402
from thief.catalog.query_repository import (  # noqa: E402
    SqlAlchemyCatalogQueryRepository,
)
from thief.catalog.repository import SqlAlchemyCatalogRepository  # noqa: E402


class CatalogQueryIntegrationTests(unittest.TestCase):
    engine: Engine

    @classmethod
    def setUpClass(cls) -> None:
        cls.engine = create_engine(os.environ["THIEF_DATABASE_URL"])
        cls.factory = sessionmaker(cls.engine, expire_on_commit=False)

    @classmethod
    def tearDownClass(cls) -> None:
        cls.engine.dispose()

    def setUp(self) -> None:
        self.now = datetime(2026, 7, 23, 8, 0, tzinfo=UTC)
        self.portrait = Category(_id(201), "portrait", "Portrait")
        self.landscape = Category(_id(202), "landscape", "Landscape")
        self.empty = Category(_id(203), "empty", "Empty")
        self.newer = self._template(
            211, self.portrait, "cinematic-portrait", "cinematic portrait", "sdxl",
            "1:1", "source-a", self.now,
        )
        self.older = self._template(
            212, self.landscape, "misty-mountain", "misty mountain", "sd15",
            "16:9", "source-b", self.now - timedelta(minutes=1),
        )
        self.suppressed = self._template(
            213, self.portrait, "hidden", "cinematic hidden", "sdxl", "1:1",
            "source-a", None,
        )
        self._insert_fixture()
        self.addCleanup(self._delete_fixture)

    def test_lists_only_published_templates_with_stable_cursor(self) -> None:
        queries = SqlAlchemyCatalogQueryRepository(self.factory)

        first = queries.list_templates(CatalogFilter(limit=1))
        second = queries.list_templates(CatalogFilter(limit=1, cursor=first.next_cursor))

        self.assertEqual([item.template.id for item in first.items], [self.newer.id])
        self.assertIsNotNone(first.next_cursor)
        self.assertEqual([item.template.id for item in second.items], [self.older.id])
        self.assertIsNone(second.next_cursor)

    def test_applies_search_and_all_exact_filters(self) -> None:
        queries = SqlAlchemyCatalogQueryRepository(self.factory)
        filters = (
            CatalogFilter(query="cinematic"),
            CatalogFilter(category="portrait"),
            CatalogFilter(model="sdxl"),
            CatalogFilter(aspect_ratio="1:1"),
            CatalogFilter(source="source-a"),
        )

        for value in filters:
            page = queries.list_templates(value)
            self.assertEqual([item.template.id for item in page.items], [self.newer.id])

    def test_detail_examples_and_categories_hide_unpublished_rows(self) -> None:
        queries = SqlAlchemyCatalogQueryRepository(self.factory)

        detail = queries.get_template(self.newer.id)
        categories = queries.list_categories()

        self.assertIsNotNone(detail)
        self.assertEqual([example.asset_id for example in detail.examples], [_id(221)])
        self.assertIsNone(queries.get_template(self.suppressed.id))
        self.assertEqual(
            [(item.category.slug, item.template_count) for item in categories],
            [("landscape", 1), ("portrait", 1)],
        )

    def _template(
        self, number: int, category: Category, slug: str, prompt: str, model: str,
        ratio: str, source: str, published_at: datetime | None,
    ) -> PromptTemplate:
        return PromptTemplate(
            id=_id(number), slug=slug, title=slug.replace("-", " ").title(),
            prompt=prompt, negative_prompt=None, source_model=model,
            aspect_ratio=ratio, parameters={"seed": number}, category_id=category.id,
            source=SourceAttribution(
                source, f"https://example.test/{number}", str(number), "fixture-v1",
                "test-fixture", self.now,
            ),
            content_hash=f"{number:064x}",
            status=(TemplateStatus.PUBLISHED if published_at else TemplateStatus.SUPPRESSED),
            published_at=published_at,
        )

    def _insert_fixture(self) -> None:
        with self.factory.begin() as session:
            repository = SqlAlchemyCatalogRepository(session)
            for category in (self.portrait, self.landscape, self.empty):
                repository.add_category(category)
            for template in (self.newer, self.older, self.suppressed):
                repository.add_template(template)
                session.execute(
                    text(
                        "INSERT INTO catalog.search_documents "
                        "(template_id, search_text) VALUES (:id, :text)"
                    ),
                    {"id": template.id, "text": template.prompt},
                )
            repository.add_example(
                GenerationExample(_id(222), self.newer.id, _id(221), "Example", 0)
            )

    def _delete_fixture(self) -> None:
        with self.engine.begin() as connection:
            connection.execute(
                text("DELETE FROM catalog.prompt_templates WHERE id = ANY(:ids)"),
                {"ids": [self.newer.id, self.older.id, self.suppressed.id]},
            )
            connection.execute(
                text("DELETE FROM catalog.categories WHERE id = ANY(:ids)"),
                {"ids": [self.portrait.id, self.landscape.id, self.empty.id]},
            )


def _id(number: int) -> UUID:
    return UUID(f"00000000-0000-4000-8000-{number:012d}")


if __name__ == "__main__":
    unittest.main()
