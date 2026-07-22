"""Create the catalog schema."""

from alembic import op


revision = "catalog_0001"
down_revision = None
branch_labels = ("catalog",)
depends_on = None


def upgrade() -> None:
    op.execute('CREATE SCHEMA "catalog"')


def downgrade() -> None:
    op.execute('DROP SCHEMA "catalog"')
