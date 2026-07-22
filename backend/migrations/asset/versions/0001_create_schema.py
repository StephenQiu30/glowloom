"""Create the asset schema."""

from alembic import op


revision = "asset_0001"
down_revision = None
branch_labels = ("asset",)
depends_on = None


def upgrade() -> None:
    op.execute('CREATE SCHEMA "asset"')


def downgrade() -> None:
    op.execute('DROP SCHEMA "asset"')
