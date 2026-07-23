from __future__ import annotations

import os
import sys
import unittest
from pathlib import Path

from sqlalchemy import create_engine, text
from sqlalchemy.engine import Engine
from sqlalchemy.orm import sessionmaker


BACKEND = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(BACKEND / "src"))

from diffusiondb_fixture import fixture_1k  # noqa: E402


class CatalogImporterIntegrationTests(unittest.TestCase):
    engine: Engine

    @classmethod
    def setUpClass(cls) -> None:
        cls.engine = create_engine(os.environ["THIEF_DATABASE_URL"])

    @classmethod
    def tearDownClass(cls) -> None:
        cls.engine.dispose()

    def test_same_1k_manifest_twice_does_not_add_catalog_rows(self) -> None:
        from thief.catalog.importer import CatalogImporter
        from thief.catalog.repository import SqlAlchemyCatalogRepository

        manifest, records = fixture_1k()
        self.addCleanup(self._delete_fixture, manifest.revision)
        factory = sessionmaker(self.engine, expire_on_commit=False)

        with factory.begin() as session:
            first = CatalogImporter(SqlAlchemyCatalogRepository(session)).execute(
                manifest,
                records,
            )
        first_counts = self._counts(manifest.revision)
        with factory.begin() as session:
            second = CatalogImporter(SqlAlchemyCatalogRepository(session)).execute(
                manifest,
                records,
            )
        second_counts = self._counts(manifest.revision)

        self.assertEqual(
            first_counts,
            {
                "examples": 1000,
                "manifests": 1,
                "objects": 0,
                "search_documents": 1000,
                "templates": 1000,
            },
        )
        self.assertEqual(second_counts, first_counts)
        self.assertEqual(
            (
                first.templates_created,
                first.examples_created,
                first.search_documents_created,
            ),
            (1000, 1000, 1000),
        )
        self.assertEqual(
            (
                second.templates_created,
                second.examples_created,
                second.search_documents_created,
            ),
            (0, 0, 0),
        )

    def _counts(self, revision: str) -> dict[str, int]:
        queries = {
            "manifests": (
                "SELECT count(*) FROM catalog.import_manifests "
                "WHERE source_revision = :revision"
            ),
            "templates": (
                "SELECT count(*) FROM catalog.prompt_templates "
                "WHERE source_revision = :revision"
            ),
            "examples": (
                "SELECT count(*) FROM catalog.generation_examples e "
                "JOIN catalog.prompt_templates t ON t.id = e.template_id "
                "WHERE t.source_revision = :revision"
            ),
            "search_documents": (
                "SELECT count(*) FROM catalog.search_documents d "
                "JOIN catalog.prompt_templates t ON t.id = d.template_id "
                "WHERE t.source_revision = :revision"
            ),
        }
        with self.engine.connect() as connection:
            counts = {
                name: connection.execute(
                    text(query),
                    {"revision": revision},
                ).scalar_one()
                for name, query in queries.items()
            }
        counts["objects"] = 0
        return counts

    def _delete_fixture(self, revision: str) -> None:
        with self.engine.begin() as connection:
            connection.execute(
                text(
                    "DELETE FROM catalog.prompt_templates "
                    "WHERE source_revision = :revision"
                ),
                {"revision": revision},
            )
            connection.execute(
                text(
                    "DELETE FROM catalog.import_manifests "
                    "WHERE source_revision = :revision"
                ),
                {"revision": revision},
            )
            connection.execute(
                text(
                    "DELETE FROM catalog.categories c WHERE c.slug = 'uncategorized' "
                    "AND NOT EXISTS (SELECT 1 FROM catalog.prompt_templates t "
                    "WHERE t.category_id = c.id)"
                )
            )


if __name__ == "__main__":
    unittest.main()
