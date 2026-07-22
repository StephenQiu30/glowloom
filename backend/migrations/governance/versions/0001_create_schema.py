"""Create the governance schema."""

from alembic import op


revision = "governance_0001"
down_revision = None
branch_labels = ("governance",)
depends_on = None


def upgrade() -> None:
    op.execute('CREATE SCHEMA "governance"')


def downgrade() -> None:
    op.execute('DROP SCHEMA "governance"')
