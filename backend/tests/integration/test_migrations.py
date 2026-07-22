from __future__ import annotations

import os
import unittest

from sqlalchemy import create_engine, text
from sqlalchemy.engine import Engine

from tools.architecture import MODULES


class MigrationIntegrationTests(unittest.TestCase):
    engine: Engine

    @classmethod
    def setUpClass(cls) -> None:
        cls.engine = create_engine(os.environ["THIEF_DATABASE_URL"])

    @classmethod
    def tearDownClass(cls) -> None:
        cls.engine.dispose()

    def test_all_module_schemas_and_revisions_exist(self) -> None:
        with self.engine.connect() as connection:
            schemas = set(
                connection.execute(
                    text(
                        "SELECT schema_name FROM information_schema.schemata "
                        "WHERE schema_name = ANY(:schemas)"
                    ),
                    {"schemas": list(MODULES)},
                ).scalars()
            )
            revisions = set(
                connection.execute(
                    text("SELECT version_num FROM public.alembic_version")
                ).scalars()
            )

        self.assertEqual(schemas, set(MODULES))
        expected_revisions = {f"{module}_0001" for module in MODULES}
        expected_revisions.remove("identity_0001")
        expected_revisions.add("identity_0002")
        self.assertEqual(revisions, expected_revisions)

    def test_only_identity_has_business_tables_after_s1_t01(self) -> None:
        with self.engine.connect() as connection:
            tables = set(
                connection.execute(
                    text(
                        "SELECT table_schema, table_name "
                        "FROM information_schema.tables "
                        "WHERE table_schema = ANY(:schemas)"
                    ),
                    {"schemas": list(MODULES)},
                )
            )

        self.assertEqual(
            tables,
            {
                ("identity", "users"),
                ("identity", "invitations"),
                ("identity", "sessions"),
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


if __name__ == "__main__":
    unittest.main()
