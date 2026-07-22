from __future__ import annotations

import os

from alembic import context
from sqlalchemy import engine_from_config, pool


config = context.config
target_metadata = None


def database_url() -> str:
    try:
        return os.environ["THIEF_DATABASE_URL"]
    except KeyError as error:
        raise RuntimeError("THIEF_DATABASE_URL is required") from error


def run_migrations_offline() -> None:
    context.configure(
        url=database_url(),
        target_metadata=target_metadata,
        literal_binds=True,
        dialect_opts={"paramstyle": "named"},
    )
    with context.begin_transaction():
        context.run_migrations()


def run_migrations_online() -> None:
    settings = config.get_section(config.config_ini_section) or {}
    settings["sqlalchemy.url"] = database_url()
    engine = engine_from_config(
        settings,
        prefix="sqlalchemy.",
        poolclass=pool.NullPool,
    )
    with engine.connect() as connection:
        context.configure(connection=connection, target_metadata=target_metadata)
        with context.begin_transaction():
            context.run_migrations()


if context.is_offline_mode():
    run_migrations_offline()
else:
    run_migrations_online()
