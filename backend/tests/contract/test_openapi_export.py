from __future__ import annotations

import json
import urllib.request
from pathlib import Path

from tests.contract.operation_contract import EXPECTED_OPERATIONS

ROOT = Path(__file__).resolve().parents[3]
BACKEND = ROOT / "backend"


def fetch(url: str) -> bytes:
    with urllib.request.urlopen(url, timeout=2) as response:
        assert response.status == 200
        return response.read()


def test_openapi_http_response_is_deterministic(live_openapi_url: str) -> None:
    first = fetch(live_openapi_url)
    second = fetch(live_openapi_url)
    assert first == second
    document = json.loads(first)
    assert document["openapi"] == "3.1.0"
    assert document["info"] == {"title": "Lanverse API", "version": "0.1.0"}
    operations = {
        (operation["operationId"], method, path)
        for path, path_item in document["paths"].items()
        for method, operation in path_item.items()
        if isinstance(operation, dict) and "operationId" in operation
    }
    assert operations == EXPECTED_OPERATIONS


def test_repository_has_no_static_openapi_intermediate() -> None:
    assert not (BACKEND / "openapi").exists()
    assert not (BACKEND / "scripts" / "export_openapi.py").exists()
