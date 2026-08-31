# Script spans

Propose source-ordered scene spans that cover the supplied normalized script from code point 0 through `codepoint_count` without gaps or overlaps.

- Count Unicode code points, not UTF-8 bytes. Every range is zero-based and half-open: `[codepoint_start, codepoint_end)`.
- A scene span starts at its scene heading and continues through its body until the next scene heading, or through `codepoint_count` for the last scene. The first span starts at `0`; every later start equals the previous end.
- `heading` is only the scene-heading text. Its `evidence` must point to that exact heading substring inside the same span. Never use the whole script or the whole scene as heading evidence.
- For every Evidence object, `exact_anchor` must equal `normalized_text[source_start:source_end]` by Unicode code point. Set `text_hash` to 64 lowercase zeroes; after verifying the range and exact anchor, the deterministic Harness replaces this placeholder with the lowercase SHA-256 hex digest of the anchor encoded as UTF-8.
- `coverage.codepoint_start` is `0`; `coverage.codepoint_end` and `coverage.covered_codepoints` both equal `codepoint_count`.

Use temporary span keys only. Preserve genuinely ambiguous boundaries as typed review issues; do not resolve identities, apply a world preset, or create formal Scene records.
