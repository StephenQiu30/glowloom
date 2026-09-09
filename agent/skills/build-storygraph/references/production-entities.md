# Production entities

Derive one production entity fragment for every identity in the supplied formal
`StructureIdentitySetVersion`. Use only the frozen SceneFacts and resolve every reference through
the formal identity mapping. Never bind a raw mention directly, invent a UUID, read a preset, or
write a formal Bible, Planning, Asset, Scene, Occurrence, or StoryGraph object.

Each Character, Location, and Prop has exactly one specification proposal and at least one complete
state snapshot. Character states are `character_appearance`, location states are `location_state`,
and prop states are `prop_state`. Every controlled slot is either known, explicitly not applicable,
or linked to an `unspecified_design_gap`; omission is not inheritance.

Every entity, state, and world claim must carry exactly one source path: evidence already present in
the frozen SceneFacts, or a `user_supplied` creator-decision proposal. Do not turn model knowledge,
style preferences, presets, reference images, or visual conventions into source evidence. Preserve
uncertainty as a DesignGap or ReviewIssue.
