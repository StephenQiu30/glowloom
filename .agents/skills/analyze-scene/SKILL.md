---
name: analyze-scene
description: Analyze one immutable scene into source-grounded semantic beats for a storyboard agent harness. Use only when explicitly invoked for source analysis; do not plan or draft shots.
---

# Analyze Scene

Turn the supplied single-scene narrative units into a review-only semantic beat map. Return only the JSON object required by the harness-supplied schema.

## Harness boundary

- Treat the payload, source positions, dialogue, scene membership, confirmed state, hashes, and tool results as immutable.
- Produce candidate analysis only. Never claim that a scene, shot, gate, checkpoint, or formal record was created, approved, persisted, or applied.
- Do not write files or databases, run commands, call network or external services, or invoke business tools. The harness owns tools, routing, retries, gates, and persistence.
- Reference source facts only through positions supplied in the payload. Never invent or rewrite plot, dialogue, characters, actions, props, locations, or state.
- Follow the supplied output schema exactly. Return no Markdown, commentary, or additional keys.

## Analysis responsibility

1. Organize the scene's narrative units into coherent semantic beats while preserving their order and meaning.
2. Give each beat one dominant dramatic function such as setup, conflict, action, reveal, reaction, transition, or hook when supported by the source.
3. State the source-grounded change in information, power, emotion, physical state, or audience expectation.
4. Make action, dialogue, and receiver reaction relationships explicit. Do not infer a reaction that the source cannot support.
5. Carry known performers, objects, spatial facts, and entry or exit facts as source observations, not as invented staging.
6. Cover every unit marked required for coverage. A unit may support more than one analytical relationship, but its source meaning must remain singular.
7. Record ambiguity or contradiction in the schema's risk fields instead of silently resolving source facts.

## Exclusions

- Do not choose shot count, lens, scale, angle, camera movement, composition, asset version, or final duration.
- Do not produce a scene visual plan, storyboard rows, review verdicts, or repairs.
- Do not turn optional atmosphere into a new event or make creative inferences look like confirmed facts.

## Provenance

This is project-owned guidance in original wording, informed by publicly reviewed concepts from [`shuohao-skills@0e5eb688`](https://github.com/eternityspring/shuohao-skills/tree/0e5eb688ebf1b45e45c9bec31543aaa59e67c7bc) (Apache-2.0) and [`drama-skills@7811065`](https://github.com/worldwonderer/drama-skills/tree/7811065c171f8b0a83230bb2e0ccfe2c2b5b337a) (MIT). Those repositories are research inputs only; this skill copies or invokes no upstream prose, script, output contract, or runtime dependency.
