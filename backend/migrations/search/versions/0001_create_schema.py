"""Create the search schema."""

from alembic import op


revision = "search_0001"
down_revision = None
branch_labels = ("search",)
depends_on = None


def upgrade() -> None:
    op.execute('CREATE SCHEMA "search"')


def downgrade() -> None:
    op.execute('DROP SCHEMA "search"')
