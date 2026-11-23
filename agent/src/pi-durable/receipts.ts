import type {
  BusinessClient,
  Grant,
  CallIdentity,
  CallReceipt,
} from '../business/client.js';
import {
  ProjectEvidenceChanged,
  ProjectMemoryChanged,
} from './context/project.js';

export async function completedReceipt(
  client: BusinessClient,
  grant: Grant,
  call: CallIdentity,
  send: () => Promise<CallReceipt>,
): Promise<CallReceipt> {
  let receipt = await client.findCall(grant, call);
  if (!receipt) receipt = await send();
  if (
    receipt.root_run_id !== grant.root_run_id ||
    receipt.submission_id !== grant.submission_id ||
    (
      [
        'task_id',
        'call_id',
        'checkpoint_id',
        'attempt',
        'contract_version',
        'input_hash',
      ] as const
    ).some((key) => receipt.call[key] !== call[key])
  )
    throw new Error('Business receipt identity mismatch');
  // An unknown outcome is reconciled by the original identity; never submit another call.
  if (
    receipt.status === 'failed' &&
    receipt.result &&
    typeof receipt.result === 'object' &&
    'code' in receipt.result
  ) {
    if (receipt.result.code === 'project_memory_changed')
      throw new ProjectMemoryChanged();
    if (receipt.result.code === 'project_evidence_changed')
      throw new ProjectEvidenceChanged();
  }
  if (receipt.status !== 'completed')
    throw new Error(
      receipt.status === 'failed'
        ? 'Business call failed'
        : 'Business call requires reconciliation',
    );
  return receipt;
}
