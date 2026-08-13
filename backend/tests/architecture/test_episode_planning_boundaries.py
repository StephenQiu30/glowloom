from pathlib import Path


ROOT = Path(__file__).resolve().parents[2]
PLANNING = ROOT / "app/modules/scripts/planning"


def test_episode_planning_exists_and_does_not_import_project_orm() -> None:
    service = PLANNING / "service.py"

    assert service.is_file()
    source = service.read_text(encoding="utf-8")
    assert "app.modules.projects.models" not in source
    assert "app.modules.projects.repository" not in source
    assert "prj_episodes" not in source
    assert "materialize_episode_batch" in source


def test_ai_episode_planning_messages_never_contain_script_body() -> None:
    service = PLANNING / "service.py"

    assert service.is_file()
    source = service.read_text(encoding="utf-8")
    assert 'event_type="episode_planning.requested"' not in source
    assert "create_episode_planning_task" in source
