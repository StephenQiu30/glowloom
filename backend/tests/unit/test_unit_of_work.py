from __future__ import annotations

import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
BACKEND = ROOT / "backend"
sys.path.insert(0, str(BACKEND / "src"))


class FakeSession:
    def __init__(self) -> None:
        self.commits = 0
        self.rollbacks = 0
        self.closes = 0

    def commit(self) -> None:
        self.commits += 1

    def rollback(self) -> None:
        self.rollbacks += 1

    def close(self) -> None:
        self.closes += 1


class SqlAlchemyUnitOfWorkTests(unittest.TestCase):
    def test_explicit_commit_finishes_the_transaction(self) -> None:
        from thief.infrastructure.unit_of_work import SqlAlchemyUnitOfWork

        session = FakeSession()
        with SqlAlchemyUnitOfWork(lambda: session) as unit_of_work:
            unit_of_work.commit()

        self.assertEqual((session.commits, session.rollbacks, session.closes), (1, 0, 1))

    def test_uncommitted_context_rolls_back(self) -> None:
        from thief.infrastructure.unit_of_work import SqlAlchemyUnitOfWork

        session = FakeSession()
        with SqlAlchemyUnitOfWork(lambda: session):
            pass

        self.assertEqual((session.commits, session.rollbacks, session.closes), (0, 1, 1))

    def test_exception_rolls_back_and_propagates(self) -> None:
        from thief.infrastructure.unit_of_work import SqlAlchemyUnitOfWork

        session = FakeSession()
        with self.assertRaisesRegex(RuntimeError, "boom"):
            with SqlAlchemyUnitOfWork(lambda: session):
                raise RuntimeError("boom")

        self.assertEqual((session.commits, session.rollbacks, session.closes), (0, 1, 1))


if __name__ == "__main__":
    unittest.main()
