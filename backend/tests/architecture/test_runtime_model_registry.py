from pathlib import Path

from app.core.database import Base
from app.model_registry import register_implemented_models

ROOT = Path(__file__).resolve().parents[3]


def test_runtime_registry_resolves_every_implemented_foreign_key() -> None:
    register_implemented_models()

    for table in Base.metadata.tables.values():
        for foreign_key in table.foreign_keys:
            assert foreign_key.column.table.name in Base.metadata.tables


def test_background_entrypoints_register_models_before_database_work() -> None:
    for relative_path in (
        "backend/app/initialize_database.py",
        "backend/app/scheduler.py",
        "backend/app/io_worker.py",
        "backend/app/media_worker.py",
    ):
        source = (ROOT / relative_path).read_text()
        assert "register_implemented_models()" in source

    server = (ROOT / "backend/app/server.py").read_text()
    assert "partial(run_media_worker, settings)" in server
