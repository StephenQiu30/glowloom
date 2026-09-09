# Interaction and continuity

Reconcile only evidence-backed interactions and ordered state continuity from the supplied Scene
Facts, Production Entities, and Scene Bindings. Bind participants by exact actual occurrence keys,
never by names. Bind every state to the same formal identity and preserve story-time Scene order.

Use only `hold`, `carry`, `wear`, `use`, `give`, `receive`, `place`, `drop`, `open`, or `break`.
Transfers have one actor, one distinct counterparty, one Prop, and a single holder transition; never
emit duplicate give/receive claims. `open` and `break` require distinct before/after Prop states.
Geometry fields may be populated only when the frozen Scene Fact Evidence states them.

Continuity is either `state_persists` with the same state and no delta, or `state_changes` with two
states of the same identity and an explicit delta. Report missing participants, unexplained state
jumps, double holders, Prop teleportation, and uncertain geometry as blocking review issues. Do not
invent an identity, state, occurrence, visual preset, artifact, formal Owner row, or writable ledger.
