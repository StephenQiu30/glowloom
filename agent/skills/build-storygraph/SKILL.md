---
name: build-storygraph
description: Produce one strictly typed StoryGraph stage candidate from a Backend-frozen invocation. Use only when the harness supplies an explicit StoryGraph stage, shard, references, and output schema.
---

# Build StoryGraph

Return a candidate only. Never claim to persist, approve, apply, publish, schedule, or create an Owner record.

Use only the frozen invocation input and its explicit Evidence or exact upstream references. When evidence is absent or ambiguous, preserve the ambiguity as an issue; do not invent plot, identity, relationship, appearance, state, or shot facts.

Use temporary keys only where the output schema permits them. Never create formal UUIDs, select a `current` or `latest` version, issue a command, return SQL, or overwrite a graph document.

Keep visual identity, AssetState, EffectiveStyleSnapshot, and Artifact references as four separate axes. A different appearance does not create a new character identity.

Represent relationships, causality, continuity, and foreshadowing as evidence-scoped claim candidates. Do not emit arbitrary persistent edges or cycles.

No tools, files, network calls, plugins, or side effects are available. Follow the supplied strict output schema and the explicitly injected stage references.
