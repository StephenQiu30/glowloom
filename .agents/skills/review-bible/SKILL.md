---
name: review-bible
description: Independently review a reconciled Production Bible candidate against exact whole-script evidence. Use only when explicitly invoked for Bible review; report scoped issues without editing, confirming, or persisting the candidate.
---

# Review Bible

Audit the supplied candidate Production Bible against its validated evidence, document mapping, and deterministic gate results. Return only the JSON object required by the harness-supplied schema.

## Harness boundary

- Treat source text, document revision, validated evidence ranges, reconciled candidate, episode mapping, hashes, and deterministic gate results as immutable.
- Produce a review report only. Do not edit, repair, confirm, persist, materialize, approve, or publish any identity, state, world entry, asset, or occurrence.
- Do not write files or databases, run commands, call network or external services, or invoke business tools. The harness owns validation, routing, human decisions, confirmation, and persistence.
- Never downgrade, suppress, or reinterpret a deterministic blocker as passed. Do not claim that a gate ran unless its result is supplied.
- Follow the supplied output schema exactly. Return no Markdown, commentary, candidate replacements, or additional keys.

## Review responsibility

1. Verify that each material claim is traceable to a supplied document-absolute range and that every verbatim value matches its cited source slice exactly. If the required source slice is unavailable, report it as unverifiable instead of reconstructing it.
2. Report unsupported facts, missing evidence, invalid or chunk-local ranges, episode mapping errors, evidence assigned to the wrong entity, and summaries that introduce new information.
3. Detect likely false merges and false splits. In particular, flag one character split into multiple identities only because clothing, age phase, injury, disguise, title, rank presentation, allegiance phase, or other state changed.
4. Also flag distinct subjects merged through similar names, shared titles, namesakes, twins, doubles, replicas, or insufficient pronoun evidence.
5. Check alias uniqueness within each entity kind, kind consistency, unresolved references, relationship direction, state dimension, temporal scope, and mutually exclusive same-dimension state overlap.
6. Check world entries for unsupported generalization, lost exceptions, viewpoint presented as fact, contradictory rules, incorrect chronology, and conflicts concealed by normalization.
7. Distinguish objective identity, evidence, timeline, or rule failures from optional editorial preferences. Apply severity only according to the supplied policy and concrete downstream consequence.
8. Scope every issue to the narrowest candidate key, field, evidence reference, episode, or state boundary that supports it, and provide a bounded repair hint without performing the repair.
9. If no supported issue exists, return the schema-valid empty issue result. Do not assert confirmation, persistence, or materialization eligibility beyond the supplied gate status.
10. Report an entity with no evidence-backed `base` state as blocking. Report any `stable_spec` or `state_spec` field outside the project Asset contract supplied by the harness as blocking; do not silently translate or discard unknown fields.

## Exclusions

- Do not rewrite canonical names, aliases, identity descriptions, states, timelines, relationships, or world entries.
- Do not invent evidence, propose creative additions, generate production assets, or review storyboard aesthetics.
- Do not approve ambiguous merges simply because they reduce entity count, and do not require separate identities merely because visual references differ.

## Ownership

This is project-owned guidance with no runtime dependency on external repository checkouts or upstream scripts.
