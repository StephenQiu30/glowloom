"""Create the ingestion schema."""

from alembic import op


revision = "ingestion_0001"
down_revision = None
branch_labels = ("ingestion",)
depends_on = None


def upgrade() -> None:
    op.execute('CREATE SCHEMA "ingestion"')


def downgrade() -> None:
    op.execute('DROP SCHEMA "ingestion"')
