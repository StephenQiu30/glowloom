# Script spans

Propose source-ordered episode spans and scene spans that both cover the supplied normalized script from code point 0 through `codepoint_count` without gaps or overlaps.

- Count Unicode code points, not UTF-8 bytes. Every range is zero-based and half-open: `[codepoint_start, codepoint_end)`.
- Episode spans are ordered by `position` starting at 1. Each episode lists every contained `scene_span_id` in source order, and every scene belongs to exactly one episode through `episode_span_id`.
- When the script has no explicit episode heading, emit one episode covering the full source with `heading` and `evidence` set to null. Never invent an episode title.
- When an episode heading exists, `heading` and `evidence` must either both be present or both be null. Present evidence points to the exact episode heading inside that episode span.
- A scene span starts at its scene heading and continues through its body until the next scene heading, or through `codepoint_count` for the last scene. The first span starts at `0`; every later start equals the previous end.
- `heading` is only the scene-heading text. Its `evidence` must point to that exact heading substring inside the same span. Never use the whole script or the whole scene as heading evidence.
- For every Evidence object, `exact_anchor` must equal `normalized_text[source_start:source_end]` by Unicode code point. Set `text_hash` to 64 lowercase zeroes; after verifying the range and exact anchor, the deterministic Harness replaces this placeholder with the lowercase SHA-256 hex digest of the anchor encoded as UTF-8.
- `coverage.codepoint_start` is `0`; `coverage.codepoint_end` and `coverage.covered_codepoints` both equal `codepoint_count`.

Use temporary episode and scene span keys only. Preserve genuinely ambiguous boundaries as typed review issues; do not resolve identities, apply a world preset, or create formal Episode or Scene records.

When `stage_input.repair` is present, return a complete valid ScriptSpan candidate after applying only its typed `change_spec` to the listed `target_keys` and `affected_scope_keys`. Use the frozen `issue_refs` and `evidence_refs` as the repair boundary, preserve every unaffected span exactly, and do not broaden the operation. The repair payload is authoritative; no free-form reviewer note is part of the model input.
