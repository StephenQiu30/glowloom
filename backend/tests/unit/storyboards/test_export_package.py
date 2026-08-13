import csv
import hashlib
import io
import json
import zipfile
from uuid import UUID

from app.modules.storyboards.exports.contracts import (
    ExportAsset,
    ExportAssetReference,
    ExportNarrativeReference,
    ExportShot,
    ExportSnapshot,
    ExportUnit,
)
from app.modules.storyboards.exports.package import build_storyboard_package


def _id(value: int) -> UUID:
    return UUID(int=value)


def _snapshot() -> ExportSnapshot:
    digest = hashlib.sha256(b"fixed").hexdigest()
    return ExportSnapshot(
        workspace_id=_id(1),
        project_id=_id(2),
        episode_id=_id(3),
        script_version_id=_id(4),
        script_content_hash=digest,
        narrative_structure_id=_id(5),
        narrative_structure_revision=2,
        narrative_dependency_hash=digest,
        coverage_basis_hash=digest,
        coverage_evaluation_hash=digest,
        readiness_evaluation_hash=digest,
        units=(
            ExportUnit(
                narrative_unit_id=_id(6),
                unit_version_id=_id(7),
                position=1,
                kind="action",
                exact_text="沈岚 <回头> & 决断",
                text_hash=digest,
                required_for_coverage=True,
                coverage_status="covered",
            ),
        ),
        assets=(
            ExportAsset(
                asset_id=_id(8),
                asset_state_id=_id(9),
                asset_version_id=_id(10),
                kind="character",
                name='沈岚, "女帝"',
                state_label="常服",
                state_revision=1,
                readiness_hash=digest,
            ),
        ),
        shots=(
            ExportShot(
                shot_id=_id(11),
                shot_spec_version_id=_id(12),
                position=1,
                title="回头 <特写>",
                spec_version_no=3,
                spec_content_hash=digest,
                spec_input_hash=digest,
                spec={
                    "visual": {"environment": "河岸"},
                    "action_beats": [{"description": "沈岚回头"}],
                },
                prompt="河岸，沈岚回头",
                readiness_hash=digest,
                asset_references=(
                    ExportAssetReference(
                        slot_key="character.lead",
                        role="subject",
                        asset_id=_id(8),
                        asset_state_id=_id(9),
                        asset_version_id=_id(10),
                        binding_source="manual",
                        subject_key="lead",
                    ),
                ),
                narrative_references=(
                    ExportNarrativeReference(
                        reference_id=_id(13),
                        narrative_unit_id=_id(6),
                        unit_version_id=_id(7),
                        channel="visual",
                        role="primary",
                        coverage_mode="full",
                        segment_start=None,
                        segment_end=None,
                        contribution="required",
                        origin="human",
                    ),
                ),
            ),
        ),
    )


def test_storyboard_package_is_deterministic_and_self_describing() -> None:
    snapshot = _snapshot()
    input_hash = hashlib.sha256(
        json.dumps(
            snapshot.model_dump(mode="json"),
            ensure_ascii=False,
            sort_keys=True,
            separators=(",", ":"),
        ).encode()
    ).hexdigest()

    first = build_storyboard_package(snapshot, input_hash)
    second = build_storyboard_package(snapshot, input_hash)

    assert first == second
    assert first.sha256 == hashlib.sha256(first.content).hexdigest()
    assert first.size_bytes == len(first.content)
    assert [item.path for item in first.files] == [
        "manifest.json",
        "storyboard.csv",
        "storyboard.html",
        "storyboard.json",
    ]
    with zipfile.ZipFile(io.BytesIO(first.content)) as package:
        assert package.namelist() == [
            "manifest.json",
            "storyboard.csv",
            "storyboard.html",
            "storyboard.json",
        ]
        assert all(item.date_time == (1980, 1, 1, 0, 0, 0) for item in package.infolist())
        manifest = json.loads(package.read("manifest.json"))
        assert manifest["schema_label"] == "lanverse.storyboard.export.manifest.1"
        assert manifest["input_hash"] == input_hash
        assert {item["path"] for item in manifest["files"]} == {
            "storyboard.csv",
            "storyboard.html",
            "storyboard.json",
        }
        for item in manifest["files"]:
            content = package.read(item["path"])
            assert item["sha256"] == hashlib.sha256(content).hexdigest()
            assert item["size_bytes"] == len(content)


def test_storyboard_package_keeps_machine_and_human_formats_safe() -> None:
    result = build_storyboard_package(_snapshot(), "a" * 64)

    with zipfile.ZipFile(io.BytesIO(result.content)) as package:
        json_payload = json.loads(package.read("storyboard.json"))
        assert json_payload["schema_label"] == "lanverse.storyboard.export.storyboard.1"
        assert json_payload["snapshot"]["shots"][0]["title"] == "回头 <特写>"

        csv_content = package.read("storyboard.csv")
        assert csv_content.startswith(b"\xef\xbb\xbf")
        rows = list(csv.DictReader(io.StringIO(csv_content.decode("utf-8-sig"))))
        assert rows[0]["shot_title"] == "回头 <特写>"
        assert rows[0]["asset_names"] == '沈岚, "女帝"'
        assert rows[0]["narrative_text"] == "沈岚 <回头> & 决断"

        html = package.read("storyboard.html").decode()
        assert "回头 &lt;特写&gt;" in html
        assert "沈岚 &lt;回头&gt; &amp; 决断" in html
        assert "<script" not in html.lower()
