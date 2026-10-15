import {
  labelNames,
  validatePromptDraft,
  type PromptDraft,
  type LabelKind,
} from "./prompt-document";
/** Server allocations stay immutable; collisions in an explicitly retained local draft receive new numbers. */
export function reconcileLabels(
  content: PromptDraft,
  server: PromptDraft,
  known = content,
) {
  const draft = structuredClone(content),
    changes: string[] = [];
  const labels = structuredClone(server.labelState);
  const active = new Set(draft.references.map((r) => r.id));
  for (const kind of Object.keys(labels.highWater) as LabelKind[])
    labels.highWater[kind] = Math.max(
      labels.highWater[kind],
      known.labelState.highWater[kind],
    );
  for (const [id, allocation] of Object.entries(known.labelState.allocations)) {
    if (labels.allocations[id]) continue;
    const collision = Object.values(labels.allocations).some(
      (a) =>
        a.labelKind === allocation.labelKind &&
        a.ordinal === allocation.ordinal,
    );
    if (collision && !active.has(id)) continue; // Original recovery branch retains the colliding tombstone.
    const next = collision
      ? { ...allocation, ordinal: ++labels.highWater[allocation.labelKind] }
      : allocation;
    labels.allocations[id] = next;
    if (collision)
      changes.push(
        `${labelNames[allocation.labelKind]}${allocation.ordinal} → ${labelNames[next.labelKind]}${next.ordinal}`,
      );
  }
  draft.labelState = labels;
  validatePromptDraft(draft);
  return { draft, changes };
}
