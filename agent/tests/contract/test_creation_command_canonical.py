import json
from pathlib import Path

from app.candidate_runtime.canonical import production_canonical_hash


def test_creation_command_matches_go_canonical_fixture() -> None:
    root = Path(__file__).resolve().parents[3]
    fixture = json.loads(
        (root / "backend/tests/production/creation/testdata/command.json").read_text()
    )
    assert production_canonical_hash(fixture["command"]) == fixture["payload_hash"]
