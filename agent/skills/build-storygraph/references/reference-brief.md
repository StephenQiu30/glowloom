# Reference Brief compilation

Produce only one `ReferenceBriefCandidate` for the exact frozen Target. Do not approve or publish a Brief, call a media Provider, generate an image, choose a model, or replace any Backend-owned reference.

Copy the Workspace, Project, Approved Reference Plan, Target, Visual Foundation, Effective Style/Policy, dependency AssetVersion selections, Stage Release, typed read-set root, source refs, design focus, forbidden changes, and required view roles exactly. Treat an explicit empty dependency list as a frozen fact, not as permission to search for dependencies.

Use the purpose branch selected by `target_kind`:

- `character_identity_anchor`: preserve the complete identity invariant slots and front/profile/back views.
- `character_appearance`: inherit the selected identity anchor; change only approved variable slots and keep front/profile/back views.
- `location_board`: preserve topology, material, scale, and the empty-location occupancy policy.
- `prop_sheet`: preserve dimensions, structure, state/mechanism slots, and the no-hands/no-people occupancy policy.
- `scene_composition`: bind only the supplied scene closure and selected base AssetVersions.
- `interaction_composition`: bind only the supplied interaction, participants, hand/contact/orientation/scale, and selected base AssetVersions.

Return structured source-versus-design slots, layout/scale constraints, rights/provenance requirements, and QC rubric refs. Do not emit a free-form Provider prompt, Secret, `current`/`latest` pointer, extra identity, prop, state, scene, occurrence, interaction, dependency, or view role. If the frozen input is insufficient for the required strict branch, fail instead of inventing facts.
