import type { JsonObject } from '@earendil-works/pi-durable';
import { RunStatus } from '../pi-durable/docs.js';
import { deliveryDefaults } from '../config.js';

// Wire values of Go's agent/domain.DeliveryStatus. They are business facts,
// distinct from the Harness run and submission states.
export const DeliveryStatus = {
  Pending: 'pending',
  AwaitingReview: 'awaiting_review',
  Incomplete: 'incomplete',
  Completed: 'completed',
} as const;

export const deliveryCreateTool = 'shot_delivery_create';

export interface DeliveryOptions {
  deliveryPollMs?: number;
  deliveryTimeoutMs?: number;
  requireDeliveryFacts?: boolean;
}

function object(value: unknown): value is JsonObject {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

// Only the owning Go application can establish completion. Model text and a
// successful tool submission cannot substitute for the original export facts.
export function deliveryOutcome(
  snapshot: unknown,
  previous: JsonObject | null,
  options: DeliveryOptions,
  now = Date.now(),
): { status: string; wait: JsonObject | null } {
  const first = previous?.first_wait_at ?? new Date(now).toISOString();
  const deadline =
    previous?.deadline_at ??
    new Date(
      now + (options.deliveryTimeoutMs ?? deliveryDefaults.timeoutMs),
    ).toISOString();
  const oldIds = Array.isArray(previous?.delivery_ids)
    ? previous.delivery_ids.filter((id): id is string => typeof id === 'string')
    : [];
  const items = object(snapshot) ? snapshot.shot_deliveries : undefined;
  const deliveries = Array.isArray(items) ? items : [];
  const valid =
    snapshot !== undefined &&
    object(snapshot) &&
    ((items === undefined &&
      !options.requireDeliveryFacts &&
      previous === null) ||
      (Array.isArray(items) &&
        items.length <= 64 &&
        items.every(
          (item) =>
            object(item) &&
            typeof item.delivery_id === 'string' &&
            /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
              item.delivery_id,
            ) &&
            (Object.values(DeliveryStatus) as string[]).includes(
              String(item.status),
            ),
        )));
  const ids = [
    ...new Set([
      ...oldIds,
      ...deliveries.flatMap((item) =>
        object(item) && typeof item.delivery_id === 'string'
          ? [item.delivery_id]
          : [],
      ),
    ]),
  ].sort();
  const missing = oldIds.some(
    (id) => !deliveries.some((item) => object(item) && item.delivery_id === id),
  );
  if (
    valid &&
    !missing &&
    deliveries.every(
      (item) => object(item) && item.status === DeliveryStatus.Completed,
    )
  )
    return { status: RunStatus.Completed, wait: null };
  const reason =
    !valid || missing
      ? 'delivery_facts_unavailable'
      : deliveries.some(
            (item) => object(item) && item.status === DeliveryStatus.Incomplete,
          )
        ? 'delivery_incomplete'
        : 'delivery_pending';
  const expired =
    typeof deadline !== 'string' ||
    !Number.isFinite(Date.parse(deadline)) ||
    Date.parse(deadline) <= now;
  return {
    status:
      expired || reason === 'delivery_incomplete'
        ? RunStatus.NeedsReconciliation
        : RunStatus.WaitingDelivery,
    wait: {
      delivery_ids: ids,
      first_wait_at: first,
      deadline_at: deadline,
      next_wakeup_at: new Date(
        now + (options.deliveryPollMs ?? deliveryDefaults.pollMs),
      ).toISOString(),
      reason: expired ? 'delivery_deadline_exceeded' : reason,
    },
  };
}
