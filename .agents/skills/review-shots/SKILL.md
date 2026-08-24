---
name: review-shots
description: Independently review assembled storyboard candidates and return evidence-scoped issues without editing them. Use only when explicitly invoked for semantic director review, not drafting or repair.
---

# Review Shots

Audit the supplied candidate storyboard against the fixed source, accepted plans, continuity ledger, profile, and deterministic gate results. Return only the JSON object required by the harness-supplied schema.

## Harness boundary

- Treat all source, plans, candidate rows, asset state, hashes, and deterministic gate results as immutable evidence.
- Produce a review report only. Do not edit, repair, approve, persist, apply, publish, or send any shot to generation.
- Do not write files or databases, run commands, call network or external services, or invoke business tools. The harness validates issue scope and controls routing, repair, retries, and persistence.
- Never downgrade, suppress, or reinterpret a deterministic blocker as passed. Do not claim that a hard gate ran unless its result is present.
- Follow the supplied output schema exactly. Return no Markdown, commentary, revised rows, or additional keys.

## Review responsibility

1. Review independently instead of rationalizing drafting decisions.
2. For every issue, provide the schema-required severity, exact scope, concrete evidence, and bounded repair hint. Evidence must cite supplied source positions, shot keys, fields, or adjacent values.
3. Report unsupported source realization, duplicated action without new contribution, missing consequential reaction, spatial or axis discontinuity, gaze or position jumps, broken motion, prop-state conflict, action overload, unperformable duration, dialogue mismatch, first-frame leakage, incomplete end state, asset conflict, and unsupported reveal or hook treatment.
4. Distinguish a material semantic or continuity failure from an aesthetic preference. Use warnings for non-blocking craft concerns unless the supplied policy makes the consequence objectively blocking.
5. Keep issue scope as narrow as the evidence permits so repair can be targeted.
6. If no supported issue exists, return the schema-valid empty issue result without inventing criticism.

## Exclusions

- Do not rewrite rows, introduce alternative coverage, change the scene plan, or choose among repair solutions.
- Do not recompute authoritative source membership, coverage, duration, asset validity, or hashes; deterministic tools own those facts.
- Do not treat personal taste, shot variety, or a preferred camera style as a hard gate.

Read [review rubric](references/review-rubric.md) before assigning issue scope and severity.

## Provenance

This is project-owned guidance in original wording, informed by publicly reviewed concepts from [`shuohao-skills@0e5eb688`](https://github.com/eternityspring/shuohao-skills/tree/0e5eb688ebf1b45e45c9bec31543aaa59e67c7bc) (Apache-2.0) and [`drama-skills@7811065`](https://github.com/worldwonderer/drama-skills/tree/7811065c171f8b0a83230bb2e0ccfe2c2b5b337a) (MIT). Those repositories are research inputs only; this skill copies or invokes no upstream prose, script, output contract, or runtime dependency.
