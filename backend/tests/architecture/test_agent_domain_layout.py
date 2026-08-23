from pathlib import Path

MODULES_ROOT = Path(__file__).resolve().parents[2] / "app" / "modules"


def test_agent_harness_packages_are_owned_by_business_domains() -> None:
    assert not (MODULES_ROOT / "agents").exists()

    agent_packages = tuple(
        path for path in MODULES_ROOT.glob("*/agents") if path.is_dir()
    )
    assert MODULES_ROOT / "storyboards" / "agents" in agent_packages
    for package in agent_packages:
        implementation_files = {
            path.name for path in package.glob("*.py") if path.name != "__init__.py"
        }
        assert implementation_files, f"Agent package must not be an empty placeholder: {package}"
