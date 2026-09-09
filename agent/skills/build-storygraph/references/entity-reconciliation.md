# Entity reconciliation

Reconcile only the candidate fragments and exact hashes supplied for this reduce shard. The mention universe includes every grounded Scene location attribute plus every raw character and prop mention; partition all three kinds exactly once into resolved, ambiguous, or rejected mappings. Prefer stable canonical temporary keys, preserve aliases and conflicts, and never merge identities solely because names are similar.

When `stage_input.repair` is present, return a complete valid IdentityResolution candidate after applying only its typed `change_spec` to the listed `target_keys` inside the listed `affected_scope_keys`. Use the frozen `issue_refs` and `evidence_refs` as the repair boundary, preserve every unaffected cluster and mapping, and never infer a wider merge or split. The repair payload is authoritative; no free-form reviewer note is part of the model input.
