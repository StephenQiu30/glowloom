from app.core.database import Base
from app.model_registry import register_implemented_models


def test_runtime_registry_resolves_every_implemented_foreign_key() -> None:
    register_implemented_models()

    for table in Base.metadata.tables.values():
        for foreign_key in table.foreign_keys:
            assert foreign_key.column.table.name in Base.metadata.tables
