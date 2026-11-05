import type { JsonObject } from '@earendil-works/pi-durable';
import type { BusinessTool } from '../../sessions/manifest.js';
export const ROLES = [
  'script',
  'art',
  'storyboard',
  'shot',
  'voice',
  'edit',
  'qa',
] as const;
export function explicitRoleToolNames(
  tools: readonly BusinessTool[],
  role: string,
) {
  return tools
    .filter((t) => role === 'producer' || t.roles?.includes(role))
    .map((t) => t.name);
}
export function allocateBudget(
  state: { model_steps: number; tool_steps: number; delegations: JsonObject[] },
  requested?: { model_steps: number; tool_steps: number },
) {
  const active = state.delegations.filter((x) => x.status === 'running');
  if (active.length >= 4)
    throw new Error('Specialist concurrency limit exceeded');
  const free = (kind: 'model' | 'tool') =>
    40 -
    state[`${kind}_steps`] -
    active.reduce(
      (n, x) =>
        n +
        Math.max(0, Number(x[`${kind}_budget`]) - Number(x[`${kind}_used`])),
      0,
    );
  const model = requested?.model_steps ?? Math.min(8, free('model'));
  const tool = requested?.tool_steps ?? Math.min(8, free('tool'));
  if (
    !Number.isSafeInteger(model) ||
    !Number.isSafeInteger(tool) ||
    model < 1 ||
    tool < 0 ||
    model > free('model') ||
    tool > free('tool')
  )
    throw new Error('Specialist allocation exceeds remaining root budget');
  return {
    model_budget: model,
    tool_budget: tool,
    model_used: 0,
    tool_used: 0,
  };
}
export function adoptedSources(
  adopted: readonly JsonObject[],
  requested: readonly string[],
) {
  const allowed = new Set(
    adopted.flatMap((x) => [
      typeof x.delegation_id === 'string' ? x.delegation_id : '',
      ...(Array.isArray(x.source_refs)
        ? x.source_refs.filter((s): s is string => typeof s === 'string')
        : []),
    ]),
  );
  if (requested.some((x) => !allowed.has(x)))
    throw new Error('Source reference has not been adopted');
  return [...requested];
}
