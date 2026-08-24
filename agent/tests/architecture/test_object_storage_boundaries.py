from pathlib import Path

from app.modules.media.storage import ObjectStoragePort

ROOT = Path(__file__).resolve().parents[2]
BUSINESS_MODULES = ROOT / "app" / "modules"


def test_object_storage_port_exposes_only_the_eight_accepted_operations() -> None:
    operations = {
        name
        for name, member in vars(ObjectStoragePort).items()
        if not name.startswith("_") and callable(member)
    }

    assert operations == {
        "ensure_bucket",
        "presign_upload",
        "presign_download",
        "stat",
        "put",
        "copy",
        "stream",
        "delete",
    }


def test_business_modules_do_not_import_sdk_or_read_storage_credentials() -> None:
    forbidden = (
        "from minio",
        "import minio",
        "minio_access_key",
        "minio_secret_key",
        "minio_endpoint",
    )
    violations: list[str] = []
    for path in BUSINESS_MODULES.rglob("*.py"):
        source = path.read_text(encoding="utf-8")
        if any(marker in source for marker in forbidden):
            violations.append(str(path.relative_to(ROOT)))

    assert violations == []
