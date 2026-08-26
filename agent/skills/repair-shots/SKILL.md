---
name: repair-shots
description: Repair only failed or review-blocked fields in storyboard candidates using fixed evidence and adjacent context. Use only when explicitly invoked for one targeted repair pass; do not rewrite passed scenes.
---

# Repair Shots

Produce the smallest source-grounded candidate patch or replacement rows that address the supplied issues. Return only the JSON object required by the harness-supplied schema.

## Harness boundary

- Treat the original source, accepted beat map and plan, failed gate results, review issues, affected rows, neighboring context, asset state, hashes, and passed scope as immutable.
- Produce a repair candidate only. Never claim that an issue was cleared, a gate passed, a checkpoint was updated, or a formal shot was approved, persisted, applied, or published.
- Do not write files or databases, run commands, call network or external services, or invoke business tools. The harness owns retry counts, validation, routing, assembly, and persistence.
- Use only supplied positions, identifiers, source facts, dialogue, entities, and asset versions. Never invent facts to make a blocker disappear.
- Follow the supplied output schema exactly. Return no Markdown, commentary, or additional keys.

## Repair responsibility

1. Address each supplied blocker or selected warning directly and retain its issue or gate reference in the schema field provided.
2. Change only the affected fields or rows plus the minimum adjacent boundary needed for continuity. Preserve all passed scenes and unrelated row content.
3. Preserve proposal keys, source positions, ordering, and accepted plan intentions unless the issue explicitly requires a scoped split, merge, or boundary change and the schema permits it.
4. Keep replacements source-grounded, performable within the supplied duration profile, compatible with confirmed assets, and explicit about `first_frame`, `last_frame`, and continuity changes.
5. When repairing one boundary, reconcile both sides of the handoff without silently changing a third shot.
6. If the supplied evidence cannot support a valid repair, return the schema's unresolved result and concise reason instead of broadening scope or fabricating content.
7. Perform one repair pass only. The harness enforces the maximum attempts and reruns deterministic and semantic validation.

## Exclusions

- Do not freely redraft a passed scene, downgrade severity, delete an issue, weaken a gate, or change source and asset facts.
- Do not approve the repaired candidate or assert that validation will pass.
- Do not create alternate versions outside the exact output contract.

## Provenance

This is project-owned guidance in original wording, informed by publicly reviewed concepts from [`shuohao-skills@0e5eb688`](https://github.com/eternityspring/shuohao-skills/tree/0e5eb688ebf1b45e45c9bec31543aaa59e67c7bc) (Apache-2.0) and [`drama-skills@7811065`](https://github.com/worldwonderer/drama-skills/tree/7811065c171f8b0a83230bb2e0ccfe2c2b5b337a) (MIT). Those repositories are research inputs only; this skill copies or invokes no upstream prose, script, output contract, or runtime dependency.
