from __future__ import annotations

import os
import unittest

from sqlalchemy import create_engine, text
from sqlalchemy.engine import Engine

from tools.architecture import BUSINESS_MODULES


class MigrationIntegrationTests(unittest.TestCase):
    engine: Engine

    @classmethod
    def setUpClass(cls) -> None:
        cls.engine = create_engine(os.environ["THIEF_DATABASE_URL"])

    @classmethod
    def tearDownClass(cls) -> None:
        cls.engine.dispose()

    def test_current_business_schemas_have_one_platform_revision(self) -> None:
        with self.engine.connect() as connection:
            schemas = set(
                connection.execute(
                    text(
                        "SELECT schema_name FROM information_schema.schemata "
                        "WHERE schema_name = ANY(:schemas)"
                    ),
                    {"schemas": list(BUSINESS_MODULES)},
                ).scalars()
            )
            revisions = set(
                connection.execute(
                    text("SELECT version_num FROM public.alembic_version")
                ).scalars()
            )

        self.assertEqual(schemas, set(BUSINESS_MODULES))
        self.assertEqual(revisions, {"platform_0002"})

    def test_identity_and_catalog_own_their_business_tables(self) -> None:
        with self.engine.connect() as connection:
            tables = set(
                connection.execute(
                    text(
                        "SELECT table_schema, table_name "
                        "FROM information_schema.tables "
                        "WHERE table_schema = ANY(:schemas)"
                    ),
                    {"schemas": list(BUSINESS_MODULES)},
                )
            )

        self.assertEqual(
            tables,
            {
                ("identity", "users"),
                ("identity", "invitations"),
                ("identity", "sessions"),
                ("catalog", "categories"),
                ("catalog", "prompt_templates"),
                ("catalog", "generation_examples"),
                ("catalog", "import_manifests"),
                ("catalog", "search_documents"),
            },
        )

    def test_identity_tables_enforce_role_owner_and_token_constraints(self) -> None:
        expected = {
            "ck_identity_users_email_normalized",
            "ck_identity_users_role",
            "ck_identity_invitations_email_normalized",
            "ck_identity_invitations_role",
            "ck_identity_invitations_expiry",
            "ck_identity_invitations_single_resolution",
            "ck_identity_sessions_expiry",
            "fk_identity_invitations_invited_by_users",
            "fk_identity_sessions_user_id_users",
            "uq_identity_users_email",
            "uq_identity_invitations_token_hash",
            "uq_identity_sessions_token_hash",
        }
        with self.engine.connect() as connection:
            constraints = set(
                connection.execute(
                    text(
                        "SELECT constraint_name FROM information_schema.table_constraints "
                        "WHERE table_schema = 'identity'"
                    )
                ).scalars()
            )

        self.assertTrue(expected <= constraints, expected - constraints)

    def test_catalog_tables_enforce_publication_and_deduplication(self) -> None:
        expected = {
            "ck_catalog_prompt_templates_status",
            "ck_catalog_prompt_templates_content_hash",
            "fk_catalog_prompt_templates_category_id_categories",
            "fk_catalog_generation_examples_template_id_prompt_templates",
            "uq_catalog_categories_slug",
            "uq_catalog_prompt_templates_slug",
            "uq_catalog_prompt_templates_content_hash",
            "uq_catalog_generation_examples_template_asset",
            "uq_catalog_import_manifests_source_revision_checksum",
            "fk_catalog_search_documents_template_id_prompt_templates",
        }
        with self.engine.connect() as connection:
            constraints = set(
                connection.execute(
                    text(
                        "SELECT constraint_name FROM information_schema.table_constraints "
                        "WHERE table_schema = 'catalog'"
                    )
                ).scalars()
            )

        self.assertTrue(expected <= constraints, expected - constraints)

        with self.engine.connect() as connection:
            parameters_type = connection.execute(
                text(
                    "SELECT data_type FROM information_schema.columns "
                    "WHERE table_schema = 'catalog' "
                    "AND table_name = 'prompt_templates' "
                    "AND column_name = 'parameters'"
                )
            ).scalar_one()

        self.assertEqual(parameters_type, "jsonb")


if __name__ == "__main__":
    unittest.main()
