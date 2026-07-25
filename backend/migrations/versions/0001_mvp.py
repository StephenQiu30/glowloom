from __future__ import annotations

from pathlib import Path

from alembic import op

revision = "0001_mvp"
down_revision = None
branch_labels = None
depends_on = None

ROOT = Path(__file__).resolve().parents[3]
SQL_PATTERN = "sql/[0-9][0-9]_*.sql"


def reviewed_sql_files() -> list[Path]:
    files = sorted(ROOT.glob(SQL_PATTERN))
    if len(files) != 20:
        raise RuntimeError(f"expected 20 reviewed SQL files, found {len(files)}")
    return files


def upgrade() -> None:
    for path in reviewed_sql_files():
        for statement in path.read_text().split(";"):
            if statement.strip():
                op.execute(statement)


def downgrade() -> None:
    raise RuntimeError("0001_mvp is forward-only and preserves business facts")
