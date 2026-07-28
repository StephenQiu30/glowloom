from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]


def test_production_and_test_code_are_separate() -> None:
    assert (ROOT / "backend/app").is_dir()
    assert (ROOT / "backend/tests").is_dir()
    assert not list((ROOT / "backend/app").rglob("test_*.py"))
    assert not list((ROOT / "frontend/src").rglob("*.test.*"))
    assert not list((ROOT / "frontend/src").rglob("*.spec.*"))


def test_forbidden_overdesigned_directories_do_not_exist() -> None:
    forbidden = [
        ROOT / "backend/src/lanverse",
        ROOT / "backend/app/app",
        ROOT / "shared",
        ROOT / "common",
        ROOT / "packages",
    ]
    assert not [path for path in forbidden if path.exists()]
