import asyncio
from argparse import ArgumentParser

from app.core.database import engine
from app.core.migrations import adopt_existing_database, validate_backup_reference
from app.model_registry import register_implemented_models


def _parse_backup_reference() -> str:
    parser = ArgumentParser(
        description=("Adopt a structurally identical pre-Alembic database after a verified backup.")
    )
    parser.add_argument(
        "--backup-reference",
        required=True,
        help="Operator-owned backup or snapshot reference recorded for recovery.",
    )
    return str(parser.parse_args().backup_reference)


async def main(backup_reference: str) -> None:
    backup_reference = validate_backup_reference(backup_reference)
    register_implemented_models()
    await adopt_existing_database(engine, backup_reference=backup_reference)
    await engine.dispose()
    print(f"database baseline adopted; backup_reference={backup_reference}")


if __name__ == "__main__":
    asyncio.run(main(_parse_backup_reference()))
