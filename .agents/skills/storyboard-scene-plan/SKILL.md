---
name: storyboard-scene-plan
description: Create a review-only visual plan for one scene from an accepted beat map and fixed project context. Use only when explicitly invoked for scene planning; do not draft final shot rows.
---

# Storyboard Scene Plan

Translate an accepted semantic beat map into a scene-level visual plan that a later shot drafter can realize. Return only the JSON object required by the harness-supplied schema.

## Harness boundary

- Treat the scene context, beat map, continuity state, asset state, duration budget, hashes, and deterministic tool results as immutable.
- Produce a candidate plan only. Never claim that shots, gates, checkpoints, or formal records were created, approved, persisted, or applied.
- Do not write files or databases, run commands, call network or external services, or invoke business tools. The harness owns tools, routing, retries, gates, and persistence.
- Use only source, dialogue, scene, and asset positions supplied by the payload. Never create new plot facts, dialogue, entities, or asset state.
- Follow the supplied output schema exactly. Return no Markdown, commentary, or additional keys.

## Planning responsibility

1. Preserve the accepted beat order and state what each visual passage must make readable.
2. Establish only the geography needed for later action: screen axis, spatial zones, subject relationships, entrances and exits, gaze, movement direction, and important object state.
3. Propose a scene rhythm and distribute the supplied duration budget across beats without fixing arbitrary universal shot lengths.
4. Identify justified coverage intentions such as an establishing view, receiver reaction, decisive insert, match action, or hook. Every intention must cite its source contribution.
5. Carry continuity from the supplied entry state to a clear proposed exit state, distinguishing confirmed facts from staging proposals in the schema fields provided.
6. Make lighting, atmosphere, and movement choices only when they clarify attention, emotion, space, or continuity.
7. Surface unsupported or conflicting staging as risks; do not repair the source or asset facts.

## Exclusions

- Do not rewrite the beat map or decide that a required beat can be omitted.
- Do not produce final shot rows, proposal identifiers, exact lens packages, dialogue rewrites, or production asset mutations.
- Do not perform gate validation or claim the plan is safe to persist.

Read [visual planning rules](references/visual-planning-rules.md) when choosing spatial, continuity, rhythm, or coverage intentions.

## Provenance

This guidance is original project wording informed by `eternityspring/shuohao-skills@0e5eb688ebf1b45e45c9bec31543aaa59e67c7bc` (Apache-2.0; copyright and NOTICE remain with the vendored source) and design research from `worldwonderer/drama-skills@7811065c171f8b0a83230bb2e0ccfe2c2b5b337a` (MIT). No upstream prose, scripts, or output contract is copied or invoked by this skill.
