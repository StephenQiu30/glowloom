import { BACKGROUND_CONTEXT as context } from '@earendil-works/chord/context';
import type {
  Conversation,
  Harness,
  JsonObject,
  TaskId,
} from '@earendil-works/pi-durable';
import {
  BusinessClient,
  type CallIdentity,
  type CallReceipt,
} from '../integrations/go-business-api/client.js';
import { StateDoc } from '../runtime/pi-durable/docs.js';

function identity(value: unknown): CallIdentity | undefined {
  if (!value || typeof value !== 'object') return;
  const c = value as Record<string, unknown>;
  if (
    [
      'task_id',
      'call_id',
      'checkpoint_id',
      'contract_version',
      'input_hash',
    ].some((k) => typeof c[k] !== 'string') ||
    !Number.isSafeInteger(c.attempt) ||
    Number(c.attempt) < 1
  )
    return;
  return c as unknown as CallIdentity;
}
function sameCall(a: CallIdentity, b: CallIdentity) {
  return (
    a.task_id === b.task_id &&
    a.call_id === b.call_id &&
    a.checkpoint_id === b.checkpoint_id &&
    a.attempt === b.attempt &&
    a.contract_version === b.contract_version &&
    a.input_hash === b.input_hash
  );
}

/** Reconcile original Go receipts after Pi has terminally interrupted a submission. Never execute or revive it. */
export async function reconcileInterrupted(input: {
  harness: Harness;
  root: Conversation;
  business: BusinessClient;
  runId: string;
  segmentId: string;
}): Promise<{
  status: 'completed' | 'needs_reconciliation';
  calls: JsonObject[];
}> {
  const { harness, root, business, runId, segmentId } = input;
  const state = await harness.snapshot(StateDoc, root.id, context);
  const calls: JsonObject[] = [];
  let final: CallReceipt | undefined;
  const reservations = (state?.reserved_calls ?? []).filter(
    (x) => x.segment_id === segmentId,
  );
  for (const reserved of reservations) {
    const call = identity(reserved.call);
    if (!call) {
      calls.push({
        kind: reserved.kind ?? 'unknown',
        status: 'invalid_identity',
      });
      continue;
    }
    if (reserved.origin === 'local') {
      const taskId = Number(call.task_id);
      const task = Number.isSafeInteger(taskId)
        ? await harness.getTask(taskId as TaskId, context)
        : undefined;
      calls.push({
        kind: reserved.kind ?? 'tool',
        source: 'official_task',
        call: { ...call },
        status:
          task?.state.status === 'terminal' &&
          task.state.outcome.status === 'completed'
            ? 'completed'
            : 'unknown',
      });
      continue;
    }
    const receipt = await business.findCall(
      { root_run_id: runId, submission_id: segmentId },
      call,
    );
    if (
      receipt &&
      (receipt.root_run_id !== runId ||
        receipt.submission_id !== segmentId ||
        !sameCall(receipt.call, call))
    )
      throw new Error('Business reconciliation receipt identity mismatch');
    const proof: JsonObject = {
      kind: reserved.kind ?? 'unknown',
      segment_id: segmentId,
      call: { ...call },
      status: receipt?.status ?? 'unknown',
      ...(receipt ? { receipt_id: receipt.receipt_id } : {}),
    };
    calls.push(proof);
    if (reserved.kind !== 'model') continue;
    const taskId = Number(call.task_id);
    if (!Number.isSafeInteger(taskId)) continue;
    const task = await harness.getTask(taskId as TaskId, context);
    if (task?.conversationId === root.id) final = receipt;
  }
  const result = final?.result as Record<string, unknown> | undefined;
  const complete =
    final?.status === 'completed' &&
    typeof result?.output === 'string' &&
    result.stop_reason === 'stop' &&
    (result.tool_calls === undefined ||
      (Array.isArray(result.tool_calls) && result.tool_calls.length === 0)) &&
    calls.every((c) => c.status === 'completed');
  if (complete && final) {
    // A bookkeeping-only official write leaves the original Pi submission's interrupted result intact.
    await root.submit(
      {
        type: 'write',
        requestId: `reconciliation:${final.receipt_id}`,
        entry: {
          kind: 'glowloom.reconciled_receipt',
          data: {
            root_run_id: runId,
            segment_id: segmentId,
            receipt_id: final.receipt_id,
            call: { ...final.call },
            provider_result: final.result as JsonObject,
            official_submission_status: 'interrupted',
          },
        },
      },
      context,
    );
  }
  const status = complete ? 'completed' : 'needs_reconciliation';
  await harness.commit(async (tx) => {
    const s = await tx.doc(StateDoc, root.id);
    if (s.root_run_id !== runId || s.segment_id !== segmentId)
      throw new Error('Reconciliation execution segment changed');
    s.reconciliation = {
      segment_id: segmentId,
      status,
      calls,
      official_submission_status: 'interrupted',
    };
  }, context);
  return { status, calls };
}
