from __future__ import annotations

from collections.abc import Callable
from typing import Self

from sqlalchemy.orm import Session

from thief_adapters.identity.repository import SqlAlchemyIdentityRepository
from thief_adapters.infrastructure import SqlAlchemyUnitOfWork


class SqlAlchemyIdentityUnitOfWork(SqlAlchemyUnitOfWork):
    identities: SqlAlchemyIdentityRepository

    def __init__(self, session_factory: Callable[[], Session]) -> None:
        super().__init__(session_factory)

    def __enter__(self) -> Self:
        super().__enter__()
        self.identities = SqlAlchemyIdentityRepository(self._active_session())
        return self
