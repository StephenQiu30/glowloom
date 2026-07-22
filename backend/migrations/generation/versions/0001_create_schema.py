"""Create the generation schema."""

from alembic import op


revision = "generation_0001"
down_revision = None
branch_labels = ("generation",)
depends_on = None


def upgrade() -> None:
    op.execute('CREATE SCHEMA "generation"')


def downgrade() -> None:
    op.execute('DROP SCHEMA "generation"')
