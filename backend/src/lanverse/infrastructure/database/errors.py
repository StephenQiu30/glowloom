from __future__ import annotations

import asyncpg  # type: ignore[import-untyped]

from lanverse.shared_kernel.errors import ApplicationError


class DatabaseError(ApplicationError):
    code = "database_error"

    def __init__(self) -> None:
        super().__init__(self.code)


class DatabaseConflictError(DatabaseError):
    code = "database_conflict"


class DatabaseUnavailableError(DatabaseError):
    code = "database_unavailable"


class DatabaseOperationError(DatabaseError):
    code = "database_operation_failed"


def translate_database_error(error: BaseException) -> DatabaseError:
    if isinstance(
        error,
        (
            asyncpg.UniqueViolationError,
            asyncpg.ForeignKeyViolationError,
            asyncpg.CheckViolationError,
        ),
    ):
        return DatabaseConflictError()
    if isinstance(
        error,
        (
            asyncpg.CannotConnectNowError,
            asyncpg.ConnectionDoesNotExistError,
            asyncpg.PostgresConnectionError,
        ),
    ):
        return DatabaseUnavailableError()
    return DatabaseOperationError()
