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


def test_every_runtime_entrypoint_fails_closed_on_outdated_database_revision() -> None:
    for relative_path in (
        "backend/app/server.py",
        "backend/app/scheduler.py",
        "backend/app/io_worker.py",
        "backend/app/media_worker.py",
    ):
        source = (ROOT / relative_path).read_text()
        assert "await assert_database_at_head()" in source

    initializer = (ROOT / "backend/app/initialize_database.py").read_text()
    assert "await upgrade_database(engine)" in initializer
    assert "assert_database_at_head" not in initializer


def test_unified_server_configures_process_telemetry_before_child_roles() -> None:
    server = (ROOT / "backend/app/server.py").read_text()

    supervisor_position = server.index("await supervise_services(")
    assert server.index("configure_logging(") < supervisor_position
    assert server.index("configure_telemetry(") < supervisor_position
    assert 'service="lanverse-server"' in server
    assert 'service_name="lanverse-server"' in server
