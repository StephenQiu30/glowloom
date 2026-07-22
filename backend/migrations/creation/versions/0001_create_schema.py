"""Create the creation schema."""

from alembic import op


revision = "creation_0001"
down_revision = None
branch_labels = ("creation",)
depends_on = None


def upgrade() -> None:
    op.execute('CREATE SCHEMA "creation"')


def downgrade() -> None:
    op.execute('DROP SCHEMA "creation"')
