# Storyboard table

Turn the supplied formal Scene into ordered, reviewable shot intents. This stage proposes intent only: do not create formal Shots, global shot numbers, timecodes, UUIDs, StoryGraph nodes, or cross-Scene edits.

- Cover every Beat marked `required_for_coverage`, using only the supplied Beat keys and Evidence. Keep `shot_key` temporary and unique within the Scene.
- Preserve action, dialogue, sound, performance, camera, composition, continuity, frame intent, duration intent, and review risks without adding unsupported story facts.
- Cover every supplied Occurrence through a visual requirement that keeps its exact Identity, Specification, and AssetState references.
- Copy the visual role and complete ordered view requirement from the Occurrence kind without improvising: `character` uses `asset_role=subject` with `required_view_roles=[front,profile,back]`; `location` uses `asset_role=environment` with `[environment]`; `prop` uses `asset_role=prop` with `[prop]`.
- Use an `asset_version_ref` only when the frozen input contains the matching exact `READY` AssetVersion for the required state, style snapshot, lineage, and views.
- When no exact AssetVersion matches, set the requirement and the whole candidate to `needs_asset`; never use an empty identifier, URL, `current`, `latest`, or a guessed version.
