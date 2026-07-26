from __future__ import annotations

import json
import urllib.request
from pathlib import Path

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
        operation["operationId"]
        for path in document["paths"].values()
        for operation in path.values()
        if isinstance(operation, dict) and "operationId" in operation
    }
    assert operations == {
        "createProject",
        "listProjects",
        "getProject",
        "getEpisode",
        "createSourceRevision",
        "confirmSource",
        "listSourceRevisions",
        "getSourceRevision",
        "listTasks",
        "getTask",
        "cancelTask",
        "retryTask",
        "generateScript",
        "getCurrentScript",
        "saveScript",
        "confirmScript",
        "deriveScriptDraft",
        "listScriptVersions",
        "getScriptVersion",
        "generateStoryboard",
        "getStoryboard",
        "listCreativeAssets",
        "getCreativeAssetVersion",
        "saveCreativeAsset",
        "saveStoryboard",
        "confirmStoryboard",
        "deriveStoryboardDraft",
        "listStoryboardVersions",
        "getStoryboardVersion",
        "generateMedia",
        "listCandidates",
        "adoptCandidate",
        "authorizeCandidatePreview",
        "listDeliveries",
        "getDelivery",
        "authorizeDownload",
        "createSubtitles",
        "getSubtitles",
        "saveSubtitles",
        "listSubtitleVersions",
        "getSubtitleVersion",
        "deriveSubtitleDraft",
        "confirmSubtitles",
    }


def test_repository_has_no_static_openapi_intermediate() -> None:
    assert not (BACKEND / "openapi").exists()
    assert not (BACKEND / "scripts" / "export_openapi.py").exists()
