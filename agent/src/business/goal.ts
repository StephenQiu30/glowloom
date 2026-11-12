import type { JsonObject } from '@earendil-works/pi-durable';

/** Wire values owned by Go agent/domain, checked by the business schema test. */
export const GoalKind = {
  Chat: 'chat',
  Production: 'production',
} as const satisfies Record<string, AgentBusinessApi.GoalKind>;
export const OperationStatus = {
  Draft: 'draft',
  Quoted: 'quoted',
  Confirmed: 'confirmed',
  Submitting: 'submitting',
  Submitted: 'submitted',
  Succeeded: 'succeeded',
  Ingesting: 'ingesting',
  Cancelling: 'cancelling',
  Completed: 'completed',
  Failed: 'failed',
  Cancelled: 'cancelled',
  Expired: 'expired',
  Unknown: 'unknown',
  Reconciling: 'reconciling',
  Manual: 'manual',
} as const;
export const businessLimits = { maxDeliveries: 64 } as const;
export type GoalContract = JsonObject &
  Required<
    Pick<AgentBusinessApi.GoalContract, 'kind' | 'required_deliveries'>
  > &
  Pick<AgentBusinessApi.GoalContract, 'source' | 'delivery'>;

export function freezeGoalContract(value: unknown): GoalContract {
  if (value === undefined)
    return { kind: GoalKind.Chat, required_deliveries: 0 };
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw new Error('Invalid business goal contract');
  const goal = value as Record<string, unknown>;
  if (!Number.isSafeInteger(goal.required_deliveries))
    throw new Error('Invalid business goal count');
  if (goal.kind === GoalKind.Chat) {
    if (
      goal.required_deliveries !== 0 ||
      goal.source !== undefined ||
      goal.delivery !== undefined
    )
      throw new Error('Invalid chat goal');
  } else if (
    goal.kind !== GoalKind.Production ||
    Number(goal.required_deliveries) < 1 ||
    Number(goal.required_deliveries) > businessLimits.maxDeliveries
  ) {
    throw new Error('Invalid production goal');
  }
  // Go validates the formal canvas/timeline contracts. This boundary also refuses non-JSON pins.
  return JSON.parse(JSON.stringify(goal)) as GoalContract;
}

/** Publish bounded user-facing facts; never expose storage keys or procurement evidence. */
export function publicProductionFacts(value: unknown): JsonObject | null {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return null;
  const snapshot = value as Record<string, unknown>;
  const result: JsonObject = {};
  if (
    snapshot.cost &&
    typeof snapshot.cost === 'object' &&
    !Array.isArray(snapshot.cost)
  ) {
    const cost = snapshot.cost as Record<string, unknown>;
    const keys = [
      'authorized_limit',
      'pending_commitment',
      'settled_charge',
      'remaining',
    ];
    if (
      keys.every(
        (key) => Number.isSafeInteger(cost[key]) && Number(cost[key]) >= 0,
      ) &&
      typeof cost.currency === 'string' &&
      typeof cost.unit === 'string'
    ) {
      result.cost = Object.fromEntries(
        [...keys, 'currency', 'unit'].map((key) => [key, cost[key]]),
      ) as JsonObject;
    }
  }
  if (
    snapshot.goal_completion &&
    typeof snapshot.goal_completion === 'object' &&
    !Array.isArray(snapshot.goal_completion)
  ) {
    const goal = snapshot.goal_completion as Record<string, unknown>;
    result.goal_completion = Object.fromEntries(
      [
        'kind',
        'status',
        'required_deliveries',
        'completed_deliveries',
        'missing',
      ]
        .filter((key) => goal[key] !== undefined)
        .map((key) => [key, goal[key]]),
    ) as JsonObject;
  }
  if (
    Array.isArray(snapshot.shot_deliveries) &&
    snapshot.shot_deliveries.length <= businessLimits.maxDeliveries
  ) {
    result.deliveries = snapshot.shot_deliveries
      .filter(
        (item): item is Record<string, unknown> =>
          !!item && typeof item === 'object' && !Array.isArray(item),
      )
      .map((item) =>
        Object.fromEntries(
          [
            'delivery_id',
            'status',
            'video_export_id',
            'project_export_id',
            'canvas_id',
            'node_id',
            'timeline_node_id',
            'operation_id',
            'asset_id',
          ]
            .filter((key) => typeof item[key] === 'string')
            .map((key) => [key, String(item[key])]),
        ),
      );
  }
  return result;
}
