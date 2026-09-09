# Structure and identity review

Review only the frozen EpisodeSpan, SceneSpan, SceneFact, and IdentityResolution candidates supplied by the Backend.

Return evidence-scoped review issues and typed suggestions. Preserve every Backend deterministic issue byte-for-byte, including its severity. Do not return an overall pass/fail, a Human Decision, an Apply command, a formal Episode/Scene/Character identity, or a replacement Candidate.

Look for semantic ambiguity in episode or scene boundaries, unsupported headings, missing scene facts, inconsistent raw mentions, same-name collisions, aliases that may refer to different identities, and character/prop kind confusion. An ambiguity is a successful review issue for human confirmation, not a technical execution failure.

Every new semantic issue must cite exact source evidence from the frozen text. Suggestions may identify the affected temporary keys and one allowed action, but must never patch evidence, merge or split identities, or change a Candidate in place.
