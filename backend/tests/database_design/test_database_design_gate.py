# ruff: noqa: E501
from __future__ import annotations

import hashlib
import re
import subprocess
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
DOCS = ROOT / "docs"
SQL = ROOT / "sql"

# One line per table keeps physical column-order drift reviewable as a single diff.
TABLE_COLUMNS = {
    "projects": "id title status aspect_ratio width height fps timebase created_at updated_at",
    "episodes": "id project_id target_min_ticks target_max_ticks created_at updated_at",
    "source_revisions": "id episode_id version parent_id content normalization_version codepoint_count sha256 rights_basis rights_declared_at status resource_version created_at updated_at confirmed_at",
    "idempotency_records": "id owner_module operation_scope idempotency_key request_hash state response_status response_ref_json request_id created_at updated_at completed_at",
    "submission_snapshots": "id episode_id type capability input_refs_json prompt parameters_json parameters_hash model_profile_id provider_id model_id route_version schema_version content_hash created_at",
    "production_tasks": "id episode_id snapshot_id type scope_json idempotency_scope idempotency_key status progress_json retry_of_task_id error_code error_json next_action resource_version created_at updated_at finished_at",
    "production_attempts": "id task_id snapshot_id attempt_no parent_attempt_id status provider_id provider_request_key provider_request_id usage_json safety_json execution_metadata_json error_code error_summary created_at submitted_at started_at finished_at",
    "script_versions": "id episode_id version parent_id source_revision_id schema_version content_json content_hash origin_task_id model_profile_id provider_id model_id prompt_version status resource_version created_at updated_at confirmed_at",
    "creative_asset_versions": "id asset_id episode_id version parent_id source_script_version_id origin_task_id asset_type name description content_hash status resource_version created_at updated_at confirmed_at",
    "shot_spec_versions": "id episode_id version parent_id script_version_id asset_version_refs_json shots_json shot_count total_duration_ticks content_hash origin_task_id status resource_version created_at updated_at confirmed_at",
    "task_events": "event_id task_id task_resource_version event_type occurred_at correlation_id data_json",
    "task_jobs": "id task_id payload_json state lease_owner lease_until attempts next_attempt_at last_error_code created_at updated_at completed_at",
    "task_outputs": "id task_id output_type output_id ordinal created_at",
    "media_objects": "id episode_id media_kind source_kind created_at",
    "media_versions": "id media_object_id version parent_id origin_attempt_id output_slot bucket object_key mime_type byte_size sha256 status width height duration_ticks timebase probe_summary_json created_at finalized_at",
    "generation_candidates": "id episode_id task_id attempt_id output_slot usage_type usage_id input_version_id input_hash media_version_id status blocked_reason created_at finalized_at",
    "adoptions": "id episode_id usage_type usage_id input_version_id input_hash version candidate_id supersedes_id status created_at superseded_at",
    "subtitle_versions": "id episode_id version parent_id script_version_id shot_spec_version_id input_refs_json language cues_json cue_count content_hash status resource_version created_at updated_at confirmed_at",
    "render_snapshots": "id episode_id submission_scope idempotency_key request_hash initial_task_id shot_spec_version_id subtitle_version_id input_refs_json segments_json timebase width height fps audio_rate audio_channels normalization_json recipe_hash content_hash created_at",
    "delivery_versions": "id episode_id version render_task_id final_attempt_id retry_of_delivery_id render_snapshot_id mp4_media_version_id srt_media_version_id manifest_media_version_id artifact_summary_json ffmpeg_version ffprobe_summary_json status error_code created_at updated_at finished_at",
}


def frontmatter(path: Path, key: str) -> str:
    match = re.search(rf"^{re.escape(key)}:\s*(.+)$", path.read_text(), re.MULTILINE)
    assert match, f"missing {key}: {path}"
    return match.group(1).strip()


def formal_documents(folder: str) -> list[Path]:
    return sorted(path for path in (DOCS / folder).glob("*.md") if path.name != "README.md")


def input_token(path: Path) -> str:
    commit = subprocess.run(
        ["git", "log", "-1", "--format=%H", "--", str(path.relative_to(ROOT))],
        cwd=ROOT,
        check=True,
        capture_output=True,
        text=True,
    ).stdout.strip()
    return f"{frontmatter(path, 'doc_no')}/{frontmatter(path, 'version')}@{commit}"


def gate_inputs(scope: str) -> list[str]:
    latest = sorted((DOCS / "gates").glob(f"GATE-{scope}-*.md"))[-1]
    assert frontmatter(latest, "result") == "passed"
    raw = frontmatter(latest, "input_versions")
    return [item.strip() for item in raw.removeprefix("[").removesuffix("]").split(",")]


def test_gate_snapshots_match_current_accepted_inputs() -> None:
    requirements = formal_documents("requirement")
    designs = formal_documents("design")
    products = formal_documents("prd")
    plans = formal_documents("plans")
    all_documents = requirements + designs + products + plans
    assert all(frontmatter(path, "status") == "accepted" for path in all_documents)

    expected = {
        "requirement_readiness": requirements,
        "design_entry": designs,
        "database_design_ready": [designs[5], plans[2]],
        "implementation_start": all_documents,
    }
    for scope, paths in expected.items():
        assert gate_inputs(scope) == [input_token(path) for path in paths]


def table_columns(text: str) -> list[str]:
    body = text.split("CREATE TABLE", maxsplit=1)[1].split(");", maxsplit=1)[0]
    return re.findall(r"^    ([a-z][a-z0-9_]*)\s+", body, re.MULTILINE)


def test_sql_bundle_matches_the_reviewed_exact_set() -> None:
    files = sorted(SQL.glob("[0-9][0-9]_*.sql"))
    assert [path.stem.split("_", maxsplit=1)[1] for path in files] == list(TABLE_COLUMNS)

    digest = hashlib.sha256()
    texts: list[str] = []
    for path, (table, columns) in zip(files, TABLE_COLUMNS.items(), strict=True):
        text = path.read_text()
        texts.append(text)
        assert len(re.findall(r"CREATE TABLE\s+public\.", text, re.IGNORECASE)) == 1
        assert f"CREATE TABLE public.{table}" in text
        assert table_columns(text) == columns.split()
        assert not re.search(
            r"^\s*(ALTER\s+TABLE|DROP\b|INSERT\s+INTO|UPDATE\s+public\.|DELETE\s+FROM|TRUNCATE\b)",
            text,
            re.IGNORECASE | re.MULTILINE,
        )
        assert not re.search(r"CREATE\s+(DATABASE|SCHEMA)", text, re.I)
        digest.update(path.name.encode())
        digest.update(b"\0")
        digest.update(path.read_bytes())
        digest.update(b"\0")

    bundle = "\n".join(texts)
    assert len(re.findall(r"\bREFERENCES\s+public\.", bundle, re.I)) == 51
    assert len(re.findall(r"\bCREATE\s+(?:UNIQUE\s+)?INDEX\b", bundle, re.I)) == 57
    jsonb_columns = sum(
        " jsonb" in line.lower()
        for text in texts
        for line in text.splitlines()
        if re.match(r"^    [a-z][a-z0-9_]*\s+", line)
    )
    assert jsonb_columns == 22
    assert digest.hexdigest() == "5322bbc0189358f71300b37bd99be9f5e396e7682f2fb4bc4ac3f1e90cebcf3f"
