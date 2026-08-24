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
_STORYBOARD_HEADERS: tuple[str, ...] = (
    "shot_position",
    "timecode_in",
    "timecode_out",
    "duration_ms",
    "shot_id",
    "shot_spec_version_id",
    "shot_title",
    "scene_id",
    "narrative_purpose",
    "narrative_roles",
    "shot_size",
    "camera_angle",
    "camera_movement",
    "composition",
    "environment",
    "mood_lighting",
    "subject_placements",
    "action_beats",
    "dialogue_or_narration",
    "audio_intent",
    "continuity_note",
    "first_frame",
    "keyframe_notes",
    "last_frame",
    "prompt",
    "asset_names",
    "narrative_text",
    "shot_spec_json",
)


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


def _timecode(milliseconds: int) -> str:
    hours, remainder = divmod(milliseconds, 3_600_000)
    minutes, remainder = divmod(remainder, 60_000)
    seconds, millis = divmod(remainder, 1_000)
    return f"{hours:02d}:{minutes:02d}:{seconds:02d}.{millis:03d}"


def _shot_rows(snapshot: ExportSnapshot) -> list[dict[str, str | int]]:
    asset_names = {value.asset_version_id: value.name for value in snapshot.assets}
    narrative_text = {value.unit_version_id: value.exact_text for value in snapshot.units}
    narrative_kind = {value.unit_version_id: value.kind for value in snapshot.units}
    rows: list[dict[str, str | int]] = []
    elapsed_ms = 0
    for shot in snapshot.shots:
        spec = shot.spec
        audio = spec.audio_intent
        generation = spec.generation_intent
        source_texts = [
            narrative_text[value.unit_version_id] for value in shot.narrative_references
        ]
        dialogue_texts = [
            narrative_text[value.unit_version_id]
            for value in shot.narrative_references
            if narrative_kind[value.unit_version_id] in {"dialogue", "narration"}
        ]
        performance_notes = [
            value.performance_note
            for value in spec.dialogue_or_narration
            if value.performance_note is not None
        ]
        audio_parts: list[str] = []
        if audio is not None:
            if audio.ambient:
                audio_parts.append(f"ambient: {audio.ambient}")
            if audio.sound_effects:
                audio_parts.append(f"sfx: {'；'.join(audio.sound_effects)}")
        timecode_in = _timecode(elapsed_ms)
        elapsed_ms += spec.duration_ms
        rows.append(
            {
                "shot_position": shot.position,
                "timecode_in": timecode_in,
                "timecode_out": _timecode(elapsed_ms),
                "duration_ms": spec.duration_ms,
                "shot_id": str(shot.shot_id),
                "shot_spec_version_id": str(shot.shot_spec_version_id),
                "shot_title": shot.title,
                "scene_id": str(spec.script_reference.scene_id),
                "narrative_purpose": spec.narrative.purpose,
                "narrative_roles": " | ".join(
                    f"{value.role}/{value.channel}/{value.contribution}"
                    for value in shot.narrative_references
                ),
                "shot_size": spec.visual.shot_size,
                "camera_angle": spec.visual.camera_angle,
                "camera_movement": spec.visual.camera_movement,
                "composition": spec.visual.composition,
                "environment": spec.visual.environment,
                "mood_lighting": spec.visual.mood_lighting,
                "subject_placements": " | ".join(
                    f"{value.subject_key}: {value.placement}"
                    for value in spec.visual.subject_placements
                ),
                "action_beats": " | ".join(
                    f"{value.order}. {value.description}" for value in spec.action_beats
                ),
                "dialogue_or_narration": " | ".join([*dialogue_texts, *performance_notes]),
                "audio_intent": " | ".join(audio_parts),
                "continuity_note": spec.narrative.continuity_note or "",
                "first_frame": generation.first_frame or "",
                "keyframe_notes": generation.keyframe_notes or "",
                "last_frame": generation.last_frame or "",
                "prompt": shot.prompt,
                "asset_names": " | ".join(
                    asset_names[value.asset_version_id] for value in shot.asset_references
                ),
                "narrative_text": " | ".join(source_texts),
                "shot_spec_json": json.dumps(
                    spec.model_dump(mode="json"),
                    ensure_ascii=False,
                    sort_keys=True,
                    separators=(",", ":"),
                ),
            }
        )
    return rows


def _storyboard_csv(snapshot: ExportSnapshot) -> PackageMember:
    output = io.StringIO(newline="")
    writer = csv.DictWriter(output, fieldnames=_STORYBOARD_HEADERS, lineterminator="\r\n")
    writer.writeheader()
    writer.writerows(_shot_rows(snapshot))
    return PackageMember(
        path="storyboard.csv",
        media_type="text/csv; charset=utf-8",
        content=b"\xef\xbb\xbf" + output.getvalue().encode("utf-8"),
    )


def _storyboard_html(snapshot: ExportSnapshot) -> PackageMember:
    head = (
        '<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">'
        "<title>Storyboard Export</title><style>"
        "body{font-family:system-ui,sans-serif;margin:24px}"
        "table{border-collapse:collapse;width:100%;font-size:13px}"
        "th,td{border:1px solid #ccc;padding:8px;text-align:left;vertical-align:top}"
        "th{background:#f3f4f6}pre{white-space:pre-wrap;margin:0}"
        "</style></head><body><h1>Storyboard Export</h1><table><thead><tr>"
        "<th># / Time</th><th>Source / Purpose</th><th>Camera / Composition</th>"
        "<th>Blocking / Action</th><th>Dialogue / Sound</th>"
        "<th>Frames / Continuity</th><th>Assets / Prompt</th>"
        "</tr></thead><tbody>"
    )
    body = "".join(
        "<tr>"
        f"<td><strong>{html.escape(str(row['shot_position']))}</strong><br>"
        f"{html.escape(str(row['timecode_in']))} → {html.escape(str(row['timecode_out']))}<br>"
        f"{html.escape(str(row['duration_ms']))} ms</td>"
        f"<td><strong>{html.escape(str(row['shot_title']))}</strong><br>"
        f"Scene: {html.escape(str(row['scene_id']))}<br>"
        f"{html.escape(str(row['narrative_purpose']))}<br>"
        f"<pre>{html.escape(str(row['narrative_text']))}</pre>"
        f"{html.escape(str(row['narrative_roles']))}</td>"
        f"<td>{html.escape(str(row['shot_size']))} / "
        f"{html.escape(str(row['camera_angle']))} / "
        f"{html.escape(str(row['camera_movement']))}<br>"
        f"<pre>{html.escape(str(row['composition']))}</pre>"
        f"{html.escape(str(row['environment']))}<br>"
        f"{html.escape(str(row['mood_lighting']))}</td>"
        f"<td><pre>{html.escape(str(row['subject_placements']))}</pre>"
        f"<pre>{html.escape(str(row['action_beats']))}</pre></td>"
        f"<td><pre>{html.escape(str(row['dialogue_or_narration']))}</pre>"
        f"<pre>{html.escape(str(row['audio_intent']))}</pre></td>"
        f"<td>IN: <pre>{html.escape(str(row['first_frame']))}</pre>"
        f"KEY: <pre>{html.escape(str(row['keyframe_notes']))}</pre>"
        f"OUT: <pre>{html.escape(str(row['last_frame']))}</pre>"
        f"CONT: <pre>{html.escape(str(row['continuity_note']))}</pre></td>"
        f"<td>{html.escape(str(row['asset_names']))}<br>"
        f"<pre>{html.escape(str(row['prompt']))}</pre></td>"
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
                "asset_version_ids": [str(value.asset_version_id) for value in snapshot.assets],
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
