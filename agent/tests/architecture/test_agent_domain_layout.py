import ast
from pathlib import Path

MODULES_ROOT = Path(__file__).resolve().parents[2] / "app" / "modules"
APP_ROOT = MODULES_ROOT.parent


def _asyncio_wait_for_calls(source_path: Path) -> list[ast.Call]:
    tree = ast.parse(source_path.read_text(encoding="utf-8"))
    return [
        node
        for node in ast.walk(tree)
        if isinstance(node, ast.Call)
        and isinstance(node.func, ast.Attribute)
        and isinstance(node.func.value, ast.Name)
        and node.func.value.id == "asyncio"
        and node.func.attr == "wait_for"
    ]


def _assert_waits_for_named_event(
    calls: list[ast.Call],
    *,
    event_name: str,
    timeout_name: str,
) -> None:
    for call in calls:
        assert call.args
        awaited = call.args[0]
        assert isinstance(awaited, ast.Call)
        assert isinstance(awaited.func, ast.Attribute)
        assert isinstance(awaited.func.value, ast.Name)
        assert awaited.func.value.id == event_name
        assert awaited.func.attr == "wait"
        timeout = next(
            (keyword.value for keyword in call.keywords if keyword.arg == "timeout"),
            None,
        )
        assert isinstance(timeout, ast.Name)
        assert timeout.id == timeout_name


def test_agent_harness_packages_are_owned_by_business_domains() -> None:
    assert not (MODULES_ROOT / "agents").exists()

    agent_packages = tuple(path for path in MODULES_ROOT.glob("*/agents") if path.is_dir())
    assert MODULES_ROOT / "storyboards" / "agents" in agent_packages
    for package in agent_packages:
        implementation_files = {
            path.name for path in package.glob("*.py") if path.name != "__init__.py"
        }
        assert implementation_files, f"Agent package must not be an empty placeholder: {package}"


def test_ai_business_harnesses_do_not_impose_wall_clock_deadlines() -> None:
    business_roots = (
        MODULES_ROOT / "skills",
        MODULES_ROOT / "scripts" / "production_bibles",
        MODULES_ROOT / "storyboards" / "agents",
    )
    business_sources = tuple(
        source_path
        for business_root in business_roots
        for source_path in business_root.rglob("*.py")
    )
    assert business_sources
    forbidden_deadline_markers = (
        "timeout_seconds",
        "asyncio.wait_for",
        "asyncio.timeout(",
        "fail_after(",
        "move_on_after(",
        "skill_timeout",
        "codex_result_unknown",
    )
    for source_path in business_sources:
        source = source_path.read_text(encoding="utf-8")
        for marker in forbidden_deadline_markers:
            assert marker not in source

    codex_path = APP_ROOT / "integrations" / "codex_local.py"
    codex_source = codex_path.read_text(encoding="utf-8")
    assert "timeout_seconds" not in codex_source
    assert "model_reasoning_effort" not in codex_source
    assert "skill_timeout" not in codex_source
    assert "codex_result_unknown" not in codex_source
    codex_waits = _asyncio_wait_for_calls(codex_path)
    assert len(codex_waits) == 1
    assert "asyncio.wait_for(process.wait(), timeout=5)" in codex_source

    worker_path = APP_ROOT / "runtime" / "workers" / "io.py"
    worker_waits = _asyncio_wait_for_calls(worker_path)
    assert len(worker_waits) == 2
    _assert_waits_for_named_event(
        worker_waits,
        event_name="stop",
        timeout_name="next_attempt_seconds",
    )
