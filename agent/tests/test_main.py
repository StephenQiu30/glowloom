from uuid import UUID

from fastapi.testclient import TestClient

from main import app, canonical_request_hash


def test_private_agent_requires_backend_token() -> None:
    client = TestClient(app)
    response = client.get("/internal/agent-runs/00000000-0000-0000-0000-000000000000")
    assert response.status_code == 403


def test_private_agent_run_is_idempotent() -> None:
    client = TestClient(app)
    payload = {
        "skill": "script_analysis",
        "stage": "manifest",
        "request_hash": canonical_request_hash("snapshot-1"),
        "snapshot_ref": "snapshot-1",
    }
    headers = {"X-Lanverse-Agent-Token": "lanverse-agent-local"}
    first = client.post("/internal/agent-runs", json=payload, headers=headers)
    second = client.post("/internal/agent-runs", json=payload, headers=headers)
    assert first.status_code == 200
    assert second.status_code == 200
    assert first.json()["run_id"] == second.json()["run_id"]
    assert UUID(first.json()["run_id"])
