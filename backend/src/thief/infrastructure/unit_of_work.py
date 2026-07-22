from __future__ import annotations

from collections.abc import Callable
from types import TracebackType
from typing import Self

from sqlalchemy.orm import Session


class SqlAlchemyUnitOfWork:
    def __init__(self, session_factory: Callable[[], Session]) -> None:
        self._session_factory = session_factory
        self._session: Session | None = None
        self._finished = False

    def __enter__(self) -> Self:
        self._session = self._session_factory()
        self._finished = False
        return self

    def __exit__(
        self,
        exception_type: type[BaseException] | None,
        exception: BaseException | None,
        traceback: TracebackType | None,
    ) -> None:
        try:
            if not self._finished:
                self.rollback()
        finally:
            self._active_session().close()
            self._session = None

    def commit(self) -> None:
        self._active_session().commit()
        self._finished = True

    def rollback(self) -> None:
        self._active_session().rollback()
        self._finished = True

    def _active_session(self) -> Session:
        if self._session is None:
            raise RuntimeError("UnitOfWork is not active")
        return self._session
