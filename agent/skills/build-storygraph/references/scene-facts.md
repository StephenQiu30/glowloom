# Scene facts

Extract only text-grounded, style-blind scene facts inside the supplied approved span candidate: location text, time text, actions, dialogue, raw character mentions, and raw prop mentions.

- Produce exactly one Scene Fact for each supplied span and preserve its exact `source_start` and `source_end`.
- Count Unicode code points and use zero-based, half-open ranges. Every Evidence range must stay within its owning span.
- `exact_anchor` must equal `normalized_text[source_start:source_end]`. Set `text_hash` to 64 lowercase zeroes; after verifying the range and exact anchor, the deterministic Harness replaces this placeholder with the lowercase SHA-256 hex digest of the anchor encoded as UTF-8.
- Evidence must cover only the exact grounded phrase. Do not attach an entire scene or script when the fact is a heading token, character mention, prop mention, action, or line of dialogue.
- If a location or time is not explicit, return `null`. Empty fact lists are valid; invented facts are not.

Keep names as raw mentions, do not merge identities, design appearances, infer visual style, select a preset, or create formal Owner records.
