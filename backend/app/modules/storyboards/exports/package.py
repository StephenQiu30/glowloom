import csv
import hashlib
import html
import io
import json
import zipfile

from app.modules.storyboards.exports.contracts import (
    ExportFile,
    ExportSnapshot,
    PackageMember,
    PackageResult,
)
from app.modules.storyboards.hashing import canonical_payload_hash

_ZIP_TIMESTAMP = (1980, 1, 1, 0, 0, 0)


def _json_bytes(value: object) -> bytes:
    return json.dumps(
        value,
        ensure_ascii=False,
        sort_keys=True,
        separators=(",", ":"),
    ).encode("utf-8")


def _file(member: PackageMember) -> ExportFile:
    return ExportFile(
        path=member.path,
        media_type=member.media_type,
        sha256=hashlib.sha256(member.content).hexdigest(),
        size_bytes=len(member.content),
    )


def _storyboard_json(snapshot: ExportSnapshot) -> PackageMember:
    return PackageMember(
        path="storyboard.json",
        media_type="application/json",
        content=_json_bytes(
            {
                "schema_label": "lanverse.storyboard.export.storyboard.1",
                "snapshot": snapshot.model_dump(mode="json"),
            }
        ),
    )


def _shot_rows(snapshot: ExportSnapshot) -> list[list[str | int]]:
    asset_names = {value.asset_version_id: value.name for value in snapshot.assets}
    narrative_text = {
        value.unit_version_id: value.exact_text for value in snapshot.units
    }
    rows: list[list[str | int]] = []
    for shot in snapshot.shots:
        rows.append(
            [
                shot.position,
                str(shot.shot_id),
                str(shot.shot_spec_version_id),
                shot.title,
                shot.prompt,
                " | ".join(
                    asset_names[value.asset_version_id]
                    for value in shot.asset_references
                ),
                " | ".join(
                    narrative_text[value.unit_version_id]
                    for value in shot.narrative_references
                ),
                json.dumps(
                    shot.spec.model_dump(mode="json"),
                    ensure_ascii=False,
                    sort_keys=True,
                    separators=(",", ":"),
                ),
            ]
        )
    return rows


def _storyboard_csv(snapshot: ExportSnapshot) -> PackageMember:
    output = io.StringIO(newline="")
    writer = csv.writer(output, lineterminator="\r\n")
    writer.writerow(
        (
            "shot_position",
            "shot_id",
            "shot_spec_version_id",
            "shot_title",
            "prompt",
            "asset_names",
            "narrative_text",
            "shot_spec_json",
        )
    )
    writer.writerows(_shot_rows(snapshot))
    return PackageMember(
        path="storyboard.csv",
        media_type="text/csv; charset=utf-8",
        content=b"\xef\xbb\xbf" + output.getvalue().encode("utf-8"),
    )


def _storyboard_html(snapshot: ExportSnapshot) -> PackageMember:
    head = (
        "<!doctype html><html lang=\"zh-CN\"><head><meta charset=\"utf-8\">"
        "<title>Storyboard Export</title><style>"
        "body{font-family:system-ui,sans-serif;margin:24px}"
        "table{border-collapse:collapse;width:100%}"
        "th,td{border:1px solid #ccc;padding:8px;text-align:left;vertical-align:top}"
        "th{background:#f3f4f6}pre{white-space:pre-wrap;margin:0}"
        "</style></head><body><h1>Storyboard Export</h1><table><thead><tr>"
        "<th>#</th><th>Shot</th><th>Prompt</th><th>Assets</th>"
        "<th>Narrative</th></tr></thead><tbody>"
    )
    body = "".join(
        "<tr>"
        f"<td>{html.escape(str(row[0]))}</td>"
        f"<td>{html.escape(str(row[3]))}</td>"
        f"<td><pre>{html.escape(str(row[4]))}</pre></td>"
        f"<td>{html.escape(str(row[5]))}</td>"
        f"<td><pre>{html.escape(str(row[6]))}</pre></td>"
        "</tr>"
        for row in _shot_rows(snapshot)
    )
    return PackageMember(
        path="storyboard.html",
        media_type="text/html; charset=utf-8",
        content=(head + body + "</tbody></table></body></html>").encode("utf-8"),
    )


def _manifest(
    snapshot: ExportSnapshot,
    input_hash: str,
    files: tuple[ExportFile, ...],
) -> PackageMember:
    return PackageMember(
        path="manifest.json",
        media_type="application/json",
        content=_json_bytes(
            {
                "schema_label": "lanverse.storyboard.export.manifest.1",
                "input_hash": input_hash,
                "episode_id": str(snapshot.episode_id),
                "script_version_id": str(snapshot.script_version_id),
                "narrative_structure_id": str(snapshot.narrative_structure_id),
                "narrative_unit_version_ids": [
                    str(value.unit_version_id) for value in snapshot.units
                ],
                "shot_spec_version_ids": [
                    str(value.shot_spec_version_id) for value in snapshot.shots
                ],
                "asset_version_ids": [
                    str(value.asset_version_id) for value in snapshot.assets
                ],
                "coverage_basis_hash": snapshot.coverage_basis_hash,
                "coverage_evaluation_hash": snapshot.coverage_evaluation_hash,
                "readiness_evaluation_hash": snapshot.readiness_evaluation_hash,
                "files": [value.model_dump(mode="json") for value in files],
            }
        ),
    )


def _zip_bytes(members: tuple[PackageMember, ...]) -> bytes:
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_STORED) as archive:
        for member in sorted(members, key=lambda value: value.path):
            info = zipfile.ZipInfo(member.path, date_time=_ZIP_TIMESTAMP)
            info.compress_type = zipfile.ZIP_STORED
            info.create_system = 3
            info.external_attr = 0o100644 << 16
            archive.writestr(info, member.content)
    return output.getvalue()


def build_storyboard_package(
    snapshot: ExportSnapshot,
    input_hash: str,
) -> PackageResult:
    if canonical_payload_hash(snapshot.model_dump(mode="json")) != input_hash:
        raise ValueError("storyboard export input hash does not match its snapshot")
    representations = (
        _storyboard_csv(snapshot),
        _storyboard_html(snapshot),
        _storyboard_json(snapshot),
    )
    manifest = _manifest(snapshot, input_hash, tuple(_file(item) for item in representations))
    members = (manifest, *representations)
    files = tuple(_file(item) for item in sorted(members, key=lambda value: value.path))
    content = _zip_bytes(members)
    return PackageResult(
        content=content,
        sha256=hashlib.sha256(content).hexdigest(),
        size_bytes=len(content),
        files=files,
    )
