---
name: extract-bible-evidence
description: Extract source-grounded Production Bible evidence candidates from one immutable full-script chunk using document-absolute spans. Use only when explicitly invoked by a Bible harness; do not reconcile across chunks or persist facts.
---

# Extract Bible Evidence

Extract local evidence candidates for project-level characters, locations, props, costumes, sounds, visual motifs, relationships, timeline events, terms, factions, and world rules. Return only the JSON object required by the harness-supplied schema.

## Harness boundary

- Treat the document revision, source hash, chunk text, document-absolute chunk bounds, episode mapping, coordinate convention, and identifiers as immutable.
- Produce evidence candidates only. Never claim that an entity was globally identified, reconciled, reviewed, confirmed, persisted, materialized as an asset, or made available to later episodes or storyboards.
- Do not write files or databases, run commands, call network or external services, or invoke business tools. The harness owns chunking, checkpoints, deterministic validation, global reconciliation, review, confirmation, and persistence.
- Follow the supplied output schema exactly. Return no Markdown, commentary, or additional keys.

## Evidence contract

1. Every observation must cite at least one document-absolute, half-open source range using the coordinate unit declared by the harness. Never return chunk-local offsets as document positions.
2. Each verbatim evidence value must equal the original source slice at its cited range exactly. Preserve spelling, capitalization, whitespace, and punctuation; do not normalize the quotation.
3. Cite an evidence catalog item as one indivisible object: copy its `source_start`, `source_end`, `text_hash`, `exact_anchor`, and `episode_number` unchanged. Never recalculate, abbreviate, or retype the hash.
4. Derive episode numbers only from the supplied document-to-episode mapping. Do not infer an episode from narrative chronology or nearby headings when the mapping is absent.
5. Keep distinct observations distinct when their ranges or meanings differ. Do not combine separate passages into a quotation or extend a range to include unsupported context.
6. If an exact absolute range cannot be established, return the schema's unresolved result or omit the unsupported observation. Never estimate a position.
7. Set `parent_entity_key` only when `kind` is `entity_state`. For `entity` and `world_entry` observations it must be `null`; every `entity_state` observation must name its parent entity key.

## Extraction responsibility

1. Record names, aliases, titles, pronouns, relationships, visible traits, locations, objects, costumes, sounds, factions, terminology, events, and explicit world facts only when supported by the supplied chunk.
2. Separate stable identity evidence from state evidence. Clothing, age phase, injury, disguise, rank presentation, allegiance phase, carried items, and other time-varying appearance or status belong to state observations; they do not by themselves establish a new character.
3. Preserve all locally plausible identity readings when a name, title, pronoun, disguise, or relationship is ambiguous. Mark the ambiguity instead of selecting an unsupported identity.
4. Describe world facts and rules at the narrowest level supported by the text. Do not convert one event into a universal rule, or implication into confirmed lore.
5. Preserve contradictions and changes as separate evidence observations. Do not repair continuity or decide which passage is authoritative.
6. Limit local grouping to mentions that the supplied chunk directly connects. Cross-chunk alias resolution and global identity decisions belong to reconciliation.

## Exclusions

- Do not invent legal names, ages, ethnicity, physical details, backstory, motives, relationships, chronology, geography, visual style, or world rules to complete a profile.
- Do not turn transient expression, pose, camera treatment, or staging into a persistent asset state.
- Do not generate image prompts, character sheets, scene plans, storyboard rows, or production-ready asset descriptions.
- Do not silently merge similar names, titles, locations, or objects, and do not split one subject solely because its appearance or status changes.

## Ownership

This is project-owned guidance with no runtime dependency on external repository checkouts or upstream scripts.
