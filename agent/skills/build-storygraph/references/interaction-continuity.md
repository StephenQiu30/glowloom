# Interaction and continuity

Reconcile only evidence-backed interactions and ordered state continuity from the supplied Scene
Facts, Production Entities, and Scene Bindings. Bind participants by exact actual occurrence keys,
never by names. Bind every state to the same formal identity and preserve story-time Scene order.

Emit every Scene exactly once in `scene_story_times`, sorted by its explicit `story_time_key`; do not
reuse Scene array, source, edit, or playback order as story time. Every claim has a stable series key,
positive revision, and nullable superseded claim key. Revision 1 has no superseded key; later
revisions require one. Keep interactions and continuity in canonical story-time order.

Use only `hold`, `carry`, `wear`, `use`, `give`, `receive`, `place`, `drop`, `open`, or `break`.
Transfers have one actor, one distinct counterparty, one Prop, and a single holder transition; never
emit duplicate give/receive claims. `give` transfers actor to counterparty; `receive` transfers
counterparty to actor. `carry` preserves the actor holder, `use` never transfers a holder, and
`place`/`drop` release the actor holder. Every Prop State change has one explicit `state_delta`;
`open` and `break` require that change. `hand` is typed, and relative scale is a reduced positive
rational. For every populated hand, grip, contact, direction, or scale field, emit the matching
`geometry_evidence` field from the same frozen Scene action; leave both the value and its Evidence
empty when the script does not state it.

Continuity is either `state_persists` with the same state and no delta, or `state_changes` with two
states of the same identity and an explicit delta. Emit one `continuity_ledger` entry for every
actual Character and Prop in every Scene. Each entry binds the exact occurrence state, story time,
actual Scene location when present, and the subject/location Evidence. A Prop entry also records its
single holder and the Interaction that produces its exit state. A changed Prop state, holder, or
location must be explained by that transition; only carry or transfer can explain a location change.
Every Interaction must be applied to exactly one ledger entry. Every adjacent Character ledger
boundary must bind an exact Continuity claim. Report missing participants, unexplained state jumps,
double holders, Prop teleportation, and uncertain geometry as blocking review issues. Do not invent
an identity, state, occurrence, visual preset, artifact, formal Owner row, or writable Owner ledger.

When `production_world_repair` is present, begin from its exact stage-specific `base_candidate`, use
only its frozen Evidence, and return a complete strict Candidate. Upstream Entity or Occurrence repair
may update only Interaction, Continuity, and ledger keys in the authorized closure. For
`revise_interaction`, only targeted Interactions and directly dependent closure rows may change; for
`revise_continuity`, Interactions remain frozen and only targeted Continuity plus its authorized ledger
rows may change. Preserve all unrelated Interactions, Continuity, ledger entries, story-time rows,
review issues, and evidence exactly. Never widen the closure or perform opportunistic cleanup.
