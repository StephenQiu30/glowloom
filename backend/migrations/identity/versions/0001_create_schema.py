"""Create the identity schema."""

from alembic import op


revision = "identity_0001"
down_revision = None
branch_labels = ("identity",)
depends_on = None


def upgrade() -> None:
    op.execute('CREATE SCHEMA "identity"')


def downgrade() -> None:
    op.execute('DROP SCHEMA "identity"')
