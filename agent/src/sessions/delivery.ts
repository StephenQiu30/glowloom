import type { JsonObject } from '@earendil-works/pi-durable';
import { RunStatus } from '../pi-durable/docs.js';
import { deliveryDefaults } from '../config.js';
import { canonicalHash } from './manifest.js';
import {
  GoalKind,
  OperationStatus,
  type GoalContract,
  businessLimits,
} from '../business/goal.js';

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
  onCloseFailure?: () => Promise<void>;
  deliveryPollMs?: number;
  deliveryTimeoutMs?: number;
  requireDeliveryFacts?: boolean;
  goalContract?: GoalContract;
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
): {
  status: string;
  wait: JsonObject | null;
  continueOperations?: string[];
  continueDeliveries?: string[];
} {
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
        items.length <= businessLimits.maxDeliveries &&
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
  const production = options.goalContract?.kind === GoalKind.Production;
  const completion =
    object(snapshot) && object(snapshot.goal_completion)
      ? snapshot.goal_completion
      : undefined;
  const validGoal =
    !production ||
    (!!completion &&
      completion.contract_hash === canonicalHash(options.goalContract) &&
      completion.kind === GoalKind.Production &&
      completion.required_deliveries ===
        options.goalContract?.required_deliveries &&
      Number.isSafeInteger(completion.completed_deliveries) &&
      Number(completion.completed_deliveries) >= 0 &&
      Array.isArray(completion.missing) &&
      completion.missing.every((item) => typeof item === 'string') &&
      (Object.values(DeliveryStatus) as string[]).includes(
        String(completion.status),
      ));
  const goalComplete =
    !production ||
    (validGoal &&
      completion?.status === DeliveryStatus.Completed &&
      Number(completion.completed_deliveries) >=
        Number(options.goalContract?.required_deliveries) &&
      Array.isArray(completion.missing) &&
      completion.missing.length === 0);
  if (production && valid && validGoal && goalComplete)
    return { status: RunStatus.Completed, wait: null };
  const oldOperationIds = Array.isArray(previous?.operation_ids)
    ? previous.operation_ids.filter(
        (id): id is string => typeof id === 'string',
      )
    : [];
  const operations =
    object(snapshot) && Array.isArray(snapshot.production_operations)
      ? snapshot.production_operations
      : [];
  const validOperations =
    operations.length <= businessLimits.maxDeliveries &&
    operations.every(
      (item) =>
        object(item) &&
        typeof item.operation_id === 'string' &&
        /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
          item.operation_id,
        ) &&
        (Object.values(OperationStatus) as string[]).includes(
          String(item.status),
        ),
    );
  const operationIds = validOperations
    ? operations.map((item) => String((item as JsonObject).operation_id)).sort()
    : [];
  const missingOperations = oldOperationIds.some(
    (id) => !operationIds.includes(id),
  );
  const duplicateOperations =
    new Set(operationIds).size !== operationIds.length;
  const expired =
    typeof deadline !== 'string' ||
    !Number.isFinite(Date.parse(deadline)) ||
    Date.parse(deadline) <= now;
  const wait: JsonObject = {
    delivery_ids: ids,
    operation_ids: oldOperationIds,
    first_wait_at: first,
    deadline_at: deadline,
    next_wakeup_at: new Date(
      now + (options.deliveryPollMs ?? deliveryDefaults.pollMs),
    ).toISOString(),
  };
  // A failed read is not evidence that an original operation disappeared.
  // Keep its identity and absolute deadline without admitting model work.
  if (production && snapshot === undefined)
    return {
      status: expired
        ? RunStatus.NeedsReconciliation
        : RunStatus.WaitingDelivery,
      wait: {
        ...wait,
        reason: expired
          ? 'delivery_deadline_exceeded'
          : 'delivery_facts_unavailable',
      },
    };
  if (
    production &&
    (!validOperations || missingOperations || duplicateOperations)
  )
    return {
      status: RunStatus.NeedsReconciliation,
      wait: {
        ...wait,
        operation_ids: [
          ...new Set([...oldOperationIds, ...operationIds]),
        ].sort(),
        reason: 'production_facts_unavailable',
      },
    };
  const unsafeOperation = operations.some(
    (item) =>
      object(item) &&
      (
        [
          OperationStatus.Unknown,
          OperationStatus.Reconciling,
          OperationStatus.Manual,
          OperationStatus.Failed,
          OperationStatus.Cancelled,
          OperationStatus.Expired,
        ] as string[]
      ).includes(String(item.status)),
  );
  const formalDeliveryIds = deliveries
    .flatMap((item) =>
      object(item) &&
      typeof item.delivery_id === 'string' &&
      typeof item.video_export_id === 'string' &&
      typeof item.project_export_id === 'string'
        ? [item.delivery_id]
        : [],
    )
    .sort();
  const requiredMissing =
    Array.isArray(completion?.missing) &&
    completion.missing.includes('required_delivery_missing');
  if (
    production &&
    valid &&
    validGoal &&
    !goalComplete &&
    requiredMissing &&
    operationIds.length
  ) {
    const ready = operations.every(
      (item) => object(item) && item.status === OperationStatus.Completed,
    );
    return {
      status:
        expired || unsafeOperation
          ? RunStatus.NeedsReconciliation
          : RunStatus.WaitingDelivery,
      wait: {
        ...wait,
        operation_ids: operationIds,
        reason: unsafeOperation
          ? 'production_operation_requires_reconciliation'
          : expired
            ? 'delivery_deadline_exceeded'
            : ready
              ? 'production_ready'
              : 'production_operation_pending',
      },
      ...(!expired && !unsafeOperation && ready
        ? {
            continueOperations: operationIds,
            continueDeliveries: formalDeliveryIds,
          }
        : {}),
    };
  }
  if (
    valid &&
    validGoal &&
    goalComplete &&
    !missing &&
    deliveries.every(
      (item) => object(item) && item.status === DeliveryStatus.Completed,
    )
  )
    return { status: RunStatus.Completed, wait: null };
  const reason =
    !valid || !validGoal || missing
      ? 'delivery_facts_unavailable'
      : production && !goalComplete && !deliveries.length
        ? 'production_goal_incomplete'
        : deliveries.some(
              (item) =>
                object(item) && item.status === DeliveryStatus.Incomplete,
            )
          ? 'delivery_incomplete'
          : 'delivery_pending';
  return {
    status:
      expired ||
      reason === 'delivery_incomplete' ||
      reason === 'production_goal_incomplete'
        ? RunStatus.NeedsReconciliation
        : RunStatus.WaitingDelivery,
    wait: {
      ...wait,
      reason: expired ? 'delivery_deadline_exceeded' : reason,
    },
  };
}
