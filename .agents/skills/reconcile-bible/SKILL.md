---
name: reconcile-bible
description: Reconcile whole-script evidence into candidate global identities, aliases, states, and world entries. Use only when explicitly invoked by a Bible harness; do not extract new facts, confirm entities, or persist assets.
---

# Reconcile Bible

Reconcile the complete set of validated local evidence into one candidate project Production Bible. Return only the JSON object required by the harness-supplied schema.

## Harness boundary

- Treat the document revision, evidence candidates, exact source ranges, episode mapping, validation results, and identifier namespaces as immutable.
- Produce a reconciled candidate only. Never claim that an identity, alias, state, world entry, asset, or asset version was confirmed, persisted, materialized, linked, or approved.
- Do not write files or databases, run commands, call network or external services, or invoke business tools. The harness owns evidence validation, checkpoints, deterministic gates, human decisions, confirmation, and persistence.
- Follow the supplied output schema exactly. Return no Markdown, commentary, or additional keys.

## Evidence preservation

1. Every identity, alias, state, relationship, timeline claim, and world entry must retain the validated document-absolute evidence references that support it.
2. Do not create a new quotation, widen or move a source range, or use an uncited summary as evidence. Reuse only supplied evidence references.
3. A normalized name or concise description is organizational metadata, not evidence. It must not add facts absent from the cited observations.
4. When the evidence does not support one resolution, preserve the competing candidates and emit the schema's ambiguity or conflict issue.

## Harness reference protocol

- The reconcile input contains one global `evidence_catalog` object. Each short key such as `e0001` maps to one immutable, complete Evidence object.
- The input `observations` array is flattened across chunks. An observation carries `chunk_key` for provenance and cites evidence only through `evidence_refs`; it never embeds Evidence objects.
- Treat an observation's `evidence_refs` as the allowed evidence for that observation. The presence of an Evidence object elsewhere in the catalog does not support an unrelated claim.
- The output is still the formal `ProductionBibleProviderResult`, whose evidence fields require complete Evidence objects. Resolve each used reference through `evidence_catalog` and copy that catalog value unchanged into the relevant output evidence list.
- Never emit `evidence_key`, `evidence_refs`, reference wrappers, or catalog-only fields in the output. Never alter a resolved Evidence object's range, hash, anchor, or episode number. A catalog object may be copied into multiple output claims when the cited observations support each claim.
- If a required reference is absent or cannot be resolved, do not reconstruct it. Preserve the uncertainty or emit a review issue using only other valid referenced Evidence.

## Reconciliation responsibility

1. Propose one stable identity for mentions that the evidence explicitly or cumulatively supports as the same subject. Record canonical name, aliases, kind, relationships, episodes, and stable traits only to the supported extent.
2. Keep identity separate from time-varying state. Different clothing, uniforms, age phases, injuries, disguises, public roles, rank presentation, faction phases, or carried-object conditions for the same person must remain states of one candidate identity when the evidence supports that continuity.
3. Do not merge solely because names are similar, a title is shared, or two descriptions look alike. Do not split solely because appearance, role presentation, or condition changes.
4. Keep genuinely distinct people, places, or objects separate when the source distinguishes them, including namesakes, twins, doubles, replicas, and similarly named locations.
5. Build state timelines only from supplied episode or range evidence. Preserve unknown boundaries, discontinuous episodes, coexistence, and contradictory observations instead of inventing transitions.
6. Keep compatible state dimensions composable: for example, injury and costume may coexist. Flag overlapping states only when their claims are mutually exclusive within the same dimension.
7. Reconcile world entries at the narrowest supported scope. Preserve exceptions, viewpoint uncertainty, historical change, and conflicting accounts rather than flattening them into one universal rule.
8. Surface alias collisions, kind conflicts, uncertain merges or splits, unsupported stable traits, contradictory timelines, and world-rule conflicts as explicit review issues.

## Asset specification contract

Every entity must contain an evidence-backed `base` state. Additional visible or audible phases use additional state keys on that same entity. Never leave an entity without a state, and never represent a costume, injury, age phase, disguise, rank presentation, location condition, prop condition, or voice phase as a second identity.

`stable_spec` and every `state_spec` are later merged into the existing project Asset schema. Use only the fields below, with exactly these value types. Omit unknown values; do not create descriptive convenience keys such as `role`, `function`, `condition`, `costume_condition`, `description`, or `notes`. The harness supplies the entity `kind`, so the nested specs may omit `kind`; if included, it must equal the entity kind.

- `character`: `identity: string`, `appearance: string`, `age_impression: string`, `temperament: string[]`, `goals: string[]`, `relationships: string[]`, `arc_summary: string`, `voice_profile: string`.
- `location`: `spatial_description: string`, `time_weather: string`, `visual_elements: string[]`, `lighting: string`.
- `prop`: `appearance: string`, `material: string`, `usage_context: string`. Omit `holder_character_id`; entity keys are not asset UUIDs.
- `costume`: `appearance: string`, `material: string`, `usage_context: string`. Omit `wearer_character_id`; entity keys are not asset UUIDs.
- `visual_style`: `visual_language: string`, `palette: string`, `lighting_language: string`, `negative_constraints: string[]`.
- `voice`: `source_kind` is `synthetic_recording`, `human_recording`, `voice_clone`, or null; `language: string`, `performance_traits: string[]`, `allowed_usage: string[]`.

Put stable supported facts in `stable_spec`. Put only the supported differences for one phase in its `state_spec`; the `base` state may use an empty object when the stable spec already describes it. Every state still requires exact evidence and episode numbers. Unsupported but important facts belong in a source-grounded world entry, ambiguity, or review issue instead of an invented spec field.

## Exclusions

- Do not add missing biography, appearance, motivation, relationship, geography, chronology, terminology, visual style, or world logic.
- Do not choose a formal asset identifier, create an asset state or version, or bind an occurrence unless the supplied schema explicitly represents those values as candidate references.
- Do not discard minority or conflicting evidence to make the Bible appear internally consistent.
- Do not generate storyboards, image prompts, character views, location art, or production assets.

## Ownership

This is project-owned guidance with no runtime dependency on external repository checkouts or upstream scripts.
