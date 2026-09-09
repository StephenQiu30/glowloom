# Scene occurrences

Bind every formal Scene to its frozen Dialogues, Beats, and resolved identity mentions. Use only
the supplied `StructureIdentitySetVersion`, SceneFacts, and production entity/state fragments.
Every occurrence must reuse one formal identity key and one state key whose declared scope includes
that Scene. Never create an identity, state, Scene, UUID, preset, artifact, or formal business row.

Preserve `occurrence_role` exactly from the frozen identity mapping. An actual appearance cannot be
inferred from a `mentioned_only` mention. Emit one ordered occurrence for every resolved mention;
do not bind by name, fuzzy search, array position, or a current/latest pointer.

Dialogues and Beats must reproduce the frozen SceneFact text, Evidence, and order. Bind a dialogue
speaker only when its speaker Evidence exactly matches a resolved formal character mapping in the
same Scene; otherwise leave the speaker identity unresolved. Report uncertainty as a ReviewIssue.

When `production_world_repair` is present, begin from its exact stage-specific `base_candidate` and
return a complete strict Candidate. An upstream Entity repair may alter only Occurrences named by the
authorized closure. A `rebind_scene_occurrence` target may alter only that exact Occurrence, or the
Occurrences inside an explicitly targeted Scene. Dialogues, Beats, untargeted Scenes and untargeted
Occurrences remain exactly frozen. Use only the supplied Evidence; never infer broader cleanup or
accept a free-form note as repair authority.
