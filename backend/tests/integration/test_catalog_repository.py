from __future__ import annotations

import os
import sys
import unittest
from datetime import UTC, datetime
from pathlib import Path
from uuid import uuid4

from sqlalchemy import create_engine, text
from sqlalchemy.engine import Engine
from sqlalchemy.orm import sessionmaker


BACKEND = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(BACKEND / "src"))


class CatalogRepositoryIntegrationTests(unittest.TestCase):
    engine: Engine

    @classmethod
    def setUpClass(cls) -> None:
        cls.engine = create_engine(os.environ["THIEF_DATABASE_URL"])

    @classmethod
    def tearDownClass(cls) -> None:
        cls.engine.dispose()

    def test_catalog_facts_round_trip_without_cross_schema_relationships(self) -> None:
        from thief.catalog.model import (
            Category,
            GenerationExample,
            PromptTemplate,
            SourceAttribution,
            TemplateStatus,
        )
        from thief.catalog.repository import SqlAlchemyCatalogRepository

        now = datetime.now(UTC)
        category = Category(id=uuid4(), slug=f"portrait-{uuid4()}", name="Portrait")
        template = PromptTemplate(
            id=uuid4(),
            slug=f"cinematic-{uuid4()}",
            title="Cinematic portrait",
            prompt="a cinematic portrait",
            negative_prompt="blurry",
            source_model="stable-diffusion",
            aspect_ratio="1:1",
            parameters={"seed": 42},
            category_id=category.id,
            source=SourceAttribution(
                name="fixture",
                url="https://example.test/items/42",
                object_id=str(uuid4()),
                revision="revision-1",
                license="test-fixture",
                collected_at=now,
            ),
            content_hash=uuid4().hex + uuid4().hex,
            status=TemplateStatus.PUBLISHED,
            published_at=now,
        )
        example = GenerationExample(
            id=uuid4(),
            template_id=template.id,
            asset_id=uuid4(),
            alt_text="Cinematic portrait example",
            position=0,
        )
        self.addCleanup(self._delete_fixture, template.id, category.id)

        factory = sessionmaker(self.engine, expire_on_commit=False)
        with factory.begin() as session:
            repository = SqlAlchemyCatalogRepository(session)
            repository.add_category(category)
            repository.add_template(template)
            repository.add_example(example)

        with factory() as session:
            stored = SqlAlchemyCatalogRepository(session).find_template(template.id)

        self.assertEqual(stored, template)
        with self.engine.connect() as connection:
            asset_foreign_keys = connection.execute(
                text(
                    "SELECT count(*) FROM information_schema.table_constraints "
                    "WHERE table_schema = 'catalog' "
                    "AND table_name = 'generation_examples' "
                    "AND constraint_type = 'FOREIGN KEY' "
                    "AND constraint_name LIKE '%asset%'"
                )
            ).scalar_one()
        self.assertEqual(asset_foreign_keys, 0)

    def _delete_fixture(self, template_id: object, category_id: object) -> None:
        with self.engine.begin() as connection:
            connection.execute(
                text("DELETE FROM catalog.generation_examples WHERE template_id = :id"),
                {"id": template_id},
            )
            connection.execute(
                text("DELETE FROM catalog.prompt_templates WHERE id = :id"),
                {"id": template_id},
            )
            connection.execute(
                text("DELETE FROM catalog.categories WHERE id = :id"),
                {"id": category_id},
            )


if __name__ == "__main__":
    unittest.main()
