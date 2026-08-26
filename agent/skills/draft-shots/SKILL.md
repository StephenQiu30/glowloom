---
name: draft-shots
description: Draft complete review-only storyboard table rows from one fixed scene seed and accepted visual plan. Use only when explicitly invoked for shot drafting; do not revise source or plan facts.
---

# Draft Shots

Realize one accepted scene plan as complete candidate shot rows for the key storyboard table. Return only the JSON object required by the harness-supplied schema.

## Harness boundary

- Treat the scene seed, semantic beats, accepted plan, source and dialogue positions, continuity state, asset versions, duration profile, hashes, and tool results as immutable.
- Produce candidate rows for human review only. Never claim that formal shots were created, approved, persisted, applied, published, or sent to generation.
- Do not write files or databases, run commands, call network or external services, or invoke business tools. The harness owns tools, gates, retries, assembly, timecodes, and persistence.
- Use only supplied position and identifier namespaces. Do not invent entity identifiers, plot facts, dialogue, characters, actions, props, locations, asset versions, or source state.
- Follow the supplied output schema exactly. Return no Markdown, commentary, or additional keys.

## Drafting responsibility

1. Realize every required source unit and accepted plan intention in one or more coherent rows. Repeated source references must add a distinct reaction, detail, setup, payoff, consequence, or reinterpretation.
2. Give each row one dominant narrative purpose and one readable contribution to the scene.
3. Fill the requested source, narrative role, risk, shot language, composition, environment, lighting, subject placement, facing, gaze, held-object state, dialogue, performance, ambience, sound-effect, and asset fields from confirmed inputs.
4. Use ordered, performable action beats. Preserve source order and fit action, dialogue, reaction, and travel inside the proposed duration.
5. Make `first_frame` a freezeable start state, `last_frame` the visible end state, and `keyframe_notes` only the essential changes between them.
6. Write explicit `continuity_in` and `continuity_out` for relevant position, direction, gaze, hand or prop state, continuing motion, entry or exit, light, and sound.
7. Use shot scale, angle, camera movement, and composition to direct attention. Static remains valid when movement has no dramatic job.
8. Bind only confirmed assets that are visibly required, preserving their supplied identity, version, and state.
9. Stay within the supplied duration profile and scene budget. Do not enforce a fixed shot count or universal minimum duration.

## Exclusions

- Do not rewrite source, beats, or the accepted scene plan. Surface a conflict in the provided risk fields rather than silently redesigning it.
- Do not calculate authoritative timecodes, validate gates, approve the result, or create a second persisted scene representation.
- Do not add speculative visual events solely to make an image or video prompt more elaborate.

Read [shot table rules](references/shot-table-rules.md) before constructing candidate rows.

## Provenance

This is project-owned guidance in original wording, informed by publicly reviewed concepts from [`shuohao-skills@0e5eb688`](https://github.com/eternityspring/shuohao-skills/tree/0e5eb688ebf1b45e45c9bec31543aaa59e67c7bc) (Apache-2.0) and [`drama-skills@7811065`](https://github.com/worldwonderer/drama-skills/tree/7811065c171f8b0a83230bb2e0ccfe2c2b5b337a) (MIT). Those repositories are research inputs only; this skill copies or invokes no upstream prose, script, output contract, or runtime dependency.
