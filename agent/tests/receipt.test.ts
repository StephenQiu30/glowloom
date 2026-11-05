import { expect, it } from 'vitest';
import {
  BusinessClient,
  type CallIdentity,
  type CallReceipt,
  type Grant,
} from '../src/integrations/go-business-api/client.js';
import { completedReceipt } from '../src/runtime/pi-durable/model-provider.js';
const call: CallIdentity = {
  task_id: '1',
  call_id: 'call',
  checkpoint_id: 'checkpoint',
  attempt: 1,
  contract_version: '1',
  input_hash: 'a'.repeat(64),
};
const grant = { root_run_id: 'run', submission_id: 'segment' } as Grant;
class ReceiptClient extends BusinessClient {
  constructor(readonly saved: CallReceipt) {
    super('http://fixture.invalid', 'fixture');
  }
  override async findCall() {
    return this.saved;
  }
}
it.each([
  'root_run_id',
  'submission_id',
  'task_id',
  'input_hash',
  'attempt',
  'contract_version',
  'checkpoint_id',
  'call_id',
])(
  'rejects a recovered receipt with a mismatched %s before adopting its result',
  async (field) => {
    const receipt: CallReceipt = {
      receipt_id: 'receipt',
      root_run_id: 'run',
      submission_id: 'segment',
      call: { ...call },
      status: 'completed',
      cost_micros: 0,
      result: {},
    };
    if (field === 'root_run_id' || field === 'submission_id')
      receipt[field] = 'other';
    else
      (receipt.call as unknown as Record<string, unknown>)[field] =
        field === 'attempt' ? 2 : 'other';
    let sends = 0;
    await expect(
      completedReceipt(new ReceiptClient(receipt), grant, call, async () => {
        sends++;
        return receipt;
      }),
    ).rejects.toThrow('identity mismatch');
    expect(sends).toBe(0);
  },
);
