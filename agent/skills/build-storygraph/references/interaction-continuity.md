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
rational. Geometry fields may be populated only when the frozen Scene Fact Evidence states them.

Continuity is either `state_persists` with the same state and no delta, or `state_changes` with two
states of the same identity and an explicit delta. Report missing participants, unexplained state
jumps, double holders, Prop teleportation, and uncertain geometry as blocking review issues. Do not
invent an identity, state, occurrence, visual preset, artifact, formal Owner row, or writable ledger.
